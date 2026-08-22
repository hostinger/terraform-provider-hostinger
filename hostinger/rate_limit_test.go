package hostinger

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// getZone issues a plain zone GET through do(), so these tests exercise the
// retry wrapper itself rather than any particular caller of it.
func getZone(t *testing.T, client *HostingerClient) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, client.BaseURL+zonePath, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	client.addStandardHeaders(req)

	return client.do(req)
}

// TestDo_RetriesShortRateLimit covers the case worth waiting out: a brief
// backoff, after which the request goes through.
func TestDo_RetriesShortRateLimit(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	var attempts atomic.Int32
	server.setIntercept(func(w http.ResponseWriter, _ *http.Request) bool {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return true
		}
		return false
	})

	resp, err := getZone(t, client)
	if err != nil {
		t.Fatalf("expected the retry to succeed, got %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected the retried request to succeed, got HTTP %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
}

// TestDo_FailsFastOnLongRateLimit is the block this provider actually hit: an
// observed Retry-After of 2272 seconds. Waiting it out inline would hang an
// apply for over half an hour, so it is reported instead -- with the wait,
// which the old "error code: 1015" passthrough never gave.
func TestDo_FailsFastOnLongRateLimit(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	server.setIntercept(func(w http.ResponseWriter, _ *http.Request) bool {
		w.Header().Set("Retry-After", "2272")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "error code: 1015")
		return true
	})

	start := time.Now()
	_, err := getZone(t, client)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if elapsed > 5*time.Second {
		t.Errorf("expected to fail fast, took %s", elapsed)
	}

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected a *RateLimitError, got %T: %v", err, err)
	}
	if rateLimitErr.RetryAfter != 2272*time.Second {
		t.Errorf("expected a 2272s wait, got %s", rateLimitErr.RetryAfter)
	}

	msg := err.Error()
	for _, want := range []string{"37m52s", "clears at", "whole API host", "error code: 1015"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected the message to mention %q, got: %s", want, msg)
		}
	}

	if n := server.totalRequests(); n != 1 {
		t.Errorf("expected a long block not to be retried, got %d requests", n)
	}
}

// TestDo_ReplaysRequestBody guards the trap in retrying an HTTP request: the
// body is a reader the first attempt consumes, so a naive replay sends an empty
// body and the write silently does nothing.
func TestDo_ReplaysRequestBody(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	var mu sync.Mutex
	var seen []string

	server.setIntercept(func(w http.ResponseWriter, r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		seen = append(seen, string(body))
		first := len(seen) == 1
		mu.Unlock()

		if first {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return true
		}
		return false
	})

	const payload = `{"overwrite":false,"zone":[{"name":"api","type":"A"}]}`

	req, err := http.NewRequest(http.MethodPut, client.BaseURL+zonePath, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	client.addStandardHeaders(req)

	resp, err := client.do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(seen) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(seen))
	}
	for i, body := range seen {
		if body != payload {
			t.Errorf("attempt %d sent %q, want %q", i+1, body, payload)
		}
	}
}

// TestDo_GivesUpAfterRepeatedShortRateLimits stops a limiter that keeps saying
// "one more second" from holding an apply open indefinitely.
func TestDo_GivesUpAfterRepeatedShortRateLimits(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	server.setIntercept(func(w http.ResponseWriter, _ *http.Request) bool {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		return true
	})

	_, err := getZone(t, client)

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected a *RateLimitError, got %T: %v", err, err)
	}
	if rateLimitErr.Attempts != maxRateLimitRetries+1 {
		t.Errorf("expected %d attempts, got %d", maxRateLimitRetries+1, rateLimitErr.Attempts)
	}
	if n := server.totalRequests(); n != maxRateLimitRetries+1 {
		t.Errorf("expected %d requests, got %d", maxRateLimitRetries+1, n)
	}
}

func TestDo_RateLimitWithoutRetryAfter(t *testing.T) {
	server := newTestAPIServer(t, map[string]string{"example.com": testZoneExample})
	client := server.client()

	server.setIntercept(func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusTooManyRequests)
		return true
	})

	_, err := getZone(t, client)

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected a *RateLimitError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "no usable Retry-After header") {
		t.Errorf("unexpected message: %s", err)
	}
	if n := server.totalRequests(); n != 1 {
		t.Errorf("expected no retry without a Retry-After, got %d requests", n)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "seconds", value: "2272", want: 2272 * time.Second, ok: true},
		{name: "zero", value: "0", want: 0, ok: true},
		{name: "surrounding space", value: " 30 ", want: 30 * time.Second, ok: true},
		{name: "http date", value: "Sat, 22 Aug 2026 12:00:30 GMT", want: 30 * time.Second, ok: true},
		{name: "http date in the past", value: "Sat, 22 Aug 2026 11:59:00 GMT", want: 0, ok: true},
		{name: "absent", value: "", ok: false},
		{name: "negative", value: "-1", ok: false},
		{name: "nonsense", value: "soon", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := http.Header{}
			if tc.value != "" {
				header.Set("Retry-After", tc.value)
			}

			got, ok := parseRetryAfter(header, now)
			if ok != tc.ok {
				t.Fatalf("ok: got %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("duration: got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestRewind_WithoutGetBody refuses to replay a body it cannot rebuild, rather
// than resending an empty one.
func TestRewind_WithoutGetBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "http://example.invalid/", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.GetBody = nil

	if _, err := rewind(req); err == nil {
		t.Error("expected an error, got none")
	}
}
