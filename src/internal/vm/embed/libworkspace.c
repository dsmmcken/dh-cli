/*
 * libworkspace.c — LD_PRELOAD library for transparent workspace file access.
 *
 * Intercepts glibc file operations (openat, fstatat, faccessat) and proxies
 * requests for /workspace/* paths to a host file server over vsock. Files are
 * cached locally in /tmp/.wscache/ for subsequent access.
 *
 * Compile: gcc -shared -fPIC -O2 -o libworkspace.so libworkspace.c -ldl -lpthread
 */

#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <pthread.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

/* Linux vsock definitions */
#ifndef AF_VSOCK
#define AF_VSOCK 40
#endif

#ifndef VMADDR_CID_HOST
#define VMADDR_CID_HOST 2
#endif

struct sockaddr_vm {
    unsigned short svm_family;
    unsigned short svm_reserved1;
    unsigned int svm_port;
    unsigned int svm_cid;
    unsigned char svm_zero[sizeof(struct sockaddr) -
                           sizeof(unsigned short) -
                           sizeof(unsigned short) -
                           sizeof(unsigned int) -
                           sizeof(unsigned int)];
};

/* Protocol constants — must match fileserver_linux.go */
#define FILE_SERVER_PORT 10001
#define OP_STAT    1
#define OP_READ    2
#define OP_READDIR 3
#define STATUS_OK    0
#define STATUS_NOENT 1

#define WORKSPACE_PREFIX "/workspace/"
#define WORKSPACE_PREFIX_LEN 11
#define WORKSPACE_DIR "/workspace"
#define WORKSPACE_DIR_LEN 10
#define CACHE_DIR "/tmp/.wscache/"
#define CACHE_DIR_LEN 14
#define READ_CHUNK_SIZE (1024 * 1024)  /* 1 MiB */

/* Re-entrancy guard: prevents infinite recursion when our hook calls libc. */
static __thread int in_hook = 0;

/* Persistent vsock connection, protected by mutex. */
static pthread_mutex_t vsock_mu = PTHREAD_MUTEX_INITIALIZER;
static int vsock_fd = -1;

/* Real libc function pointers. */
typedef int (*real_openat_t)(int, const char *, int, ...);
typedef int (*real_fstatat_t)(int, const char *, struct stat *, int);
typedef int (*real_faccessat_t)(int, const char *, int, int);

static real_openat_t    real_openat    = NULL;
static real_fstatat_t   real_fstatat   = NULL;
static real_faccessat_t real_faccessat = NULL;

/* ---- Initialization ---- */

static void init_real_funcs(void) {
    if (!real_openat) {
        real_openat = (real_openat_t)dlsym(RTLD_NEXT, "openat");
    }
    if (!real_fstatat) {
        real_fstatat = (real_fstatat_t)dlsym(RTLD_NEXT, "fstatat");
    }
    if (!real_faccessat) {
        real_faccessat = (real_faccessat_t)dlsym(RTLD_NEXT, "faccessat");
    }
}

__attribute__((constructor))
static void lib_init(void) {
    init_real_funcs();
}

/* ---- Path helpers ---- */

/*
 * resolve_path: resolve pathname relative to dirfd into an absolute path.
 * Handles AT_FDCWD, absolute paths, and relative paths.
 */
static int resolve_path(int dirfd, const char *pathname, char *resolved, size_t resolved_size) {
    if (!pathname)
        return -1;

    if (pathname[0] == '/') {
        /* Absolute path */
        if (strlen(pathname) >= resolved_size)
            return -1;
        strncpy(resolved, pathname, resolved_size - 1);
        resolved[resolved_size - 1] = '\0';
        return 0;
    }

    /* Relative path — need CWD or dirfd */
    char dirpath[PATH_MAX];
    if (dirfd == AT_FDCWD) {
        if (!getcwd(dirpath, sizeof(dirpath)))
            return -1;
    } else {
        /* Read /proc/self/fd/<dirfd> to get the directory path */
        char fdlink[64];
        snprintf(fdlink, sizeof(fdlink), "/proc/self/fd/%d", dirfd);
        ssize_t n = readlink(fdlink, dirpath, sizeof(dirpath) - 1);
        if (n < 0)
            return -1;
        dirpath[n] = '\0';
    }

    int len = snprintf(resolved, resolved_size, "%s/%s", dirpath, pathname);
    if (len < 0 || (size_t)len >= resolved_size)
        return -1;
    return 0;
}

