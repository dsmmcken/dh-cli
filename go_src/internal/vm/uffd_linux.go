//go:build linux

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// UFFD ioctl numbers for amd64.
const (
	// UFFDIO_COPY: _IOWR(0xAA, 0x03, struct uffdio_copy) where sizeof = 40.
	_UFFDIO_COPY = 0xc028aa03

	// UFFDIO_ZEROPAGE: _IOWR(0xAA, 0x04, struct uffdio_zeropage) where sizeof = 32.
	_UFFDIO_ZEROPAGE = 0xc020aa04
)

// copyChunkSize is the size of each UFFDIO_COPY request. Larger chunks reduce
// the number of ioctls (2GB / 256MB = 8 ioctls instead of ~500K page faults).
const copyChunkSize = 256 * 1024 * 1024

// uffdMsgSize is the size of struct uffd_msg (32 bytes on amd64).
const uffdMsgSize = 32

// UFFD event types from linux/userfaultfd.h.
const (
	_UFFD_EVENT_PAGEFAULT = 0x12
	_UFFD_EVENT_REMOVE    = 0x15
)

// ufffdioCopy matches struct uffdio_copy from linux/userfaultfd.h (40 bytes).
type ufffdioCopy struct {
	dst  uint64 // destination address (in uffd-registered range)
	src  uint64 // source address (our mmap'd buffer)
	len  uint64 // length in bytes
	mode uint64 // flags (0 for eager copy)
	copy int64  // output: bytes actually copied, or negative errno
}

// Compile-time size assertion.
var _ [40]byte = [unsafe.Sizeof(ufffdioCopy{})]byte{}

// uffdioZeropage matches struct uffdio_zeropage from linux/userfaultfd.h (32 bytes).
type uffdioZeropage struct {
	start    uint64 // start of range (uffdio_range.start)
	len      uint64 // length of range (uffdio_range.len)
	mode     uint64 // flags (0)
	zeropage int64  // output: bytes zeroed, or negative errno
}

// Compile-time size assertion.
var _ [32]byte = [unsafe.Sizeof(uffdioZeropage{})]byte{}

// memRegion is the JSON format Firecracker sends over the UFFD UDS.
// See GuestRegionUffdMapping in Firecracker's persist.rs.
type memRegion struct {
	BaseHostVirtAddr uint64 `json:"base_host_virt_addr"`
	Size             uint64 `json:"size"`
	Offset           uint64 `json:"offset"`
	PageSize         uint64 `json:"page_size"`     // bytes (Firecracker v1.12+)
	PageSizeKiB      uint64 `json:"page_size_kib"` // deprecated, actually bytes despite name
}

// pageSize returns the effective page size for this region.
func (r *memRegion) pageSize() uint64 {
	if r.PageSize > 0 {
		return r.PageSize
	}
	// Fall back to deprecated field (same unit despite the name).
	if r.PageSizeKiB > 0 {
		return r.PageSizeKiB
	}
	return 4096
}

// dataExtent describes a contiguous range of non-hole data in the snapshot file.
type dataExtent struct {
	offset uint64 // offset in file
	length uint64 // length of data region
}

// ProbeUffd checks whether the userfaultfd(2) syscall is available on this
// system. Returns true if a UFFD fd was successfully created (and closed).
// Common failure: vm.unprivileged_userfaultfd=0 and no CAP_SYS_PTRACE.
func ProbeUffd() bool {
	fd, _, errno := unix.Syscall(unix.SYS_USERFAULTFD, unix.O_CLOEXEC|unix.O_NONBLOCK, 0, 0)
	if errno != 0 {
		return false
	}
	unix.Close(int(fd))
	return true
}

// uffdHandler manages the UFFD socket lifecycle for page population.
// It eagerly copies data regions via UFFDIO_COPY, then handles page faults
// on hole regions lazily via a background goroutine.
type uffdHandler struct {
	socketPath string
	memFile    string
	listener   *net.UnixListener
	uffdFd     int       // kept open for VM lifetime; -1 if not yet received
	done       chan error // signaled when eager population completes (nil = success)
	cancel     context.CancelFunc
}

