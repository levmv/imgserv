package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/levmv/imgserv/config"
	"github.com/levmv/imgserv/storage"
	"golang.org/x/sync/semaphore"
	"log"
	"net/http"
	"time"
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
		log.Printf("Error %d %v", status, err)
		switch status {
		case http.StatusNotFound:
			http.Error(w, http.StatusText(http.StatusNotFound), status)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}

func startServer(cancel context.CancelFunc, conf config.ServerConf) {

	maxSem = semaphore.NewWeighted(int64(conf.MaxClients))
	queueSem = semaphore.NewWeighted(int64(conf.Concurrency))

	go func() {
		for range time.Tick(time.Duration(conf.FreeMemoryInterval) * time.Second) {
			Free()
		}
	}()

	http.Handle("/", appHandler(serveImg))
	http.Handle("/share", appHandler(serveShareImg))
	http.Handle("/upload", appHandler(UploadHandler))
	http.Handle("/upload_file", appHandler(UploadFileHandler))
	http.Handle("/delete", appHandler(DeleteHandler))
	http.Handle("/stat", appHandler(serveStat))
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(404)
	})

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
		if errors.Is(storage.NotFoundError, err) {
			log.Println("can't delete not existent file", key)
			return 200, err
		}
		return 500, err
	}

	return 200, nil
}
