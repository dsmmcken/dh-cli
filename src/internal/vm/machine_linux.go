//go:build linux

package vm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	// VsockCID is the guest Context Identifier for the vsock device.
	// Must be >= 3 (0=hypervisor, 1=reserved, 2=host).
	VsockCID = 3

	// VsockPort is the port inside the VM where the runner daemon listens.
	VsockPort = 10000
)

// BootAndSnapshot boots a fresh VM, waits for Deephaven readiness,
// then pauses and creates a snapshot. Used by `dh vm prepare`.
func BootAndSnapshot(ctx context.Context, cfg *VMConfig, paths *VMPaths, stderr io.Writer) error {
	version := cfg.Version
	rootfsPath := paths.RootfsForVersion(version)
	snapDir := paths.SnapshotDirForVersion(version)

	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return fmt.Errorf("creating snapshot dir: %w", err)
	}

	// Copy rootfs as the backing disk for the snapshot
	diskPath := filepath.Join(snapDir, "disk.ext4")
	if err := copyFile(rootfsPath, diskPath); err != nil {
		return fmt.Errorf("copying rootfs for snapshot: %w", err)
	}

	// Create instance directory
	instanceID := fmt.Sprintf("prepare-%d", time.Now().UnixNano())
	instanceDir := paths.InstanceDir(instanceID)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		return fmt.Errorf("creating instance dir: %w", err)
	}
	defer os.RemoveAll(instanceDir)

	socketPath := filepath.Join(instanceDir, "firecracker.sock")
	// Vsock UDS path goes in the snapshot directory because Firecracker embeds
	// the absolute path in the snapshot binary state. On restore, it re-binds
	// at that same path, so it must still be valid (not in a temp instance dir).
	vsockPath := filepath.Join(snapDir, "vsock.sock")

	// Configure Firecracker — uses vsock for host-VM communication (no TAP needed)
	vcpuCount := int64(DefaultVCPUCount)
	memSize := int64(DefaultMemSizeMiB)
	fcCfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: paths.Kernel,
		KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init.sh",
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(diskPath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
		VsockDevices: []firecracker.VsockDevice{
			{
				ID:   "vsock0",
				Path: vsockPath,
				CID:  VsockCID,
			},
		},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  &vcpuCount,
			MemSizeMib: &memSize,
		},
	}

	if cfg.Verbose {
		fmt.Fprintf(stderr, "Booting VM (kernel=%s, rootfs=%s)...\n", paths.Kernel, diskPath)
	}

	// Build command — capture serial console output to stderr
	fcCmd := firecracker.VMCommandBuilder{}.
		WithBin(paths.Firecracker).
		WithSocketPath(socketPath).
		WithStdout(stderr).
		WithStderr(stderr).
		Build(ctx)

	logger := log.New()
	logger.SetLevel(log.WarnLevel)

	machine, err := firecracker.NewMachine(ctx, fcCfg,
		firecracker.WithProcessRunner(fcCmd),
		firecracker.WithLogger(log.NewEntry(logger)),
	)
	if err != nil {
		return fmt.Errorf("creating firecracker machine: %w", err)
	}

	// Add balloon device handler before Start() — Firecracker requires balloon
	// to be configured before InstanceStart. Start with amount=0 (don't reclaim
	// anything yet), deflateOnOom=true so the guest can reclaim memory later.
	machine.Handlers.FcInit = machine.Handlers.FcInit.Append(
		firecracker.NewCreateBalloonHandler(0, true, 0),
	)

	if err := machine.Start(ctx); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}
	defer machine.StopVMM()

	fmt.Fprintf(stderr, "VM booted, waiting for runner daemon via vsock...\n")

	// Wait for the runner daemon inside the VM. It starts AFTER connecting a
	// pydeephaven Session to Deephaven, so a successful vsock connection means
	// both the server and the warm session are ready.
	if err := waitForVsock(ctx, vsockPath, VsockPort, 120*time.Second); err != nil {
		return fmt.Errorf("runner daemon not reachable via vsock within 120s: %w", err)
	}

	// Give the runner daemon a moment to fully enter its accept loop
	time.Sleep(500 * time.Millisecond)

	// Warm up the JVM by running progressively complex scripts through the
	// full execution pipeline. This triggers C2 JIT compilation of Deephaven's
	// run_script, table creation, Arrow serialization, and pickle/base64 code
	// paths. 20 iterations ensures the JVM's tiered compiler promotes all hot
	// methods to optimized native code. The warmed-up JVM state is captured in
	// the snapshot, so subsequent restores skip the multi-second JIT cost.
	fmt.Fprintf(stderr, "Warming up JVM (20 iterations)...\n")
	warmupScripts := []string{
		// Phase 1 (iterations 0-4): basic execution path
		"x = 1\ndel x",
		// Phase 2 (iterations 5-9): table creation + update (DH core JIT path)
		"from deephaven import empty_table\nt = empty_table(1).update(['x = i'])\ndel t, empty_table",
		// Phase 3 (iterations 10-14): wrapper-like pattern (pickle + base64 + IO)
		"import io, pickle, base64\nb = io.StringIO()\nb.write('test')\nd = {'stdout': b.getvalue(), 'result_repr': '1'}\nbase64.b64encode(pickle.dumps(d))\ndel io, pickle, base64, b, d",
		// Phase 4 (iterations 15-19): multi-column table + expressions
		"from deephaven import empty_table\nt = empty_table(10).update(['x = i', 'y = x * x', 'z = (double)x / 3.14'])\ndel t, empty_table",
	}

	for i := 0; i < 20; i++ {
		script := warmupScripts[i/5]
		if i/5 >= len(warmupScripts) {
			script = warmupScripts[len(warmupScripts)-1]
		}
		warmupReq := &VsockRequest{Code: script, ShowTables: true, ShowTableMeta: true}
		warmupResp, err := ExecuteViaVsock(vsockPath, VsockPort, warmupReq)
		if err != nil {
			return fmt.Errorf("JVM warmup iteration %d failed: %w", i, err)
		}
		if warmupResp.Error != nil && *warmupResp.Error != "" {
			return fmt.Errorf("JVM warmup script error: %s", *warmupResp.Error)
		}
		runMs := ""
		if warmupResp.Timing != nil {
			if v, ok := warmupResp.Timing["run_script_ms"]; ok {
				runMs = fmt.Sprintf(" (run_script: %vms)", v)
			}
		}
		fmt.Fprintf(stderr, "  warmup %d/20%s\n", i+1, runMs)
	}

	// Inflate balloon to reclaim unused guest memory before snapshotting.
	// JVM starts with -Xms32m -XX:-AlwaysPreTouch so G1 only commits
	// heap regions on demand. After warmup, committed memory is much
	// lower (~200-400MB). Leave 512 MiB headroom for kernel + JVM +
	// Python residual. deflateOnOom=true reclaims on demand after restore.
	balloonMiB := int64(DefaultMemSizeMiB - 512)
	fmt.Fprintf(stderr, "Inflating balloon to %d MiB to reclaim unused pages...\n", balloonMiB)
	if err := machine.UpdateBalloon(ctx, balloonMiB); err != nil {
		return fmt.Errorf("inflating balloon: %w", err)
	}
	// Wait for guest balloon driver to reclaim pages
	time.Sleep(3 * time.Second)

	// Deflate balloon back to 0 before snapshotting. The inflation already
	// caused MADV_DONTNEED on reclaimed pages (they're now zeros). Deflating
	// returns those page ranges to the guest so the restored VM has full
	// memory available without needing deflateOnOom to kick in.
	fmt.Fprintf(stderr, "Deflating balloon before snapshot...\n")
	if err := machine.UpdateBalloon(ctx, 0); err != nil {
		return fmt.Errorf("deflating balloon: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	if cfg.Verbose {
		fmt.Fprintf(stderr, "Deephaven ready, pausing VM for snapshot...\n")
	}

	// Pause VM
	if err := machine.PauseVM(ctx); err != nil {
		return fmt.Errorf("pausing VM: %w", err)
	}

	// Create snapshot
	memPath := filepath.Join(snapDir, "snapshot_mem")
	statePath := filepath.Join(snapDir, "snapshot_vmstate")

	if err := machine.CreateSnapshot(ctx, memPath, statePath); err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}

	// Post-process the snapshot memory file: punch holes in zero-filled
	// regions to make it sparse. Firecracker writes all pages sequentially
	// (including balloon-freed zeros), so we need to retroactively convert
	// zero regions into filesystem holes.
	if err := punchHoles(memPath, stderr); err != nil {
		// Non-fatal: restore still works, just slower without sparse optimization
		fmt.Fprintf(stderr, "Warning: could not make snapshot sparse: %v\n", err)
	}

	// Clean up the vsock socket file — Firecracker already captured the path
	// in the snapshot state; leaving the stale socket would confuse restore.
	os.Remove(vsockPath)

	// Write metadata
	meta := &SnapshotMetadata{
		Version:    version,
		CreatedAt:  time.Now(),
		DHPort:     DefaultDHPort,
		MemSizeMiB: DefaultMemSizeMiB,
		BalloonMiB: int(balloonMiB),
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(snapDir, "metadata.json"), metaBytes, 0o644); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	if cfg.Verbose {
		fmt.Fprintf(stderr, "Snapshot created at %s\n", snapDir)
	}

	return nil
}