// startUffdHandler creates a UDS listener and spawns a goroutine that waits for
// Firecracker to connect, receives the UFFD fd, and eagerly populates data pages.
// The socket file exists after this returns (satisfying SDK validation).
func startUffdHandler(ctx context.Context, socketPath, memFilePath string, stderr io.Writer) (*uffdHandler, error) {
	// Remove stale socket if present
	os.Remove(socketPath)

	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("listening on UFFD socket %s: %w", socketPath, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	h := &uffdHandler{
		socketPath: socketPath,
		memFile:    memFilePath,
		listener:   listener,
		uffdFd:     -1,
		done:       make(chan error, 1),
		cancel:     cancel,
	}

	go h.run(ctx, stderr)
	return h, nil
}

// Wait blocks until eager data population completes or the context is cancelled.
func (h *uffdHandler) Wait(ctx context.Context) error {
	select {
	case err := <-h.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close cleans up the UFFD handler: closes the fd, listener, and removes the socket.
func (h *uffdHandler) Close() error {
	h.cancel()
	if h.uffdFd >= 0 {
		unix.Close(h.uffdFd)
		h.uffdFd = -1
	}
	h.listener.Close()
	os.Remove(h.socketPath)
	return nil
}

// run is the main handler goroutine. It eagerly populates data pages, then
// starts a lazy fault handler for hole pages.
func (h *uffdHandler) run(ctx context.Context, stderr io.Writer) {
	h.done <- h.doPopulate(ctx, stderr)
}

func (h *uffdHandler) doPopulate(ctx context.Context, stderr io.Writer) error {
	// Accept connection from Firecracker (blocks until snapshot load)
	conn, err := h.listener.AcceptUnix()
	if err != nil {
		return fmt.Errorf("accepting UFFD connection: %w", err)
	}
	defer conn.Close()

	// Receive UFFD fd (SCM_RIGHTS) and memory region JSON
	uffdFd, regions, err := receiveUffdAndRegions(conn)
	if err != nil {
		return fmt.Errorf("receiving UFFD handshake: %w", err)
	}
	h.uffdFd = uffdFd

	if len(regions) == 0 {
		return fmt.Errorf("Firecracker sent 0 memory regions")
	}

	// Open the snapshot memory file for sparse scanning and mmap
	f, err := os.Open(h.memFile)
	if err != nil {
		return fmt.Errorf("opening memory file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat memory file: %w", err)
	}
	fileSize := uint64(fi.Size())

	// Hint the kernel about sequential access for readahead
	unix.Fadvise(int(f.Fd()), 0, int64(fileSize), unix.FADV_SEQUENTIAL)

	data, err := unix.Mmap(int(f.Fd()), 0, int(fileSize), unix.PROT_READ, unix.MAP_PRIVATE|unix.MAP_POPULATE)
	if err != nil {
		return fmt.Errorf("mmap memory file: %w", err)
	}
	defer unix.Munmap(data)

	mmapBase := uintptr(unsafe.Pointer(&data[0]))

	// Eagerly populate only data regions (non-holes). Hole pages are left
	// unpopulated and served lazily via the background fault handler.
	var totalData, totalHoles uint64
	sparse := false
	for i, region := range regions {
		if err := ctx.Err(); err != nil {
			return err
		}

		extents, err := scanDataExtents(f, region.Offset, region.Size)
		if err != nil {
			// If sparse scanning fails, fall back to copying the entire region.
			if err := populateRegionFull(uffdFd, region, mmapBase); err != nil {
				return fmt.Errorf("populating region %d (full): %w", i, err)
			}
			totalData += region.Size
			continue
		}

		sparse = true
		dataCopied, err := populateRegionDataOnly(uffdFd, region, mmapBase, extents)
		if err != nil {
			return fmt.Errorf("populating region %d data: %w", i, err)
		}
		totalData += dataCopied
		totalHoles += region.Size - dataCopied
	}

	_ = totalHoles

	// If there are holes, start a background goroutine to handle page faults
	// lazily. Any access to an unpopulated (hole) page by the VM will block
	// the faulting vCPU thread until we respond with UFFDIO_ZEROPAGE.
	if sparse && totalHoles > 0 {
		go h.lazyFaultHandler(ctx, uffdFd)
	}

	return nil
}

// lazyFaultHandler polls the UFFD fd and serves page faults on hole pages
// with UFFDIO_ZEROPAGE. Runs until the context is cancelled (VM destroyed).
func (h *uffdHandler) lazyFaultHandler(ctx context.Context, uffdFd int) {
	var msgBuf [uffdMsgSize]byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Poll with a short timeout so we can check ctx cancellation
		fds := []unix.PollFd{{
			Fd:     int32(uffdFd),
			Events: unix.POLLIN,
		}}
		n, err := unix.Poll(fds, 100) // 100ms timeout
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return // fd closed or error
		}
		if n == 0 {
			continue // timeout, check ctx
		}

		// Read the uffd_msg
		nr, err := unix.Read(uffdFd, msgBuf[:])
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				continue
			}
			return // fd closed
		}
		if nr < uffdMsgSize {
			continue
		}

		// Parse event type (first byte of struct uffd_msg)
		event := msgBuf[0]

		switch event {
		case _UFFD_EVENT_PAGEFAULT:
			// Fault address is at offset 16 in struct uffd_msg (the union field)
			faultAddr := *(*uint64)(unsafe.Pointer(&msgBuf[16]))
			// Page-align the fault address
			pageAddr := faultAddr & ^uint64(4095)

			zp := uffdioZeropage{
				start: pageAddr,
				len:   4096,
				mode:  0,
			}
			unix.Syscall(
				unix.SYS_IOCTL,
				uintptr(uffdFd),
				uintptr(_UFFDIO_ZEROPAGE),
				uintptr(unsafe.Pointer(&zp)),
			)

		case _UFFD_EVENT_REMOVE:
			// Balloon deflation — pages being removed from UFFD tracking.
			// No action needed; the kernel handles the removal.

		default:
			// Ignore unknown events
		}
	}
}

