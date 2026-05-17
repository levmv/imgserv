//go:build memtest

package vips

import (
	"fmt"
	"runtime"
	"testing"
)

func TestMemorySoakResizeExport(t *testing.T) {
	const (
		warmupIterations = 200
		testIterations   = 2000
		maxGoHeapGrowth  = 8 << 20
		maxVipsGrowth    = 8 << 20
	)

	inputs := [][]byte{
		rgbJPEG(t, 1600, 900),
		transparentPNG(t),
		jpegWithOrientation(t, 6),
	}

	for i := 0; i < warmupIterations; i++ {
		if err := resizeExportCycle(inputs[i%len(inputs)]); err != nil {
			t.Fatalf("warmup iteration %d failed: %v", i, err)
		}
	}

	before := memorySnapshot()

	for i := 0; i < testIterations; i++ {
		if err := resizeExportCycle(inputs[i%len(inputs)]); err != nil {
			t.Fatalf("test iteration %d failed: %v", i, err)
		}
	}

	after := memorySnapshot()
	t.Logf("memory before: %s", before)
	t.Logf("memory after:  %s", after)

	if growth := uint64Growth(before.goHeapAlloc, after.goHeapAlloc); growth > maxGoHeapGrowth {
		t.Fatalf("Go heap grew by %s, threshold %s", byteSize(growth), byteSize(maxGoHeapGrowth))
	}
	if growth := after.vipsMem - before.vipsMem; growth > maxVipsGrowth {
		t.Fatalf("tracked vips memory grew by %s, threshold %s", byteSize(uint64(growth)), byteSize(maxVipsGrowth))
	}
	if after.vipsFiles > before.vipsFiles {
		t.Fatalf("tracked vips files grew from %d to %d", before.vipsFiles, after.vipsFiles)
	}
}

func resizeExportCycle(input []byte) error {
	var img Image
	defer img.Close()
	defer Cleanup()

	if err := img.LoadFromBuffer(input); err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if err := img.AutoRotate(); err != nil {
		return fmt.Errorf("autorotate: %w", err)
	}
	if err := img.Thumbnail(320, 240, InterestingCentre, SizeDown); err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}
	if err := img.Strip(); err != nil {
		return fmt.Errorf("strip: %w", err)
	}
	if _, err := img.ExportJpeg(85); err != nil {
		return fmt.Errorf("export jpeg: %w", err)
	}
	return nil
}

type memSnapshot struct {
	goHeapAlloc uint64
	goHeapSys   uint64
	vipsMem     int64
	vipsAllocs  int64
	vipsFiles   int64
}

func memorySnapshot() memSnapshot {
	runtime.GC()
	runtime.GC()

	var goMem runtime.MemStats
	runtime.ReadMemStats(&goMem)

	var vipsMem MemoryStats
	ReadVipsMemStats(&vipsMem)

	return memSnapshot{
		goHeapAlloc: goMem.HeapAlloc,
		goHeapSys:   goMem.HeapSys,
		vipsMem:     vipsMem.Mem,
		vipsAllocs:  vipsMem.Allocs,
		vipsFiles:   vipsMem.Files,
	}
}

func (m memSnapshot) String() string {
	return fmt.Sprintf("go_heap_alloc=%s go_heap_sys=%s vips_mem=%s vips_allocs=%d vips_files=%d",
		byteSize(m.goHeapAlloc),
		byteSize(m.goHeapSys),
		byteSize(uint64(max64(0, m.vipsMem))),
		m.vipsAllocs,
		m.vipsFiles,
	)
}

func byteSize(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for n/div >= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func max64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func uint64Growth(before uint64, after uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}