// RestoreFromSnapshot restores a VM from snapshot and returns instance info,
// machine handle, and an optional io.Closer for the UFFD handler (nil when
// using the File backend). The caller must Close the UFFD handler after
// stopping the VM.
func RestoreFromSnapshot(ctx context.Context, cfg *VMConfig, paths *VMPaths, stderr io.Writer) (*InstanceInfo, *firecracker.Machine, io.Closer, error) {
	version := cfg.Version
	snapDir := paths.SnapshotDirForVersion(version)

	if err := CheckSnapshot(paths, version); err != nil {
		return nil, nil, nil, err
	}

	// Create instance directory
	instanceID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
	instanceDir := paths.InstanceDir(instanceID)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("creating instance dir: %w", err)
	}

	socketPath := filepath.Join(instanceDir, "firecracker.sock")
	// Vsock UDS path must match the path embedded in the snapshot state —
	// Firecracker re-binds the vsock at the same absolute path on restore.
	vsockPath := filepath.Join(snapDir, "vsock.sock")

	// Use snapshot disk directly — the VM is ephemeral and destroyed after exec.
	// This avoids copying a multi-GB disk image on every invocation.
	diskPath := filepath.Join(snapDir, "disk.ext4")

	memPath := filepath.Join(snapDir, "snapshot_mem")
	statePath := filepath.Join(snapDir, "snapshot_vmstate")

	// Configure Firecracker for snapshot restore — vsock for communication.
	// KernelImagePath and MachineCfg are required by SDK validation even for restore.
	vcpuCount := int64(DefaultVCPUCount)
	memSize := int64(DefaultMemSizeMiB)
	fcCfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: paths.Kernel,
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(diskPath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
		VsockDevices: []firecracker.VsockDevice{
			{
				ID:   "vsock0",
				Path: vsockPath,
				CID:  VsockCID,
			},
		},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  &vcpuCount,
			MemSizeMib: &memSize,
		},
	}

	fcCmd := firecracker.VMCommandBuilder{}.
		WithBin(paths.Firecracker).
		WithSocketPath(socketPath).
		Build(ctx)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Determine whether to use UFFD — probe the syscall to detect kernel support.
	useUffd := cfg.UseUffd
	if useUffd && !ProbeUffd() {
		if cfg.Verbose {
			fmt.Fprintf(stderr, "UFFD not available (try: sudo sysctl -w vm.unprivileged_userfaultfd=1), falling back to File backend\n")
		}
		useUffd = false
	}

	// Start UFFD handler before creating Machine (socket must exist for SDK validation).
	var uffd *uffdHandler
	if useUffd {
		uffdSocketPath := filepath.Join(instanceDir, "uffd.sock")
		var err error
		uffd, err = startUffdHandler(ctx, uffdSocketPath, memPath, stderr)
		if err != nil {
			os.RemoveAll(instanceDir)
			return nil, nil, nil, fmt.Errorf("starting UFFD handler: %w", err)
		}
	}

	// Build snapshot options. UFFD mode loads without auto-resume so we can
	// populate all pages first; File mode auto-resumes immediately.
	var snapshotOpts []firecracker.WithSnapshotOpt
	if useUffd {
		snapshotOpts = append(snapshotOpts,
			firecracker.WithMemoryBackend(
				models.MemoryBackendBackendTypeUffd,
				filepath.Join(instanceDir, "uffd.sock"),
			),
			func(sc *firecracker.SnapshotConfig) {
				sc.ResumeVM = false
			},
		)
	} else {
		snapshotOpts = append(snapshotOpts,
			func(sc *firecracker.SnapshotConfig) {
				sc.ResumeVM = true
			},
		)
	}

	// WithSnapshot swaps the SDK handler list from default (CreateBootSource, etc.)
	// to snapshot-specific (LoadSnapshot) — setting Config.Snapshot directly
	// does NOT do this swap.
	// UFFD mode: memFilePath must be empty (Firecracker rejects both mem_file_path
	// and mem_backend in the same request).
	snapshotMemArg := memPath
	if useUffd {
		snapshotMemArg = ""
	}
	machine, err := firecracker.NewMachine(ctx, fcCfg,
		firecracker.WithProcessRunner(fcCmd),
		firecracker.WithLogger(log.NewEntry(logger)),
		firecracker.WithSnapshot(snapshotMemArg, statePath, snapshotOpts...),
	)
	if err != nil {
		if uffd != nil {
			uffd.Close()
		}
		os.RemoveAll(instanceDir)
		return nil, nil, nil, fmt.Errorf("creating firecracker machine: %w", err)
	}

	// Remove unnecessary handlers — for snapshot restore we only need StartVMM
	// and LoadSnapshot. Other handlers either configure devices already in the
	// snapshot or set up logging/networking we don't need.
	machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.AddVsocksHandlerName)
	machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.SetupNetworkHandlerName)
	machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.CreateLogFilesHandlerName)
	machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.BootstrapLoggingHandlerName)

	// Remove stale vsock socket from previous runs — Firecracker will re-bind
	// this path during snapshot restore and fails with EADDRINUSE if it exists.
	os.Remove(vsockPath)

	// Page cache warming is started earlier by the caller (WarmSnapshotPageCacheAsync)
	// to maximize overlap. No additional warming needed here.

	// Start loads the snapshot. In UFFD mode, Firecracker connects to the
	// handler which eagerly populates all pages. In File mode, this also
	// resumes the VM (~10ms restore + demand paging during execution).
	if err := machine.Start(ctx); err != nil {
		if uffd != nil {
			uffd.Close()
		}
		os.RemoveAll(instanceDir)
		return nil, nil, nil, fmt.Errorf("restoring from snapshot: %w", err)
	}

	if useUffd {
		// Wait for UFFD handler to finish populating all pages.
		if err := uffd.Wait(ctx); err != nil {
			machine.StopVMM()
			uffd.Close()
			os.RemoveAll(instanceDir)
			return nil, nil, nil, fmt.Errorf("UFFD page population: %w", err)
		}

		// All pages populated — resume VM with zero pending page faults.
		if err := machine.ResumeVM(ctx); err != nil {
			machine.StopVMM()
			uffd.Close()
			os.RemoveAll(instanceDir)
			return nil, nil, nil, fmt.Errorf("resuming VM after UFFD population: %w", err)
		}
	}

	// Skip vsock probe — the daemon was in accept() at snapshot time and
	// will respond on the caller's first real ExecuteViaVsock connection.
	// Probing here wastes a full connect/accept cycle + triggers page faults
	// on a throwaway connection.

	pid, _ := machine.PID()
	info := &InstanceInfo{
		ID:        instanceID,
		PID:       pid,
		Version:   version,
		VsockPath: vsockPath,
	}

	// Write instance info for crash recovery
	infoBytes, _ := json.Marshal(info)
	os.WriteFile(filepath.Join(instanceDir, "instance.json"), infoBytes, 0o644)

	var closer io.Closer
	if uffd != nil {
		closer = uffd
	}
	return info, machine, closer, nil
}