/*
 * is_workspace_path: check if resolved path starts with /workspace/.
 * If so, set *rel to point past the prefix.
 * Also matches /workspace exactly (the directory itself).
 */
static int is_workspace_path(const char *resolved, const char **rel) {
    if (strncmp(resolved, WORKSPACE_PREFIX, WORKSPACE_PREFIX_LEN) == 0) {
        *rel = resolved + WORKSPACE_PREFIX_LEN;
        return 1;
    }
    /* Exact match for /workspace directory itself */
    if (strcmp(resolved, WORKSPACE_DIR) == 0) {
        *rel = "";
        return 1;
    }
    return 0;
}

/* Build cache path: /tmp/.wscache/<relpath> */
static int cache_path_for(const char *rel, char *buf, size_t bufsize) {
    int n = snprintf(buf, bufsize, "%s%s", CACHE_DIR, rel);
    return (n < 0 || (size_t)n >= bufsize) ? -1 : 0;
}

/* ---- Vsock communication ---- */

/* Connect to host file server. Caller must hold vsock_mu. */
static int vsock_connect(void) {
    if (vsock_fd >= 0)
        return 0;

    int fd = socket(AF_VSOCK, SOCK_STREAM, 0);
    if (fd < 0)
        return -1;

    /* Set timeouts */
    struct timeval tv;
    tv.tv_sec = 5;
    tv.tv_usec = 0;
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv));

    struct sockaddr_vm addr;
    memset(&addr, 0, sizeof(addr));
    addr.svm_family = AF_VSOCK;
    addr.svm_cid = VMADDR_CID_HOST;
    addr.svm_port = FILE_SERVER_PORT;

    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return -1;
    }

    vsock_fd = fd;
    return 0;
}

/* Close vsock connection. Caller must hold vsock_mu. */
static void vsock_disconnect(void) {
    if (vsock_fd >= 0) {
        close(vsock_fd);
        vsock_fd = -1;
    }
}

/* Send exactly n bytes. Returns 0 on success, -1 on error. */
static int send_all(int fd, const void *buf, size_t n) {
    const char *p = (const char *)buf;
    while (n > 0) {
        ssize_t sent = send(fd, p, n, MSG_NOSIGNAL);
        if (sent <= 0) return -1;
        p += sent;
        n -= sent;
    }
    return 0;
}

/* Receive exactly n bytes. Returns 0 on success, -1 on error. */
static int recv_all(int fd, void *buf, size_t n) {
    char *p = (char *)buf;
    while (n > 0) {
        ssize_t got = recv(fd, p, n, 0);
        if (got <= 0) return -1;
        p += got;
        n -= got;
    }
    return 0;
}

/*
 * send_request: send a length-prefixed binary message and read the response.
 * Caller must hold vsock_mu and have called vsock_connect().
 * Returns allocated response buffer (caller frees) or NULL on error.
 * Sets *resp_len to the payload length.
 */
static uint8_t *send_request(const uint8_t *msg, uint32_t msg_len, uint32_t *resp_len) {
    /* Send: [4-byte length][payload] */
    uint8_t len_buf[4];
    len_buf[0] = (msg_len >> 24) & 0xFF;
    len_buf[1] = (msg_len >> 16) & 0xFF;
    len_buf[2] = (msg_len >> 8) & 0xFF;
    len_buf[3] = msg_len & 0xFF;

    if (send_all(vsock_fd, len_buf, 4) < 0) return NULL;
    if (send_all(vsock_fd, msg, msg_len) < 0) return NULL;

    /* Receive: [4-byte length][payload] */
    if (recv_all(vsock_fd, len_buf, 4) < 0) return NULL;
    *resp_len = ((uint32_t)len_buf[0] << 24) |
                ((uint32_t)len_buf[1] << 16) |
                ((uint32_t)len_buf[2] << 8) |
                (uint32_t)len_buf[3];

    if (*resp_len == 0 || *resp_len > 16 * 1024 * 1024) return NULL;

    uint8_t *resp = (uint8_t *)malloc(*resp_len);
    if (!resp) return NULL;

    if (recv_all(vsock_fd, resp, *resp_len) < 0) {
        free(resp);
        return NULL;
    }
    return resp;
}

