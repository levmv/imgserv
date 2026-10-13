package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/levmv/imgserv/config"
)

func TestServerShutdownFinishesUpload(t *testing.T) {
	const configEnv = "IMGSERV_SHUTDOWN_TEST_CONFIG"
	if path := os.Getenv(configEnv); path != "" {
		if err := run(path, config.Overrides{}); err != nil {
			t.Fatal(err)
		}
		return
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	contents := fmt.Sprintf(`{"server":{"bind_to":%q},"storage":{"type":"local","local_path":%q}}`, address, dir)
	if err := os.WriteFile(configPath, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}

	// A separate process keeps libvips shutdown and signal handling isolated.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestServerShutdownFinishesUpload$")
	command.Env = append(os.Environ(), configEnv+"="+configPath)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = command.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		command.Process.Kill()
		<-exited
		if t.Failed() {
			t.Log(output.String())
		}
	})
	waitFor := func(condition func() bool) {
		t.Helper()
		for !condition() {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-exited:
				t.Fatalf("server exited early: %v", exitErr)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	readStats := func() (stats, error) {
		var result stats
		resp, err := client.Get("http://" + address + "/stat")
		if err != nil {
			return result, err
		}
		defer resp.Body.Close()
		err = json.NewDecoder(resp.Body).Decode(&result)
		return result, err
	}
	waitFor(func() bool { _, err := readStats(); return err == nil })

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	photo := testJPEG(t, 4, 3)
	if _, err := fmt.Fprintf(conn, "POST /upload?key=photo.jpg HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(photo), photo[:1]); err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool { stats, err := readStats(); return err == nil && stats.Uploaded == 1 })
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool { _, err := readStats(); return err != nil })

	// Finish the body after the listener closes: upload processing still needs libvips.
	if _, err := conn.Write(photo[1:]); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload returned %s", resp.Status)
	}
	<-exited
	if exitErr != nil {
		t.Fatalf("server exit: %v", exitErr)
	}
	stored, err := os.ReadFile(filepath.Join(dir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if width, height := jpegDimensions(t, stored); width != 4 || height != 3 {
		t.Fatalf("stored image is %dx%d, want 4x3", width, height)
	}
}