// DestroyInstance tears down a VM instance.
func DestroyInstance(machine *firecracker.Machine, info *InstanceInfo, paths *VMPaths) {
	if machine != nil {
		machine.StopVMM()
	}
	if info != nil {
		os.RemoveAll(paths.InstanceDir(info.ID))
	}
}

// waitForVsock polls a vsock port until it accepts connections.
func waitForVsock(ctx context.Context, udsPath string, port uint32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for vsock port %d", port)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := connectVsock(udsPath, port)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(1 * time.Millisecond)
	}
}

// connectVsock connects to a vsock port on the VM through Firecracker's UDS.
func connectVsock(udsPath string, port uint32) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", udsPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to vsock UDS: %w", err)
	}

	// Firecracker vsock protocol: send "CONNECT <port>\n", expect "OK <local_port>\n"
	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", port); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sending vsock CONNECT: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading vsock response: %w", err)
	}

	if !strings.HasPrefix(line, "OK ") {
		conn.Close()
		return nil, fmt.Errorf("vsock CONNECT failed: %s", strings.TrimSpace(line))
	}

	return conn, nil
}

// VsockRequest is the JSON request sent from the host to the VM runner daemon.
type VsockRequest struct {
	Code          string `json:"code"`
	ShowTables    bool   `json:"show_tables"`
	ShowTableMeta bool   `json:"show_table_meta"`
}