/* ---- Remote file operations ---- */

/*
 * remote_stat: stat a file on the host.
 * Returns 0 on success, fills st. Returns -1 on error.
 */
static int remote_stat(const char *rel, struct stat *st) {
    uint16_t path_len = (uint16_t)strlen(rel);

    /* Build request: [op=1][2-byte path_len][path] */
    uint32_t msg_len = 1 + 2 + path_len;
    uint8_t *msg = (uint8_t *)alloca(msg_len);
    msg[0] = OP_STAT;
    msg[1] = (path_len >> 8) & 0xFF;
    msg[2] = path_len & 0xFF;
    memcpy(msg + 3, rel, path_len);

    pthread_mutex_lock(&vsock_mu);
    if (vsock_connect() < 0) {
        pthread_mutex_unlock(&vsock_mu);
        return -1;
    }

    uint32_t resp_len;
    uint8_t *resp = send_request(msg, msg_len, &resp_len);
    if (!resp) {
        vsock_disconnect();
        pthread_mutex_unlock(&vsock_mu);
        return -1;
    }
    pthread_mutex_unlock(&vsock_mu);

    /* Parse: [status][4-byte mode][8-byte size][8-byte mtime][1-byte is_dir] */
    if (resp_len < 1 || resp[0] != STATUS_OK) {
        free(resp);
        return -1;
    }
    if (resp_len < 22) {
        free(resp);
        return -1;
    }

    memset(st, 0, sizeof(*st));
    uint32_t mode = ((uint32_t)resp[1] << 24) | ((uint32_t)resp[2] << 16) |
                    ((uint32_t)resp[3] << 8) | resp[4];
    uint64_t size = ((uint64_t)resp[5] << 56) | ((uint64_t)resp[6] << 48) |
                    ((uint64_t)resp[7] << 40) | ((uint64_t)resp[8] << 32) |
                    ((uint64_t)resp[9] << 24) | ((uint64_t)resp[10] << 16) |
                    ((uint64_t)resp[11] << 8) | resp[12];
    uint64_t mtime = ((uint64_t)resp[13] << 56) | ((uint64_t)resp[14] << 48) |
                     ((uint64_t)resp[15] << 40) | ((uint64_t)resp[16] << 32) |
                     ((uint64_t)resp[17] << 24) | ((uint64_t)resp[18] << 16) |
                     ((uint64_t)resp[19] << 8) | resp[20];
    uint8_t is_dir = resp[21];

    st->st_mode = (mode_t)mode;
    if (is_dir) {
        st->st_mode |= S_IFDIR;
    } else {
        st->st_mode |= S_IFREG;
    }
    st->st_size = (off_t)size;
    st->st_mtim.tv_sec = (time_t)mtime;
    st->st_nlink = is_dir ? 2 : 1;
    st->st_blksize = 4096;
    st->st_blocks = (size + 511) / 512;

    free(resp);
    return 0;
}

/*
 * remote_read_chunk: read a chunk of a file from the host.
 * Returns bytes read (0 = EOF), or -1 on error.
 */