// scanDataExtents uses SEEK_HOLE/SEEK_DATA to find non-hole regions in the
// snapshot file within a given range. Returns an error if the filesystem does
// not support sparse file detection.
func scanDataExtents(f *os.File, rangeOffset, rangeSize uint64) ([]dataExtent, error) {
	fd := int(f.Fd())
	end := int64(rangeOffset + rangeSize)
	pos := int64(rangeOffset)
	var extents []dataExtent

	for pos < end {
		// Find start of next data region
		dataStart, err := unix.Seek(fd, pos, unix.SEEK_DATA)
		if err != nil {
			// ENXIO means no more data after pos — rest is hole
			if err == unix.ENXIO {
				break
			}
			return nil, fmt.Errorf("SEEK_DATA at %d: %w", pos, err)
		}
		if dataStart >= end {
			break
		}

		// Find end of this data region (start of next hole)
		holeStart, err := unix.Seek(fd, dataStart, unix.SEEK_HOLE)
		if err != nil {
			// If SEEK_HOLE fails, treat rest as data
			holeStart = end
		}
		if holeStart > end {
			holeStart = end
		}

		extents = append(extents, dataExtent{
			offset: uint64(dataStart),
			length: uint64(holeStart - dataStart),
		})

		pos = holeStart
	}

	return extents, nil
}

