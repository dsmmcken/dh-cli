//go:build linux

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
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

// copyChunkSize is the size of each UFFDIO_COPY request. 128MB chunks balance
// ioctl count vs memory bandwidth utilization for parallel copy goroutines.
// Used only in eager mode (DH_VM_EAGER_UFFD=1).
const copyChunkSize = 128 * 1024 * 1024

// copyWorkers is the number of parallel goroutines for eager UFFDIO_COPY.
// Used only in eager mode (DH_VM_EAGER_UFFD=1).
const copyWorkers = 4

// lazyChunkSize is the alignment/size for lazy UFFDIO_COPY responses.
// 2 MiB chunks reduce fault count by ~512x vs 4KB pages while matching
// the hugepage boundary for efficient memcpy.
const lazyChunkSize = 2 * 1024 * 1024

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

// dataExtent describes a contiguous range of non-hole data in the snapshot file.
type dataExtent struct {
	offset uint64 // offset in file
	length uint64 // length of data region
}

// regionInfo pairs a memory region with its data/hole extent map for the lazy handler.
type regionInfo struct {
	region  memRegion
	extents []dataExtent // sorted by offset
}

// copyJob is a unit of work for the parallel eager copy pool.
type copyJob struct {
	uffdFd  int
	dst     uint64 // destination in VM address space
	src     uint64 // source in our mmap
	length  uint64
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

// uffdHandler manages the UFFD lifecycle. In lazy mode (default), all page
// faults are served on demand with 2MB-aligned UFFDIO_COPY from a pre-cached
// mmap. In eager mode (DH_VM_EAGER_UFFD=1), data pages are bulk-copied
// before VM resume. The snapshot file is pre-loaded into the page cache to
// minimize I/O latency in both modes.
type uffdHandler struct {
	socketPath string
	memFile    string
	listener   *net.UnixListener
	uffdFd     int       // kept open for VM lifetime; -1 if not yet received
	done       chan error // signaled when population setup completes (nil = success)
	cancel     context.CancelFunc

	// Pre-loaded file data (available before Firecracker connects)
	file     *os.File
	fileSize uint64
	mmapData []byte
	mmapBase uintptr

	// Pre-scanned data extents (computed during preload, used by doPopulate)
	preExtents []dataExtent // sorted by offset, covering the whole file
	preSparse  bool         // true if sparse scanning succeeded
	preWarm    chan struct{} // closed when background page cache warming finishes

	// Lazy fault tracking: which 2MB-aligned chunks have been populated.
	// Protected by lazyMu. Only used in lazy mode.
	lazyMu          sync.Mutex
	populatedChunks map[uint64]struct{}
}

// startUffdHandler creates a UDS listener, pre-loads the snapshot file into
// the page cache, and spawns a goroutine that handles UFFD population.
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

	// Pre-load: open file, mmap (no MAP_POPULATE), trigger async readahead.
	// This overlaps disk I/O with Firecracker's startup (~150ms), so the file
	// is already partially or fully in the page cache when UFFDIO_COPY starts.
	if err := h.preload(); err != nil {
		listener.Close()
		cancel()
		return nil, fmt.Errorf("pre-loading snapshot file: %w", err)
	}

	go h.run(ctx, stderr)
	return h, nil
}

// preload opens the snapshot memory file, creates a read-only mmap, pre-scans
// data extents, and starts warming the page cache — all before Firecracker
// connects. This overlaps ~150ms of I/O with Firecracker's launch time.
func (h *uffdHandler) preload() error {
	f, err := os.Open(h.memFile)
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat: %w", err)
	}

	h.file = f
	h.fileSize = uint64(fi.Size())

	// mmap without MAP_POPULATE — returns immediately, no blocking I/O.
	data, err := unix.Mmap(int(f.Fd()), 0, int(h.fileSize), unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		f.Close()
		return fmt.Errorf("mmap: %w", err)
	}
	h.mmapData = data
	h.mmapBase = uintptr(unsafe.Pointer(&data[0]))

	// Request transparent huge pages for the mmap — reduces TLB misses during
	// UFFDIO_COPY by using 2MB pages instead of 4KB for the source data.
	unix.Madvise(data, unix.MADV_HUGEPAGE)

	// Trigger non-blocking readahead for the whole file.
	unix.Fadvise(int(f.Fd()), 0, int64(h.fileSize), unix.FADV_SEQUENTIAL)
	unix.Madvise(data, unix.MADV_WILLNEED)

	// Pre-scan data extents for the whole file (fast SEEK_HOLE/SEEK_DATA).
	// This runs now instead of after FC connects, saving ~20-40ms.
	extents, err := scanDataExtents(f, 0, h.fileSize)
	if err == nil {
		h.preExtents = extents
		h.preSparse = true
	}

	// Start a background goroutine that forces data pages into page cache by
	// reading them sequentially. This runs concurrently with FC launch so the
	// file is warm by the time UFFDIO_COPY starts.
	h.preWarm = make(chan struct{})
	go func() {
		defer close(h.preWarm)
		// Read only data extents (skip holes) to minimize I/O
		if h.preSparse && len(h.preExtents) > 0 {
			buf := make([]byte, 1024*1024) // 1MB read buffer
			for _, ext := range h.preExtents {
				for off := ext.offset; off < ext.offset+ext.length; off += uint64(len(buf)) {
					readLen := ext.offset + ext.length - off
					if readLen > uint64(len(buf)) {
						readLen = uint64(len(buf))
					}
					f.ReadAt(buf[:readLen], int64(off))
				}
			}
		} else {
			// Non-sparse: read entire file
			buf := make([]byte, 1024*1024)
			for off := int64(0); off < int64(h.fileSize); off += int64(len(buf)) {
				f.ReadAt(buf, off)
			}
		}
	}()

	return nil
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