static ssize_t remote_read_chunk(const char *rel, uint64_t offset, void *buf, uint32_t len) {
    uint16_t path_len = (uint16_t)strlen(rel);

    /* Build request: [op=2][2-byte path_len][path][8-byte offset][4-byte len] */
    uint32_t msg_len = 1 + 2 + path_len + 8 + 4;
    uint8_t *msg = (uint8_t *)alloca(msg_len);
    msg[0] = OP_READ;
    msg[1] = (path_len >> 8) & 0xFF;
    msg[2] = path_len & 0xFF;
    memcpy(msg + 3, rel, path_len);
    uint32_t off_pos = 3 + path_len;
    /* 8-byte offset, big-endian */
    msg[off_pos + 0] = (offset >> 56) & 0xFF;
    msg[off_pos + 1] = (offset >> 48) & 0xFF;
    msg[off_pos + 2] = (offset >> 40) & 0xFF;
    msg[off_pos + 3] = (offset >> 32) & 0xFF;
    msg[off_pos + 4] = (offset >> 24) & 0xFF;
    msg[off_pos + 5] = (offset >> 16) & 0xFF;
    msg[off_pos + 6] = (offset >> 8) & 0xFF;
    msg[off_pos + 7] = offset & 0xFF;
    /* 4-byte length, big-endian */
    msg[off_pos + 8] = (len >> 24) & 0xFF;
    msg[off_pos + 9] = (len >> 16) & 0xFF;
    msg[off_pos + 10] = (len >> 8) & 0xFF;
    msg[off_pos + 11] = len & 0xFF;

    pthread_mutex_lock(&vsock_mu);
    if (vsock_connect() < 0) {
        pthread_mutex_unlock(&vsock_mu);
        return -1;
    }

    uint32_t resp_len;
    uint8_t *resp = send_request(msg, msg_len, &resp_len);
    if (!resp) {
        vsock_disconnect();
        pthread_mutex_unlock(&vsock_mu);
        return -1;
    }
    pthread_mutex_unlock(&vsock_mu);

    /* Parse: [status=0][4-byte bytes_read][raw bytes] */
    if (resp_len < 1 || resp[0] != STATUS_OK) {
        free(resp);
        return -1;
    }
    if (resp_len < 5) {
        free(resp);
        return 0;
    }

    uint32_t bytes_read = ((uint32_t)resp[1] << 24) | ((uint32_t)resp[2] << 16) |
                          ((uint32_t)resp[3] << 8) | resp[4];
    if (bytes_read > resp_len - 5) {
        bytes_read = resp_len - 5;
    }
    if (bytes_read > len) {
        bytes_read = len;
    }

    memcpy(buf, resp + 5, bytes_read);
    free(resp);
    return (ssize_t)bytes_read;
}

/* ---- Cache management ---- */

/* Create parent directories for a cache path (including cache root). */
static void mkdirs(const char *path) {
    char tmp[PATH_MAX];
    strncpy(tmp, path, sizeof(tmp) - 1);
    tmp[sizeof(tmp) - 1] = '\0';

    /* Ensure cache root exists first */
    mkdir("/tmp/.wscache", 0755);

    for (char *p = tmp + CACHE_DIR_LEN; *p; p++) {
        if (*p == '/') {
            *p = '\0';
            mkdir(tmp, 0755);
            *p = '/';
        }
    }
}

/*
 * ensure_cached_file: fetch a file from the host and cache it locally.
 * Returns 0 on success, -1 on error.
 */
static int ensure_cached_file(const char *rel) {
    char cache_path[PATH_MAX];
    if (cache_path_for(rel, cache_path, sizeof(cache_path)) < 0)
        return -1;

    /* Check if already cached */
    struct stat st;
    if (real_fstatat(AT_FDCWD, cache_path, &st, 0) == 0)
        return 0;

    /* Stat remote file first */
    struct stat remote_st;
    if (remote_stat(rel, &remote_st) < 0)
        return -1;

    if (S_ISDIR(remote_st.st_mode)) {
        /* For directories, just create the cache dir */
        mkdirs(cache_path);
        mkdir(cache_path, 0755);
        return 0;
    }

    /* Create parent directories */
    mkdirs(cache_path);

    /* Atomic write: mkstemp → write → rename */
    char tmp_path[PATH_MAX];
    snprintf(tmp_path, sizeof(tmp_path), "%s.XXXXXX", cache_path);
    int tmp_fd = mkstemp(tmp_path);
    if (tmp_fd < 0)
        return -1;

    /* Download file in chunks */
    uint64_t offset = 0;
    uint64_t file_size = (uint64_t)remote_st.st_size;
    uint8_t *chunk_buf = (uint8_t *)malloc(READ_CHUNK_SIZE);
    if (!chunk_buf) {
        close(tmp_fd);
        unlink(tmp_path);
        return -1;
    }

    while (offset < file_size) {
        uint32_t want = READ_CHUNK_SIZE;
        if (file_size - offset < want)
            want = (uint32_t)(file_size - offset);

        ssize_t got = remote_read_chunk(rel, offset, chunk_buf, want);
        if (got < 0) {
            free(chunk_buf);
            close(tmp_fd);
            unlink(tmp_path);
            return -1;
        }
        if (got == 0)
            break;

        ssize_t written = 0;
        while (written < got) {
            ssize_t w = write(tmp_fd, chunk_buf + written, got - written);
            if (w <= 0) {
                free(chunk_buf);
                close(tmp_fd);
                unlink(tmp_path);
                return -1;
            }
            written += w;
        }
        offset += got;
    }

    free(chunk_buf);
    close(tmp_fd);

    /* Atomic rename into place */
    if (rename(tmp_path, cache_path) < 0) {
        unlink(tmp_path);
        return -1;
    }

    return 0;
}

