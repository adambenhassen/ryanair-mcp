package ryanair_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

// redirectTransport rewrites every request to the test server while preserving
// path and query, so the client's hard-coded hosts hit our fixtures.
type redirectTransport struct {
	base *url.URL
}

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.base.Scheme
	req.URL.Host = rt.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

// fakeServer records prime hits and the last request query for assertions.
type fakeServer struct {
	primeHits atomic.Int32
	lastQuery atomic.Pointer[url.Values]
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func newClient(t *testing.T, handler http.Handler) *ryanair.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	httpClient := &http.Client{Transport: redirectTransport{base: base}}
	client, err := ryanair.NewClient(ryanair.WithHTTPClient(httpClient), ryanair.WithNetworkTTL(time.Minute))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

// routeFixtures builds a handler serving the given path-prefix -> fixture map,
// recording prime hits and the last query string.
func routeFixtures(t *testing.T, fs *fakeServer, routes map[string]string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fs.primeHits.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		q := r.URL.Query()
		fs.lastQuery.Store(&q)
		for prefix, name := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write(fixture(t, name)); err != nil {
					t.Errorf("write fixture: %v", err)
				}
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	return mux
}

func TestRetryClassification(t *testing.T) {
	// Shrink the retry backoff so the retried sub-tests don't sleep for real.
	defer ryanair.SetBaseBackoff(time.Millisecond)()
	t.Run("429 is retried then succeeds", func(t *testing.T) {
		var calls atomic.Int32
		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(fixture(t, "one_way_fares.json")); err != nil {
				t.Errorf("write: %v", err)
			}
		}))
		_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
			Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-02",
		})
		if err != nil {
			t.Fatalf("expected success after retry, got %v", err)
		}
		if calls.Load() != 2 {
			t.Errorf("calls = %d, want 2 (one retry)", calls.Load())
		}
	})

	t.Run("409 is not retried", func(t *testing.T) {
		var calls atomic.Int32
		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			calls.Add(1)
			http.Error(w, `{"message":"Availability declined"}`, http.StatusConflict)
		}))
		_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
			Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-02",
		})
		var apiErr *ryanair.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *ryanair.APIError, got %v", err)
		}
		if apiErr.Status != http.StatusConflict {
			t.Errorf("status = %d, want 409", apiErr.Status)
		}
		if calls.Load() != 1 {
			t.Errorf("calls = %d, want 1 (no retry on 409)", calls.Load())
		}
	})
}

func TestValidationRejectsBadInput(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin: "XX", DateFrom: "2026-07-01", DateTo: "2026-07-02",
	})
	if err == nil {
		t.Fatal("expected error for invalid origin IATA")
	}
}