// populateRegionDataOnly copies only data extents (non-holes) for a region.
// Hole pages are left unpopulated — they'll be served by the lazy fault handler.
// Returns bytes actually copied.
func populateRegionDataOnly(uffdFd int, region memRegion, mmapBase uintptr, extents []dataExtent) (uint64, error) {
	baseAddr := region.BaseHostVirtAddr
	regionStart := region.Offset
	regionEnd := region.Offset + region.Size
	var totalCopied uint64

	for _, ext := range extents {
		extEnd := ext.offset + ext.length
		if extEnd > regionEnd {
			extEnd = regionEnd
		}
		for off := ext.offset; off < extEnd; off += copyChunkSize {
			chunkLen := uint64(copyChunkSize)
			if remaining := extEnd - off; remaining < chunkLen {
				chunkLen = remaining
			}

			srcPtr := uint64(mmapBase) + off
			dstAddr := baseAddr + (off - regionStart)

			cp := ufffdioCopy{
				dst:  dstAddr,
				src:  srcPtr,
				len:  chunkLen,
				mode: 0,
			}

			_, _, errno := unix.Syscall(
				unix.SYS_IOCTL,
				uintptr(uffdFd),
				uintptr(_UFFDIO_COPY),
				uintptr(unsafe.Pointer(&cp)),
			)
			if errno != 0 {
				return 0, fmt.Errorf("UFFDIO_COPY at offset %d: %v", off-regionStart, errno)
			}
			if cp.copy < 0 {
				return 0, fmt.Errorf("UFFDIO_COPY returned %d at offset %d", cp.copy, off-regionStart)
			}
			totalCopied += chunkLen
		}
	}

	return totalCopied, nil
}

// populateRegionFull copies all pages for a single memory region using
// UFFDIO_COPY. Used as fallback when sparse scanning is unavailable.
func populateRegionFull(uffdFd int, region memRegion, mmapBase uintptr) error {
	baseAddr := region.BaseHostVirtAddr

	for offset := uint64(0); offset < region.Size; offset += copyChunkSize {
		chunkLen := uint64(copyChunkSize)
		if remaining := region.Size - offset; remaining < chunkLen {
			chunkLen = remaining
		}

		srcPtr := uint64(mmapBase) + region.Offset + offset
		dstAddr := baseAddr + offset

		cp := ufffdioCopy{
			dst:  dstAddr,
			src:  srcPtr,
			len:  chunkLen,
			mode: 0,
		}

		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(uffdFd),
			uintptr(_UFFDIO_COPY),
			uintptr(unsafe.Pointer(&cp)),
		)
		if errno != 0 {
			return fmt.Errorf("UFFDIO_COPY at offset %d: %v", offset, errno)
		}
		if cp.copy < 0 {
			return fmt.Errorf("UFFDIO_COPY returned %d at offset %d", cp.copy, offset)
		}
	}

	return nil
}

// receiveUffdAndRegions receives the UFFD file descriptor (via SCM_RIGHTS) and
// the JSON memory region layout from Firecracker over the Unix socket.
func receiveUffdAndRegions(conn *net.UnixConn) (int, []memRegion, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return -1, nil, fmt.Errorf("getting raw conn: %w", err)
	}

	buf := make([]byte, 64*1024)              // JSON payload buffer
	oob := make([]byte, unix.CmsgSpace(4))    // space for 1 fd (4 bytes)
	var n, oobn int
	var recvErr error

	// Retry up to 5 times — sometimes Firecracker's first message arrives
	// without the fd attached (observed in Firecracker's own example handler).
	var uffdFd int = -1
	for attempt := 0; attempt < 5; attempt++ {
		controlErr := rawConn.Read(func(fd uintptr) bool {
			n, oobn, _, _, recvErr = unix.Recvmsg(int(fd), buf, oob, 0)
			return true
		})
		if controlErr != nil {
			return -1, nil, fmt.Errorf("raw conn read: %w", controlErr)
		}
		if recvErr != nil {
			return -1, nil, fmt.Errorf("recvmsg: %w", recvErr)
		}

		if oobn > 0 {
			scms, err := unix.ParseSocketControlMessage(oob[:oobn])
			if err != nil {
				return -1, nil, fmt.Errorf("parsing control message: %w", err)
			}
			for _, scm := range scms {
				fds, err := unix.ParseUnixRights(&scm)
				if err == nil && len(fds) > 0 {
					uffdFd = fds[0]
					break
				}
			}
		}

		if uffdFd >= 0 && n > 0 {
			break
		}
	}

	if uffdFd < 0 {
		return -1, nil, fmt.Errorf("no UFFD fd received via SCM_RIGHTS after 5 attempts")
	}

	var regions []memRegion
	if err := json.Unmarshal(buf[:n], &regions); err != nil {
		unix.Close(uffdFd)
		return -1, nil, fmt.Errorf("parsing memory regions: %w", err)
	}

	return uffdFd, regions, nil
}