/*
 * ensure_cached_stat: for fstatat, we need stat info. If the file is cached,
 * stat the cache. Otherwise, do a remote stat (and optionally cache the file).
 */
static int ensure_cached_stat(const char *rel, struct stat *st) {
    char cache_path[PATH_MAX];
    if (cache_path_for(rel, cache_path, sizeof(cache_path)) < 0)
        return -1;

    /* If cached, stat the cache file */
    if (real_fstatat(AT_FDCWD, cache_path, st, 0) == 0)
        return 0;

    /* Not cached — do remote stat */
    return remote_stat(rel, st);
}

/* ---- Intercepted functions ---- */

int openat(int dirfd, const char *pathname, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | __O_TMPFILE)) {
        va_list ap;
        va_start(ap, flags);
        mode = (mode_t)va_arg(ap, int);
        va_end(ap);
    }

    init_real_funcs();

    if (in_hook || !pathname)
        return real_openat(dirfd, pathname, flags, mode);

    char resolved[PATH_MAX];
    if (resolve_path(dirfd, pathname, resolved, sizeof(resolved)) < 0)
        return real_openat(dirfd, pathname, flags, mode);

    const char *rel;
    if (!is_workspace_path(resolved, &rel))
        return real_openat(dirfd, pathname, flags, mode);

    /* Read-only workspace */
    if (flags & (O_WRONLY | O_RDWR)) {
        errno = EROFS;
        return -1;
    }

    in_hook = 1;

    /* Handle /workspace directory itself */
    if (*rel == '\0') {
        in_hook = 0;
        /* Return a fd to a synthetic directory — just use /tmp/.wscache/ */
        mkdir(CACHE_DIR, 0755);
        return real_openat(AT_FDCWD, CACHE_DIR, O_RDONLY | O_DIRECTORY, 0);
    }

    int rc = ensure_cached_file(rel);
    in_hook = 0;

    if (rc < 0) {
        errno = ENOENT;
        return -1;
    }

    char cache_path[PATH_MAX];
    if (cache_path_for(rel, cache_path, sizeof(cache_path)) < 0) {
        errno = ENOENT;
        return -1;
    }

    in_hook = 1;
    int fd = real_openat(AT_FDCWD, cache_path, flags & ~(O_CREAT | O_EXCL), 0);
    in_hook = 0;
    return fd;
}

/* Also intercept open() which may not go through openat on some systems */
int open(const char *pathname, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | __O_TMPFILE)) {
        va_list ap;
        va_start(ap, flags);
        mode = (mode_t)va_arg(ap, int);
        va_end(ap);
    }
    return openat(AT_FDCWD, pathname, flags, mode);
}

/* open64 — JVM (OpenJDK) calls open64 directly for file I/O */
int open64(const char *pathname, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | __O_TMPFILE)) {
        va_list ap;
        va_start(ap, flags);
        mode = (mode_t)va_arg(ap, int);
        va_end(ap);
    }
    return openat(AT_FDCWD, pathname, flags, mode);
}

/* openat64 — 64-bit variant of openat */
int openat64(int dirfd, const char *pathname, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | __O_TMPFILE)) {
        va_list ap;
        va_start(ap, flags);
        mode = (mode_t)va_arg(ap, int);
        va_end(ap);
    }
    return openat(dirfd, pathname, flags, mode);
}

int fstatat(int dirfd, const char *pathname, struct stat *statbuf, int flags) {
    init_real_funcs();

    if (in_hook || !pathname)
        return real_fstatat(dirfd, pathname, statbuf, flags);

    char resolved[PATH_MAX];
    if (resolve_path(dirfd, pathname, resolved, sizeof(resolved)) < 0)
        return real_fstatat(dirfd, pathname, statbuf, flags);

    const char *rel;
    if (!is_workspace_path(resolved, &rel))
        return real_fstatat(dirfd, pathname, statbuf, flags);

    in_hook = 1;

    int rc;
    if (*rel == '\0') {
        /* /workspace itself — synthesize a directory stat */
        memset(statbuf, 0, sizeof(*statbuf));
        statbuf->st_mode = S_IFDIR | 0755;
        statbuf->st_nlink = 2;
        rc = 0;
    } else {
        rc = ensure_cached_stat(rel, statbuf);
    }

    in_hook = 0;

    if (rc < 0) {
        errno = ENOENT;
        return -1;
    }
    return 0;
}

