package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/params"
	"github.com/levmv/imgserv/storage"
	"github.com/levmv/imgserv/vips"
	"golang.org/x/sync/semaphore"
)

const version = "0.1.3"

const help = `usage: imgserv <action>
actions:
  server -config=<path to config.json>
  stat [-config=<path to config.json>]
  version`

var (
	maxSem     *semaphore.Weighted
	queueSem   *semaphore.Weighted
	sign       UrlSignature
	imgStorage *storage.Cached
	cfg        *config.Config
)

func run(cfgPath string) error {

	var err error

	cfg, err = config.Parse(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Resizer.Presets != nil {
		if err = params.InitPresets(string(cfg.Resizer.Presets)); err != nil {
			log.Fatal(err)
		}
	}
	if cfg.Server.MemoryLimit > 0 {
		debug.SetMemoryLimit(cfg.Server.MemoryLimit)
	}

	if cfg.Server.LogFile != "" {
		file, err := os.OpenFile(cfg.Server.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(file)
	}

	sign = NewUrlSignature(cfg.Resizer.SignatureMethod, cfg.Resizer.SignatureSecret)

	if err = vips.Init(nil); err != nil {
		log.Fatal(err)
	}
	defer vips.Shutdown()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())

	imgStorage, err = storage.NewCached(cfg.Storage)
	if err != nil {
		log.Fatalf("Fail to init storage: %v", err)
	}

	initUpload(cfg.Storage)
	if cfg.Sharer != nil {
		if err = initSharer(ctx, cfg.Sharer); err != nil {
			log.Fatalf("Fail to init sharer: %v", err)
		}
	}

	startServer(cancel, cfg.Server)

	select {
	case <-ctx.Done():
	case <-stop:
	}
	return nil
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(help)
		os.Exit(0)
	}

	var configArg string
	serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
	serverCmd.StringVar(&configArg, "config", "./config.json", "path to config json file")
	serverCmd.StringVar(&configArg, "c", "./config.json", "path to config json file (shorthand)")

	action := os.Args[1]

	switch action {
	case "version":
		fmt.Println(version)
	case "server":
		serverCmd.Parse(os.Args[2:])
		if err := run(configArg); err != nil {
			log.Fatal(err)
		}
	case "stat":
		serverCmd.Parse(os.Args[2:])
		if err := showStats(configArg); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("expected 'server', `stat` or 'version'")
		os.Exit(1)
	}
}
