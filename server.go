package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/storage"
	"golang.org/x/sync/semaphore"
)

type appHandler func(http.ResponseWriter, *http.Request) (int, error)

func serveStat(w http.ResponseWriter, r *http.Request) (int, error) {
	var curStats = newStats()

	out, err := json.MarshalIndent(curStats, "", "    ")
	if err != nil {
		return 500, err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)

	return 200, nil
}

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, err := fn(w, r); err != nil {
		if status == 499 || isClientClosedError(r.Context(), err) {
			return
		}
		log.Printf("Error %d %v", status, err)
		if status < http.StatusBadRequest || status > 599 {
			status = http.StatusInternalServerError
		}
		statusText := http.StatusText(status)
		if statusText == "" {
			statusText = http.StatusText(http.StatusInternalServerError)
		}
		http.Error(w, statusText, status)
	}
}

func isClientClosedError(ctx context.Context, err error) bool {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Is(ctxErr, context.Canceled) || errors.Is(ctxErr, context.DeadlineExceeded)
	}
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "connection reset by peer") ||
		strings.Contains(text, "request cancelled")
}

func startServer(cancel context.CancelFunc, conf config.ServerConf) {

	maxSem = semaphore.NewWeighted(int64(conf.MaxClients))
	queueSem = semaphore.NewWeighted(int64(conf.Concurrency))

	go func() {
		ticker := time.NewTicker(time.Duration(conf.FreeMemoryInterval) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			Free()
		}
	}()

	http.Handle("/", appHandler(serveImg))
	http.Handle("/share", appHandler(serveShareImg))
	http.Handle("/upload", appHandler(UploadHandler))
	http.Handle("/upload_file", appHandler(UploadFileHandler))
	http.Handle("/delete", appHandler(DeleteHandler))
	http.Handle("/stat", appHandler(serveStat))
	http.HandleFunc("/favicon.ico", http.NotFound)

	log.Printf("Starting server on %s", conf.BindTo)

	go func() {
		err := http.ListenAndServe(conf.BindTo, nil)
		if err != nil {
			log.Fatal(err)
		}
		cancel()
	}()
}

func DeleteHandler(w http.ResponseWriter, r *http.Request) (int, error) {
	key := r.URL.Query().Get("key")
	if key == "" {
		return 500, errors.New("empty key arg")
	}

	if err := imgStorage.Delete(key); err != nil {
		if errors.Is(err, storage.NotFoundError) {
			return 404, fmt.Errorf("file not found: %s", key)
		}
		return 500, err
	}

	return 200, nil
}