/* stat/lstat wrappers that go through fstatat */
int __xstat(int ver, const char *pathname, struct stat *statbuf) {
    return fstatat(AT_FDCWD, pathname, statbuf, 0);
}

int __lxstat(int ver, const char *pathname, struct stat *statbuf) {
    return fstatat(AT_FDCWD, pathname, statbuf, AT_SYMLINK_NOFOLLOW);
}

int stat(const char *pathname, struct stat *statbuf) {
    return fstatat(AT_FDCWD, pathname, statbuf, 0);
}

int lstat(const char *pathname, struct stat *statbuf) {
    return fstatat(AT_FDCWD, pathname, statbuf, AT_SYMLINK_NOFOLLOW);
}

int faccessat(int dirfd, const char *pathname, int mode, int flags) {
    init_real_funcs();

    if (in_hook || !pathname)
        return real_faccessat(dirfd, pathname, mode, flags);

    char resolved[PATH_MAX];
    if (resolve_path(dirfd, pathname, resolved, sizeof(resolved)) < 0)
        return real_faccessat(dirfd, pathname, mode, flags);

    const char *rel;
    if (!is_workspace_path(resolved, &rel))
        return real_faccessat(dirfd, pathname, mode, flags);

    /* Write access to workspace is denied */
    if (mode & W_OK) {
        errno = EROFS;
        return -1;
    }

    in_hook = 1;

    int rc;
    if (*rel == '\0') {
        /* /workspace itself always exists */
        rc = 0;
    } else {
        /* Check if file exists via remote stat */
        struct stat st;
        rc = ensure_cached_stat(rel, &st);
    }

    in_hook = 0;

    if (rc < 0) {
        errno = ENOENT;
        return -1;
    }
    return 0;
}

int access(const char *pathname, int mode) {
    return faccessat(AT_FDCWD, pathname, mode, 0);
}

/* FILE* wrappers — fopen routes through open, so these are covered by openat.
 * But just in case some implementations call fopen directly: */
FILE *fopen(const char *pathname, const char *mode) {
    init_real_funcs();

    if (in_hook || !pathname) {
        typedef FILE *(*real_fopen_t)(const char *, const char *);
        static real_fopen_t real_fopen = NULL;
        if (!real_fopen) real_fopen = (real_fopen_t)dlsym(RTLD_NEXT, "fopen");
        return real_fopen(pathname, mode);
    }

    char resolved[PATH_MAX];
    if (resolve_path(AT_FDCWD, pathname, resolved, sizeof(resolved)) < 0) {
        typedef FILE *(*real_fopen_t)(const char *, const char *);
        static real_fopen_t real_fopen = NULL;
        if (!real_fopen) real_fopen = (real_fopen_t)dlsym(RTLD_NEXT, "fopen");
        return real_fopen(pathname, mode);
    }

    const char *rel;
    if (!is_workspace_path(resolved, &rel)) {
        typedef FILE *(*real_fopen_t)(const char *, const char *);
        static real_fopen_t real_fopen = NULL;
        if (!real_fopen) real_fopen = (real_fopen_t)dlsym(RTLD_NEXT, "fopen");
        return real_fopen(pathname, mode);
    }

    /* Reject write modes */
    if (strchr(mode, 'w') || strchr(mode, 'a') || strchr(mode, '+')) {
        errno = EROFS;
        return NULL;
    }

    /* Use our openat to get a cached fd, then fdopen */
    int fd = openat(AT_FDCWD, pathname, O_RDONLY, 0);
    if (fd < 0)
        return NULL;

    typedef FILE *(*real_fdopen_t)(int, const char *);
    static real_fdopen_t real_fdopen = NULL;
    if (!real_fdopen) real_fdopen = (real_fdopen_t)dlsym(RTLD_NEXT, "fdopen");
    return real_fdopen(fd, "r");
}

/* fopen64 — same as fopen on modern glibc */
FILE *fopen64(const char *pathname, const char *mode) {
    return fopen(pathname, mode);
}