// Close cleans up the UFFD handler: closes the fd, munmaps, and removes the socket.
func (h *uffdHandler) Close() error {
	h.cancel()
	if h.uffdFd >= 0 {
		unix.Close(h.uffdFd)
		h.uffdFd = -1
	}
	if h.mmapData != nil {
		unix.Munmap(h.mmapData)
		h.mmapData = nil
	}
	if h.file != nil {
		h.file.Close()
		h.file = nil
	}
	h.listener.Close()
	os.Remove(h.socketPath)
	return nil
}

// run is the main handler goroutine.
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

	// Use pre-scanned extents from preload (already computed during FC launch).
	// Map the whole-file extents to per-region extents by clipping.
	var regionInfos []regionInfo
	sparse := h.preSparse
	for _, region := range regions {
		var extents []dataExtent
		if sparse {
			extents = clipExtentsToRegion(h.preExtents, region.Offset, region.Size)
		} else {
			// Non-sparse fallback: treat entire region as data
			extents = []dataExtent{{offset: region.Offset, length: region.Size}}
		}
		regionInfos = append(regionInfos, regionInfo{
			region:  region,
			extents: extents,
		})
	}

	// Eager mode: pre-copy all data pages before VM resume (old behavior).
	if os.Getenv("DH_VM_EAGER_UFFD") == "1" {
		<-h.preWarm

		var jobs []copyJob
		for _, ri := range regionInfos {
			base := ri.region.BaseHostVirtAddr
			regionStart := ri.region.Offset
			for _, ext := range ri.extents {
				extEnd := ext.offset + ext.length
				if extEnd > ri.region.Offset+ri.region.Size {
					extEnd = ri.region.Offset + ri.region.Size
				}
				for off := ext.offset; off < extEnd; off += copyChunkSize {
					chunkLen := uint64(copyChunkSize)
					if remaining := extEnd - off; remaining < chunkLen {
						chunkLen = remaining
					}
					jobs = append(jobs, copyJob{
						uffdFd: uffdFd,
						dst:    base + (off - regionStart),
						src:    uint64(h.mmapBase) + off,
						length: chunkLen,
					})
				}
			}
		}

		if err := parallelCopy(jobs, copyWorkers); err != nil {
			return fmt.Errorf("parallel UFFDIO_COPY: %w", err)
		}

		if sparse {
			go h.lazyFaultHandler(ctx, uffdFd, regionInfos)
		}
		return nil
	}

	// Hybrid mode (default): pre-copy the first N MB of data extents to avoid
	// cold-start page faults on the critical path (Python interpreter, JVM code
	// cache, kernel), then serve remaining faults lazily with 2MB chunks.
	eagerPreloadBytes := uint64(256 * 1024 * 1024) // default 256MB
	if v := os.Getenv("DH_VM_EAGER_MB"); v != "" {
		if mb, err := strconv.ParseUint(v, 10, 64); err == nil {
			eagerPreloadBytes = mb * 1024 * 1024
		}
	}

	h.populatedChunks = make(map[uint64]struct{})

	if eagerPreloadBytes > 0 {
		// Wait for page cache warming before UFFDIO_COPY
		<-h.preWarm

		// Build eager copy jobs for first N MB of data extents.
		// Hot pages (kernel, Python, JVM) tend to be at low file offsets
		// since they're loaded early during boot.
		var eagerJobs []copyJob
		var eagerTotal uint64
		for _, ri := range regionInfos {
			if eagerTotal >= eagerPreloadBytes {
				break
			}
			base := ri.region.BaseHostVirtAddr
			regionOff := ri.region.Offset

			for _, ext := range ri.extents {
				if eagerTotal >= eagerPreloadBytes {
					break
				}
				// ext.offset is absolute file offset; convert to region-relative
				relStart := ext.offset - regionOff
				relEnd := relStart + ext.length
				if relEnd > ri.region.Size {
					relEnd = ri.region.Size
				}

				firstChunk := (relStart / lazyChunkSize) * lazyChunkSize
				for chunkOff := firstChunk; chunkOff < relEnd && eagerTotal < eagerPreloadBytes; chunkOff += lazyChunkSize {
					chunkKey := base + chunkOff
					if _, ok := h.populatedChunks[chunkKey]; ok {
						continue
					}

					chunkEnd := chunkOff + lazyChunkSize
					if chunkEnd > ri.region.Size {
						chunkEnd = ri.region.Size
					}

					h.populatedChunks[chunkKey] = struct{}{}
					eagerJobs = append(eagerJobs, copyJob{
						uffdFd: uffdFd,
						dst:    base + chunkOff,
						src:    uint64(h.mmapBase) + regionOff + chunkOff,
						length: chunkEnd - chunkOff,
					})
					eagerTotal += chunkEnd - chunkOff
				}
			}
		}

		if len(eagerJobs) > 0 {
			if err := parallelCopy(eagerJobs, copyWorkers); err != nil {
				return fmt.Errorf("hybrid eager UFFDIO_COPY: %w", err)
			}
		}
	}

	// Start parallel lazy handler for remaining faults
	go h.lazyFaultHandlerV2(ctx, uffdFd, regionInfos)
	return nil
}