// VsockResponse is the JSON response from the VM runner daemon.
type VsockResponse struct {
	ExitCode   int            `json:"exit_code"`
	Stdout     string         `json:"stdout"`
	Stderr     string         `json:"stderr"`
	ResultRepr *string        `json:"result_repr"`
	Error      *string        `json:"error"`
	Tables     []any          `json:"tables"`
	Timing     map[string]any `json:"_timing,omitempty"`
}

// ExecuteViaVsock sends a code execution request to the VM runner daemon over
// vsock and returns the response. The daemon inside the VM has a pre-connected
// pydeephaven Session, so this avoids all host-side Python overhead.
func ExecuteViaVsock(vsockPath string, port uint32, req *VsockRequest) (*VsockResponse, error) {
	conn, err := connectVsock(vsockPath, port)
	if err != nil {
		return nil, fmt.Errorf("connecting to VM runner: %w", err)
	}
	defer conn.Close()

	// Set deadline for the entire operation
	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	// Send request as a single JSON line
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	reqBytes = append(reqBytes, '\n')
	if _, err := conn.Write(reqBytes); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	// Read response (single JSON line terminated by newline)
	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var resp VsockResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}

// copyFile copies src to dst.
// punchHoles scans a file for zero-filled regions and converts them into
// filesystem holes using fallocate(FALLOC_FL_PUNCH_HOLE). This makes the
// file sparse so that SEEK_HOLE/SEEK_DATA can identify zero vs data regions.
func punchHoles(path string, stderr io.Writer) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	fileSize := fi.Size()

	// Read in 1 MiB chunks — large enough to amortize syscall overhead,
	// small enough to detect zero regions at MiB granularity.
	const chunkSize = 1024 * 1024
	buf := make([]byte, chunkSize)

	var holesBytes int64
	for offset := int64(0); offset < fileSize; offset += chunkSize {
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return fmt.Errorf("reading at offset %d: %w", offset, err)
		}
		if n == 0 {
			break
		}

		if isZero(buf[:n]) {
			// Punch a hole — converts this range to a filesystem hole
			err := unix.Fallocate(int(f.Fd()),
				unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE,
				offset, int64(n))
			if err != nil {
				return fmt.Errorf("fallocate punch hole at %d: %w", offset, err)
			}
			holesBytes += int64(n)
		}
	}

	fmt.Fprintf(stderr, "Punched holes: %d MiB of %d MiB are now sparse\n",
		holesBytes/(1024*1024), fileSize/(1024*1024))
	return nil
}

