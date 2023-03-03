package main

import (
	"fmt"
	"github.com/levmv/go-resizer/vips"
	"io"
	"log"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

var (
	totalRequests      uint64
	requestsInProgress int32
)

func IncTotalRequests() {
	atomic.AddUint64(&totalRequests, 1)
}

func IncRequestsInProgress() {
	atomic.AddInt32(&requestsInProgress, 1)
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
	TotalRequests uint64
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
		TotalRequests: TotalRequests(),
		ReqInProgress: RequestsInProgress(),
	}

	curStats.GoStat.NumGoroutine = runtime.NumGoroutine()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	curStats.GoStat.Alloc = HumanSize(m.Alloc)
	curStats.GoStat.MemTotal = HumanSize(m.TotalAlloc)
	curStats.GoStat.MemSys = HumanSize(m.Sys)
	curStats.GoStat.LiveObjects = m.Mallocs - m.Frees

	// GC Stats
	curStats.GoStat.GcPauseTotal = fmt.Sprintf("%.2fs", float64(m.PauseTotalNs)/1000/1000/1000)
	curStats.GoStat.NumGC = m.NumGC
	curStats.GoStat.LastGC = fmt.Sprintf("%.2fs", float64(time.Now().UnixNano()-int64(m.LastGC))/1000/1000/1000)
	curStats.GoStat.NextGC = HumanSize(uint64(m.NextGC))

	vips.ReadVipsMemStats(&curStats.VipsMemStats)

	return curStats
}

func showStats(config string) error {
	cfg, err := ParseConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := http.Get("http://" + cfg.BindTo + "/stat")
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