// clipExtentsToRegion returns the subset of whole-file data extents that
// overlap with a given region [regionOffset, regionOffset+regionSize).
func clipExtentsToRegion(allExtents []dataExtent, regionOffset, regionSize uint64) []dataExtent {
	regionEnd := regionOffset + regionSize
	var result []dataExtent
	for _, ext := range allExtents {
		extEnd := ext.offset + ext.length
		// Skip extents entirely before or after the region
		if extEnd <= regionOffset || ext.offset >= regionEnd {
			continue
		}
		// Clip to region bounds
		start := ext.offset
		if start < regionOffset {
			start = regionOffset
		}
		end := extEnd
		if end > regionEnd {
			end = regionEnd
		}
		result = append(result, dataExtent{offset: start, length: end - start})
	}
	return result
}

// parallelCopy distributes UFFDIO_COPY jobs across n worker goroutines.
func parallelCopy(jobs []copyJob, workers int) error {
	if len(jobs) == 0 {
		return nil
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	jobCh := make(chan copyJob, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				cp := ufffdioCopy{
					dst:  job.dst,
					src:  job.src,
					len:  job.length,
					mode: 0,
				}
				_, _, errno := unix.Syscall(
					unix.SYS_IOCTL,
					uintptr(job.uffdFd),
					uintptr(_UFFDIO_COPY),
					uintptr(unsafe.Pointer(&cp)),
				)
				if errno != 0 {
					errCh <- fmt.Errorf("UFFDIO_COPY errno %v", errno)
					return
				}
				if cp.copy < 0 {
					errCh <- fmt.Errorf("UFFDIO_COPY returned %d", cp.copy)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	// Return first error if any
	for err := range errCh {
		return err
	}
	return nil
}

// lazyFaultHandler serves page faults on hole pages with UFFDIO_ZEROPAGE.
// Runs until the context is cancelled (VM destroyed).
func (h *uffdHandler) lazyFaultHandler(ctx context.Context, uffdFd int, regions []regionInfo) {
	const maxBatch = 16
	var buf [uffdMsgSize * maxBatch]byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fds := []unix.PollFd{{
			Fd:     int32(uffdFd),
			Events: unix.POLLIN,
		}}
		n, err := unix.Poll(fds, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		nr, err := unix.Read(uffdFd, buf[:])
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				continue
			}
			return
		}

		numMsgs := nr / uffdMsgSize
		for i := 0; i < numMsgs; i++ {
			msg := buf[i*uffdMsgSize : (i+1)*uffdMsgSize]
			event := msg[0]

			switch event {
			case _UFFD_EVENT_PAGEFAULT:
				faultAddr := *(*uint64)(unsafe.Pointer(&msg[16]))
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
				// Balloon deflation — no action needed
			}
		}
	}
}

// lazyFaultHandlerV2 serves ALL page faults lazily using 2MB-aligned
// UFFDIO_COPY from the pre-cached mmap. Both data and hole pages are served
// this way — the mmap reads zeros for hole regions, so UFFDIO_COPY produces
// the same result as UFFDIO_ZEROPAGE but with a unified code path.
//
// Faults are dispatched to a pool of 4 worker goroutines so that multiple
// vCPUs can have their faults served in parallel (the populatedChunks mutex
// already provides thread safety).
func (h *uffdHandler) lazyFaultHandlerV2(ctx context.Context, uffdFd int, regions []regionInfo) {
	const maxBatch = 16
	var buf [uffdMsgSize * maxBatch]byte

	// Dispatch faults to a worker pool for parallel handling.
	// 4 workers gives headroom beyond DefaultVCPUCount=2.
	faultCh := make(chan uint64, 64)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for faultAddr := range faultCh {
				h.handleLazyFault(uffdFd, faultAddr, regions)
			}
		}()
	}
	defer func() {
		close(faultCh)
		wg.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fds := []unix.PollFd{{
			Fd:     int32(uffdFd),
			Events: unix.POLLIN,
		}}
		n, err := unix.Poll(fds, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		nr, err := unix.Read(uffdFd, buf[:])
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				continue
			}
			return
		}

		numMsgs := nr / uffdMsgSize
		for i := 0; i < numMsgs; i++ {
			msg := buf[i*uffdMsgSize : (i+1)*uffdMsgSize]
			event := msg[0]

			switch event {
			case _UFFD_EVENT_PAGEFAULT:
				faultAddr := *(*uint64)(unsafe.Pointer(&msg[16]))
				faultCh <- faultAddr

			case _UFFD_EVENT_REMOVE:
				// Balloon deflation — no action needed
			}
		}
	}
}

