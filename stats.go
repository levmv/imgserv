package main

import (
	"fmt"
	config2 "github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/vips"
	"io"
	"log"
	"math"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

var (
	resizerRequests    uint64
	sharerRequests     uint64
	uploaderRequests   uint64
	totalRequests      uint64
	rejectedRequests   uint64
	requestsInProgress int32
)

func IncResizerRequests() {
	atomic.AddUint64(&resizerRequests, 1)
}

func IncSharerRequests() {
	atomic.AddUint64(&sharerRequests, 1)
}

func IncUploaderRequests() {
	atomic.AddUint64(&uploaderRequests, 1)
}

func IncTotalRequests() {
	atomic.AddUint64(&totalRequests, 1)
}

func IncRejectedRequests() {
	atomic.AddUint64(&rejectedRequests, 1)
}

func IncRequestsInProgress() int32 {
	return atomic.AddInt32(&requestsInProgress, 1)
}

func DecRequestsInProgress() {
	atomic.AddInt32(&requestsInProgress, -1)
}

func TotalRequests() uint64 {
	return atomic.LoadUint64(&totalRequests)
}

func RequestsInProgress() int32 {
	return atomic.LoadInt32(&requestsInProgress)
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
		Resized:       resizerRequests,
		Shares:        sharerRequests,
		Uploaded:      uploaderRequests,
		Rejected:      rejectedRequests,
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

func showStats(config string) error {
	cfg, err := config2.Parse(config)
	if err != nil {
		log.Fatal(err)
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
