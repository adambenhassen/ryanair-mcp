package ryanair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	servicesHost = "https://services-api.ryanair.com"
	wwwHost      = "https://www.ryanair.com"

	defaultTimeout    = 15 * time.Second
	defaultNetworkTTL = 6 * time.Hour
	maxRetries        = 3
	baseBackoff       = 300 * time.Millisecond
	maxBodySnippet    = 512
)

// userAgents is a small pool of realistic desktop browser User-Agent strings.
// One is chosen per request; Ryanair sometimes blocks obvious non-browser
// clients.
var userAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:122.0) Gecko/20100101 Firefox/122.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
}

// APIError describes a non-2xx response from a Ryanair endpoint.
type APIError struct {
	Endpoint string
	Status   int
	Body     string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("ryanair: %s returned HTTP %d: %s", e.Endpoint, e.Status, e.Body)
}

// Client talks to Ryanair's read APIs. It is safe for concurrent use.
type Client struct {
	http *http.Client

	primeOnce sync.Once
	primeErr  error

	netMu      sync.Mutex
	netCache   []Airport
	netRoutes  map[string][]string
	netFetched time.Time
	netTTL     time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom *http.Client (useful for tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithNetworkTTL overrides how long the cached network bundle is reused.
func WithNetworkTTL(d time.Duration) Option {
	return func(c *Client) { c.netTTL = d }
}

// NewClient builds a Client with a cookie jar and sane defaults.
func NewClient(opts ...Option) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("ryanair: new cookie jar: %w", err)
	}
	c := &Client{
		http:   &http.Client{Timeout: defaultTimeout, Jar: jar},
		netTTL: defaultNetworkTTL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// prime warms the cookie jar by hitting the public site once. Cold calls to the
// services-api host sometimes return 403 without these cookies.
func (c *Client) prime(ctx context.Context) error {
	c.primeOnce.Do(func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, wwwHost, http.NoBody)
		if err != nil {
			c.primeErr = fmt.Errorf("ryanair: build prime request: %w", err)
			return
		}
		req.Header.Set("User-Agent", randomUA())
		resp, err := c.http.Do(req)
		if err != nil {
			c.primeErr = fmt.Errorf("ryanair: prime session: %w", err)
			return
		}
		c.primeErr = drainClose(resp)
	})
	return c.primeErr
}

// getJSON performs a primed, retrying GET and decodes the JSON body into out.
func getJSON[T any](ctx context.Context, c *Client, endpoint, rawURL string, query url.Values, out *T) error {
	if err := c.prime(ctx); err != nil {
		return err
	}
	full := rawURL
	if len(query) > 0 {
		full = rawURL + "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepBackoff(ctx, attempt); err != nil {
				return err
			}
		}
		retry, err := c.doOnce(ctx, endpoint, full, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			return err
		}
	}
	return fmt.Errorf("ryanair: %s exhausted retries: %w", endpoint, lastErr)
}

// doOnce executes a single GET attempt. The bool reports whether the error is
// transient and the request should be retried.
func (c *Client) doOnce(ctx context.Context, endpoint, full string, out any) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("ryanair: build %s request: %w", endpoint, err)
	}
	req.Header.Set("User-Agent", randomUA())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return true, fmt.Errorf("ryanair: %s request: %w", endpoint, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodySnippet)) //nolint:errcheck // best-effort error snippet
		apiErr := &APIError{Endpoint: endpoint, Status: resp.StatusCode, Body: string(body)}
		return isTransient(resp.StatusCode), apiErr
	}

	if derr := json.NewDecoder(resp.Body).Decode(out); derr != nil {
		return false, fmt.Errorf("ryanair: decode %s response: %w", endpoint, derr)
	}
	return false, nil
}

// isTransient reports whether an HTTP status warrants a retry.
func isTransient(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// sleepBackoff waits with capped exponential backoff, honoring ctx cancellation.
func sleepBackoff(ctx context.Context, attempt int) error {
	delay := baseBackoff * time.Duration(1<<(attempt-1))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// drainClose discards and closes a response body, returning any close error.
func drainClose(resp *http.Response) error {
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		closeErr := resp.Body.Close()
		return errors.Join(fmt.Errorf("ryanair: drain body: %w", err), closeErr)
	}
	return resp.Body.Close()
}

// randomUA returns a random User-Agent from the pool.
func randomUA() string {
	//nolint:gosec // G404: UA rotation is cosmetic and needs no crypto-grade randomness.
	return userAgents[rand.IntN(len(userAgents))]
}

// itoa is a small helper for building query params without fmt.
func itoa(n int) string {
	return strconv.Itoa(n)
}