// handleLazyFault resolves a single page fault by UFFDIO_COPY'ing a 2MB-aligned
// chunk from the pre-cached mmap into the VM's address space.
func (h *uffdHandler) handleLazyFault(uffdFd int, faultAddr uint64, regions []regionInfo) {
	// Find which region contains the fault address
	for _, ri := range regions {
		base := ri.region.BaseHostVirtAddr
		regionEnd := base + ri.region.Size
		if faultAddr < base || faultAddr >= regionEnd {
			continue
		}

		// Compute 2MB-aligned chunk within this region
		offsetInRegion := faultAddr - base
		chunkStart := (offsetInRegion / lazyChunkSize) * lazyChunkSize
		chunkEnd := chunkStart + lazyChunkSize
		if chunkEnd > ri.region.Size {
			chunkEnd = ri.region.Size
		}
		chunkLen := chunkEnd - chunkStart

		// De-duplicate: skip if this chunk was already populated
		chunkKey := base + chunkStart
		h.lazyMu.Lock()
		if _, ok := h.populatedChunks[chunkKey]; ok {
			h.lazyMu.Unlock()
			return
		}
		h.populatedChunks[chunkKey] = struct{}{}
		h.lazyMu.Unlock()

		// UFFDIO_COPY from mmap — works for both data and holes since the
		// mmap reads zeros for sparse hole regions.
		fileOffset := ri.region.Offset + chunkStart
		cp := ufffdioCopy{
			dst:  base + chunkStart,
			src:  uint64(h.mmapBase) + fileOffset,
			len:  chunkLen,
			mode: 0,
		}
		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(uffdFd),
			uintptr(_UFFDIO_COPY),
			uintptr(unsafe.Pointer(&cp)),
		)
		if errno != 0 && errno != unix.EEXIST {
			// EEXIST is benign (race with another fault in same range).
			// Other errors: log but don't crash — faulting thread retries.
			return
		}
		return
	}

	// Fault address not in any region — shouldn't happen. Unblock with a
	// single zero page to prevent the VM from hanging.
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
}

// isInDataExtent checks if a file offset falls within any data extent.
// Extents must be sorted by offset. Uses binary search for O(log n) lookup.
func isInDataExtent(offset uint64, extents []dataExtent) bool {
	i := sort.Search(len(extents), func(i int) bool {
		return extents[i].offset+extents[i].length > offset
	})
	return i < len(extents) && offset >= extents[i].offset
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

// receiveUffdAndRegions receives the UFFD file descriptor (via SCM_RIGHTS) and
// the JSON memory region layout from Firecracker over the Unix socket.
func receiveUffdAndRegions(conn *net.UnixConn) (int, []memRegion, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return -1, nil, fmt.Errorf("getting raw conn: %w", err)
	}

	buf := make([]byte, 64*1024)           // JSON payload buffer
	oob := make([]byte, unix.CmsgSpace(4)) // space for 1 fd (4 bytes)
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