// isZero checks if a byte slice is entirely zeros.
func isZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// WarmSnapshotPageCacheAsync starts a background goroutine that reads the
// snapshot memory file's data extents into the kernel page cache.
// Called from exec as early as possible to maximize overlap with other work.
func WarmSnapshotPageCacheAsync(paths *VMPaths, version string) {
	memPath := filepath.Join(paths.SnapshotDirForVersion(version), "snapshot_mem")
	go warmSnapshotPageCache(memPath)
}

// warmSnapshotPageCache reads data extents of a snapshot memory file into the
// kernel page cache using parallel readers. On SSDs/NVMe, parallel reads
// saturate the device's I/O bandwidth better than a single sequential reader.
func warmSnapshotPageCache(memPath string) {
	f, err := os.Open(memPath)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() == 0 {
		return
	}
	fd := int(f.Fd())
	fileSize := fi.Size()

	// Hint sequential access + trigger kernel readahead
	unix.Fadvise(fd, 0, fileSize, unix.FADV_SEQUENTIAL)

	// Scan data extents (fast SEEK_HOLE/SEEK_DATA)
	extents, err := scanDataExtents(f, 0, uint64(fileSize))
	if err != nil {
		unix.Fadvise(fd, 0, fileSize, unix.FADV_WILLNEED)
		return
	}

	// FADV_WILLNEED for all extents first (non-blocking, starts kernel readahead)
	for _, ext := range extents {
		unix.Fadvise(fd, int64(ext.offset), int64(ext.length), unix.FADV_WILLNEED)
	}

	// Parallel explicit reads to guarantee page cache population.
	// 4 readers matches typical SSD queue depth for optimal throughput.
	const numReaders = 4
	if len(extents) == 0 {
		return
	}

	// Split extents across readers by total bytes (not count) for balance
	var totalBytes uint64
	for _, ext := range extents {
		totalBytes += ext.length
	}
	bytesPerReader := totalBytes / numReaders

	var wg sync.WaitGroup
	extIdx := 0
	for r := 0; r < numReaders && extIdx < len(extents); r++ {
		// Collect extents for this reader
		var readerExtents []dataExtent
		var readerBytes uint64
		for extIdx < len(extents) {
			readerExtents = append(readerExtents, extents[extIdx])
			readerBytes += extents[extIdx].length
			extIdx++
			if r < numReaders-1 && readerBytes >= bytesPerReader {
				break
			}
		}

		wg.Add(1)
		go func(exts []dataExtent) {
			defer wg.Done()
			// Each reader opens its own fd for independent I/O scheduling
			rf, err := os.Open(memPath)
			if err != nil {
				return
			}
			defer rf.Close()
			buf := make([]byte, 1024*1024)
			for _, ext := range exts {
				for off := ext.offset; off < ext.offset+ext.length; off += uint64(len(buf)) {
					readLen := ext.offset + ext.length - off
					if readLen > uint64(len(buf)) {
						readLen = uint64(len(buf))
					}
					rf.ReadAt(buf[:readLen], int64(off))
				}
			}
		}(readerExtents)
	}
	wg.Wait()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
