package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
)

func TestAppHandlerPreservesReturnedStatus(t *testing.T) {
	tests := []int{
		http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusTooManyRequests,
		http.StatusNotImplemented,
	}

	for _, status := range tests {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler := appHandler(func(w http.ResponseWriter, r *http.Request) (int, error) {
				return status, errors.New("boom")
			})

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if rec.Code != status {
				t.Fatalf("got status %d, want %d", rec.Code, status)
			}
		})
	}
}

func TestGenUuidShapeAndUniqueness(t *testing.T) {
	seen := make(map[string]bool)

	for i := 0; i < 1000; i++ {
		id, err := genUuid()
		if err != nil {
			t.Fatalf("genUuid returned an error: %v", err)
		}
		if len(id) != 22 {
			t.Fatalf("got id length %d for %q, want 22", len(id), id)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = true
	}
}

func TestIsClientClosedError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !isClientClosedError(ctx, errors.New("operation error S3: canceled, context canceled")) {
		t.Fatal("expected canceled request context to be treated as client close")
	}
	if !isClientClosedError(context.Background(), syscall.EPIPE) {
		t.Fatal("expected broken pipe to be treated as client close")
	}
	if isClientClosedError(context.Background(), errors.New("s3 timeout")) {
		t.Fatal("unexpected client close classification")
	}
}
