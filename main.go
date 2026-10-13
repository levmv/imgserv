package main

import (
	"context"
	"errors"
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

var version = "dev"

const help = `usage: imgserv <action> [options]

actions:
  server -config=<path>   run HTTP server
  stat [-config=<path>]   fetch server stats
  version                 print version

Run "imgserv <action> -h" for action options.`

var (
	maxSem     *semaphore.Weighted
	queueSem   *semaphore.Weighted
	sign       UrlSignature
	imgStorage storage.ImageStorage
	cfg        *config.Config
)

func run(cfgPath string, overrides config.Overrides) error {

	var err error

	cfg, err = config.ParseWithOverrides(cfgPath, overrides)
	if err != nil {
		return err
	}

	if cfg.Resizer.Presets != nil {
		if err = params.InitPresets(string(cfg.Resizer.Presets)); err != nil {
			return err
		}
	}
	if cfg.Server.MemoryLimit > 0 {
		debug.SetMemoryLimit(cfg.Server.MemoryLimit)
	}

	if cfg.Server.LogFile != "" {
		file, err := os.OpenFile(cfg.Server.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			return err
		}
		log.SetOutput(file)
	}

	sign = NewUrlSignature(cfg.Resizer.SignatureMethod, cfg.Resizer.SignatureSecret)

	if err = vips.Init(nil); err != nil {
		return err
	}
	defer vips.Shutdown()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	imgStorage, err = storage.New(cfg.Storage)
	if err != nil {
		return fmt.Errorf("init storage: %w", err)
	}

	initUpload(cfg.Storage)
	if cfg.Sharer != nil {
		if err = initSharer(ctx, cfg.Sharer); err != nil {
			return fmt.Errorf("init sharer: %w", err)
		}
	}

	server, serverErrors := startServer(ctx, cfg.Server)

	select {
	case err = <-serverErrors:
	case <-stop:
	}
	// Keep libvips alive until every handler finishes; a second signal forces exit.
	signal.Stop(stop)
	return errors.Join(err, server.Shutdown(context.Background()))
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(help)
		os.Exit(0)
	}

	var serverConfigArg string
	var serverOverrides config.Overrides
	serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
	serverCmd.StringVar(&serverConfigArg, "config", "./config.json", "path to config json file")
	serverCmd.StringVar(&serverConfigArg, "c", "./config.json", "path to config json file (shorthand)")
	serverCmd.StringVar(&serverOverrides.BindTo, "bind", "", "override server.bind_to")
	serverCmd.StringVar(&serverOverrides.StorageType, "storage-type", "", "override storage.type")
	serverCmd.StringVar(&serverOverrides.StorageLocalPath, "storage-local-path", "", "override storage.local_path")
	serverCmd.StringVar(&serverOverrides.StorageCachePath, "storage-cache-path", "", "override storage.cache_path")
	serverCmd.Usage = func() {
		fmt.Fprintf(serverCmd.Output(), "usage: imgserv server -config=<path> [storage overrides]\n\noptions:\n")
		serverCmd.PrintDefaults()
	}

	var statConfigArg string
	statCmd := flag.NewFlagSet("stat", flag.ExitOnError)
	statCmd.StringVar(&statConfigArg, "config", "./config.json", "path to config json file")
	statCmd.StringVar(&statConfigArg, "c", "./config.json", "path to config json file (shorthand)")
	statCmd.Usage = func() {
		fmt.Fprintf(statCmd.Output(), "usage: imgserv stat [-config=<path>]\n\noptions:\n")
		statCmd.PrintDefaults()
	}

	action := os.Args[1]

	switch action {
	case "version":
		fmt.Println(version)
	case "server":
		serverCmd.Parse(os.Args[2:])
		if err := run(serverConfigArg, serverOverrides); err != nil {
			log.Fatal(err)
		}
	case "stat":
		statCmd.Parse(os.Args[2:])
		if err := showStats(statConfigArg); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("expected 'server', `stat` or 'version'")
		os.Exit(1)
	}
}
