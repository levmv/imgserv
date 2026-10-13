package main

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/vips"
)

var (
	resizerRequests    atomic.Uint64
	sharerRequests     atomic.Uint64
	uploaderRequests   atomic.Uint64
	rejectedRequests   atomic.Uint64
	requestsInProgress atomic.Int32
)

func IncResizerRequests() {
	resizerRequests.Add(1)
}

func IncSharerRequests() {
	sharerRequests.Add(1)
}

func IncUploaderRequests() {
	uploaderRequests.Add(1)
}

func IncRejectedRequests() {
	rejectedRequests.Add(1)
}

func IncRequestsInProgress() int32 {
	return requestsInProgress.Add(1)
}

func DecRequestsInProgress() {
	requestsInProgress.Add(-1)
}

func RequestsInProgress() int32 {
	return requestsInProgress.Load()
}

type stats struct {
	Resized       uint64
	Shares        uint64
	Uploaded      uint64
	Rejected      uint64
	ReqInProgress int32
	GoStat        struct {
		LiveObjects uint64

		GcPauseTotal,
		Alloc,
		MemTotal,
		MemSys string

		NumGC        uint32
		NextGC       string
		LastGC       string
		NumGoroutine int
	}
	VipsMemStats vips.MemoryStats
}

func newStats() stats {

	curStats := stats{
		Resized:       resizerRequests.Load(),
		Shares:        sharerRequests.Load(),
		Uploaded:      uploaderRequests.Load(),
		Rejected:      rejectedRequests.Load(),
		ReqInProgress: RequestsInProgress(),
	}

	curStats.GoStat.NumGoroutine = runtime.NumGoroutine()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	curStats.GoStat.Alloc = humanSize(m.Alloc)
	curStats.GoStat.MemTotal = humanSize(m.TotalAlloc)
	curStats.GoStat.MemSys = humanSize(m.Sys)
	curStats.GoStat.LiveObjects = m.Mallocs - m.Frees

	// GC Stats
	curStats.GoStat.GcPauseTotal = fmt.Sprintf("%.2fs", float64(m.PauseTotalNs)/1000/1000/1000)
	curStats.GoStat.NumGC = m.NumGC
	curStats.GoStat.LastGC = fmt.Sprintf("%.2fs", float64(time.Now().UnixNano()-int64(m.LastGC))/1000/1000/1000)
	curStats.GoStat.NextGC = humanSize(uint64(m.NextGC))

	vips.ReadVipsMemStats(&curStats.VipsMemStats)

	return curStats
}

func showStats(configPath string) error {
	cfg, err := config.Parse(configPath)
	if err != nil {
		return err
	}

	resp, err := http.Get("http://" + cfg.Server.BindTo + "/stat")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	str, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("%s", str)

	return nil
}

func humanateBytes(s uint64, base float64, sizes []string) string {
	if s < 10 {
		return fmt.Sprintf("%d B", s)
	}
	e := math.Floor(math.Log(float64(s)) / math.Log(base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	f := "%.0f %s"
	if val < 10 {
		f = "%.1f %s"
	}

	return fmt.Sprintf(f, val, suffix)
}

func humanSize(s uint64) string {
	sizes := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	return humanateBytes(s, 1024, sizes)
}
