// Package dna is the HTTP client the dna CLI and its MCP server use to talk
// to the DomainNameAPI reseller REST API: credentials, environments, retries,
// rate-limit pacing, quota metadata, error normalization and enum
// documentation. Operations themselves are described by internal/ops and
// executed through Client.Do; responses are passed through as raw JSON.
package dna

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Base URLs of the two API environments. Both serve the same API surface;
// OTE (Operational Test & Evaluation) is a separate deployment with its own
// data and its own API key, so nothing you do there touches live domains or
// your live balance.
const (
	ProductionURL = "https://api.domainresellerapi.com"
	OTEURL        = "https://ote.domainresellerapi.com"
)

// Environment selects which API deployment a client talks to.
type Environment string

const (
	// Production is the live environment: real registrations and real charges.
	Production Environment = "production"
	// OTE is the test environment. Use your OTE API key with it.
	OTE Environment = "ote"
)

// BaseURL returns the environment's base URL.
func (e Environment) BaseURL() string {
	if e == OTE {
		return OTEURL
	}
	return ProductionURL
}

// ParseEnvironment accepts the common spellings of the two environments:
// "production", "prod", "live" and "ote", "test", "sandbox". It also accepts
// the boolean test-mode spellings used by the other DomainNameAPI SDKs
// ("true", "1", "on", "yes" mean OTE; "false", "0", "off", "no" mean production).
func ParseEnvironment(s string) (Environment, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "production", "prod", "live", "false", "0", "off", "no":
		return Production, nil
	case "ote", "test", "testing", "sandbox", "true", "1", "on", "yes":
		return OTE, nil
	}
	return "", fmt.Errorf("unknown environment %q (use \"production\" or \"ote\")", s)
}

// Client talks to the DomainNameAPI REST API.
//
// A Client is safe for concurrent use. Create one with NewClient and reuse it.
type Client struct {
	baseURL    *url.URL
	resellerID string
	apiKey     string
	httpClient *http.Client
	userAgent  string
	maxRetries int
	maxWait    time.Duration
	trace      func(TraceEvent)
	pacer      *pacer
}

// Option configures a Client.
type Option func(*Client) error

// WithEnvironment selects production (default) or OTE.
func WithEnvironment(env Environment) Option {
	return func(c *Client) error { return c.setBaseURL(env.BaseURL()) }
}

// WithBaseURL points the client at a custom base URL (for proxies and tests).
func WithBaseURL(raw string) Option {
	return func(c *Client) error { return c.setBaseURL(raw) }
}

// WithHTTPClient replaces the HTTP client. Redirects are never followed: the
// API answers 302 when the reseller ID is unknown or suspended, and the
// client turns that into an authentication error.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return errors.New("nil http client")
		}
		cp := *hc
		cp.CheckRedirect = noRedirect
		c.httpClient = &cp
		return nil
	}
}

// WithTimeout sets the per-request timeout (default 60s).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) error { c.httpClient.Timeout = d; return nil }
}

// WithUserAgent sets the User-Agent header. The API sits behind Cloudflare,
// which rejects requests without a descriptive agent, so one is always sent.
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		if strings.TrimSpace(ua) != "" {
			c.userAgent = ua
		}
		return nil
	}
}

// WithRetries sets how many times a request is retried (default 2).
// See Do for which failures are retried.
func WithRetries(n int) Option {
	return func(c *Client) error {
		if n < 0 {
			n = 0
		}
		c.maxRetries = n
		return nil
	}
}

// WithPacing enables or disables client-side request pacing (default on).
// Pacing keeps the client under the gateway's per-key rate limits
// (5 req/s overall, 2 req/s for domain search, and at least 1s between DNS
// zone calls) so long-running programs see fewer 429 responses.
func WithPacing(enabled bool) Option {
	return func(c *Client) error {
		if enabled {
			c.pacer = newPacer()
		} else {
			c.pacer = nil
		}
		return nil
	}
}

// WithTrace registers a callback that observes every HTTP exchange. The API
// key is never included in events.
func WithTrace(fn func(TraceEvent)) Option {
	return func(c *Client) error { c.trace = fn; return nil }
}

// TraceEvent describes one step of a request, for logging and debugging.
type TraceEvent struct {
	Kind     string        // "request", "response", "retry"
	Method   string        //
	URL      string        //
	Status   int           // response status (0 for transport errors)
	Attempt  int           // 1-based attempt number
	Duration time.Duration // time spent on this attempt
	Wait     time.Duration // for "retry": how long the client waits
	Body     []byte        // request or response body (may be truncated)
	Err      error         // transport error, if any
	Quota    *Quota        // parsed quota headers, if any
}

// NewClient creates a client for the given reseller ID and API key.
//
// Find both in the DomainNameAPI reseller panel under API settings. The
// reseller ID is a UUID. Production and OTE use different API keys.
// Surrounding whitespace is trimmed. Credentials are checked lazily: a client
// without them can still build requests (see NewHTTPRequest), but Do fails
// with ErrNoCredentials.
func NewClient(resellerID, apiKey string, opts ...Option) (*Client, error) {
	c := &Client{
		resellerID: strings.TrimSpace(resellerID),
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 60 * time.Second, CheckRedirect: noRedirect},
		userAgent:  "DomainNameAPI-CLI (+https://github.com/domainreseller/cli)",
		maxRetries: 2,
		maxWait:    30 * time.Second,
		pacer:      newPacer(),
	}
	if err := c.setBaseURL(ProductionURL); err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Environment variables understood by the dna CLI.
const (
	EnvResellerID = "DNA_RESELLER_ID"
	EnvAPIKey     = "DNA_API_KEY"
	EnvEnv        = "DNA_ENV"       // "production" or "ote"
	EnvTestMode   = "DNA_TEST_MODE" // "true" selects OTE (compatible with the other SDKs)
	EnvBaseURL    = "DNA_BASE_URL"
)

// BaseURL returns the base URL the client sends requests to.
func (c *Client) BaseURL() string { return c.baseURL.String() }

// HasCredentials reports whether both the reseller ID and API key are set.
func (c *Client) HasCredentials() bool { return c.resellerID != "" && c.apiKey != "" }

func (c *Client) setBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid base URL %q", raw)
	}
	c.baseURL = u
	return nil
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// ErrNoCredentials is returned by Do when the reseller ID or API key is empty.
var ErrNoCredentials = errors.New("dna: reseller ID and API key are required (set DNA_RESELLER_ID and DNA_API_KEY, or run `dna auth login`)")

// Request is one API request. internal/ops builds it from an operation and
// its input; `dna api` builds it directly.
type Request struct {
	Method string     // GET, POST, PUT or DELETE
	Path   string     // path with parameters substituted, e.g. /api/v1/domains/info
	Query  url.Values // query string
	// Body is JSON-encoded when non-nil. json.RawMessage and []byte are sent as is.
	Body any
	// Idempotent marks a non-GET request as safe to retry after network
	// errors and 5xx responses. GET requests are always treated as idempotent.
	Idempotent bool
}

// Response describes a completed HTTP exchange.
type Response struct {
	StatusCode    int
	Header        http.Header
	Body          []byte        // raw response body
	CorrelationID string        // x-correlation-id; quote it when contacting support
	Quota         *Quota        // nil when the gateway sent no quota headers
	Duration      time.Duration // total time including retries
	Attempts      int
}

// Quota mirrors the gateway's soft-quota headers. The quota never rejects
// requests: once Used exceeds Limit, the gateway delays each request (by
// Delay seconds, at most 5) until points decay. Points decay by 20% per
// minute; searches and info lookups cost 1 point, everything else 10.
type Quota struct {
	Scope string  `json:"scope"`        // category: domain, contact, ssl, products, order, deposit, hosting
	Cost  float64 `json:"cost"`         // points charged for this request
	Used  float64 `json:"used"`         // points in the bucket after this request
	Limit float64 `json:"limit"`        // soft threshold where slowing starts (200)
	Delay float64 `json:"delaySeconds"` // seconds this request was held by the gateway
	Reset float64 `json:"resetSeconds"` // seconds until the bucket is back under Limit
}

func parseQuota(h http.Header) *Quota {
	if h.Get("X-Quota-Limit") == "" && h.Get("X-Quota-Used") == "" {
		return nil
	}
	f := func(k string) float64 {
		n, _ := strconv.ParseFloat(strings.TrimSpace(h.Get(k)), 64)
		return n
	}
	return &Quota{
		Scope: h.Get("X-Quota-Scope"),
		Cost:  f("X-Quota-Cost"),
		Used:  f("X-Quota-Used"),
		Limit: f("X-Quota-Limit"),
		Delay: f("X-Quota-Delay"),
		Reset: f("X-Quota-Reset"),
	}
}

// NewHTTPRequest builds the *http.Request for r without sending it. The CLI
// uses it for --dry-run. Credentials are attached only when set.
func (c *Client) NewHTTPRequest(ctx context.Context, r *Request) (*http.Request, []byte, error) {
	if r == nil {
		return nil, nil, errors.New("nil request")
	}
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := r.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Path is already escaped (path parameters go through url.PathEscape),
	// so parse it instead of assigning url.URL.Path, which would escape twice.
	u, err := url.Parse(strings.TrimRight(c.baseURL.String(), "/") + path)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid request path %q: %w", r.Path, err)
	}
	if len(r.Query) > 0 {
		u.RawQuery = r.Query.Encode()
	}
	var body []byte
	switch b := r.Body.(type) {
	case nil:
	case json.RawMessage:
		body = b
	case []byte:
		body = b
	default:
		var err error
		body, err = json.Marshal(b)
		if err != nil {
			return nil, nil, fmt.Errorf("encode request body: %w", err)
		}
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-KEY", c.apiKey)
	}
	if c.resellerID != "" {
		req.Header["__reseller"] = []string{c.resellerID} // non-canonical name, sent verbatim
	}
	return req, body, nil
}

// Do sends r and decodes a successful JSON response into out (when out is
// non-nil and the body is not empty). Pass a *json.RawMessage to keep the
// body undecoded.
//
// Errors from the API are returned as *APIError. Retries: 429 responses are
// always retried (the gateway rejects them before they reach the backend, so
// nothing was executed), honouring Retry-After. Network errors and 502/503/504
// responses are retried only for idempotent requests (GET, or Idempotent set),
// so a registration or renewal is never sent twice.
func (c *Client) Do(ctx context.Context, r *Request, out any) (*Response, error) {
	if !c.HasCredentials() {
		return nil, ErrNoCredentials
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	idempotent := r.Idempotent || strings.EqualFold(r.Method, http.MethodGet) || r.Method == ""
	for attempt := 1; ; attempt++ {
		if c.pacer != nil {
			if err := c.pacer.wait(ctx, r.Path); err != nil {
				return nil, err
			}
		}
		req, body, err := c.NewHTTPRequest(ctx, r)
		if err != nil {
			return nil, err
		}
		c.emit(TraceEvent{Kind: "request", Method: req.Method, URL: req.URL.String(), Attempt: attempt, Body: truncate(body)})
		t0 := time.Now()
		hresp, err := c.httpClient.Do(req)
		if err != nil {
			c.emit(TraceEvent{Kind: "response", Method: req.Method, URL: req.URL.String(), Attempt: attempt, Duration: time.Since(t0), Err: err})
			if ctx.Err() != nil {
				return nil, &NetworkError{Op: req.Method + " " + r.Path, Err: ctx.Err()}
			}
			if idempotent && attempt <= c.maxRetries && retryableNetErr(err) {
				wait := backoff(attempt)
				c.emit(TraceEvent{Kind: "retry", Method: req.Method, URL: req.URL.String(), Attempt: attempt, Wait: wait, Err: err})
				if err := sleep(ctx, wait); err != nil {
					return nil, &NetworkError{Op: req.Method + " " + r.Path, Err: err}
				}
				continue
			}
			return nil, &NetworkError{Op: req.Method + " " + r.Path, Err: err}
		}
		respBody, readErr := io.ReadAll(io.LimitReader(hresp.Body, 64<<20))
		hresp.Body.Close()
		resp := &Response{
			StatusCode:    hresp.StatusCode,
			Header:        hresp.Header,
			Body:          respBody,
			CorrelationID: hresp.Header.Get("X-Correlation-Id"),
			Quota:         parseQuota(hresp.Header),
			Duration:      time.Since(start),
			Attempts:      attempt,
		}
		c.emit(TraceEvent{Kind: "response", Method: req.Method, URL: req.URL.String(), Status: resp.StatusCode, Attempt: attempt, Duration: time.Since(t0), Body: truncate(respBody), Quota: resp.Quota})
		if readErr != nil {
			return resp, &NetworkError{Op: req.Method + " " + r.Path, Err: readErr}
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
				if raw, ok := out.(*json.RawMessage); ok {
					*raw = append((*raw)[:0], respBody...)
				} else if err := json.Unmarshal(respBody, out); err != nil {
					return resp, fmt.Errorf("decode %s %s response: %w", req.Method, r.Path, err)
				}
			}
			return resp, nil
		}

		retry := resp.StatusCode == http.StatusTooManyRequests ||
			(idempotent && (resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504))
		if retry && attempt <= c.maxRetries {
			wait := retryAfter(hresp.Header, attempt)
			if wait <= c.maxWait {
				c.emit(TraceEvent{Kind: "retry", Method: req.Method, URL: req.URL.String(), Status: resp.StatusCode, Attempt: attempt, Wait: wait})
				if err := sleep(ctx, wait); err != nil {
					return resp, &NetworkError{Op: req.Method + " " + r.Path, Err: err}
				}
				continue
			}
		}
		return resp, newAPIError(resp)
	}
}

func (c *Client) emit(ev TraceEvent) {
	if c.trace != nil {
		c.trace(ev)
	}
}

func truncate(b []byte) []byte {
	const max = 16 << 10
	if len(b) > max {
		return append(append([]byte{}, b[:max]...), []byte("…(truncated)")...)
	}
	return b
}

func retryableNetErr(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var oe *net.OpError
	if errors.As(err, &oe) {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		strings.Contains(err.Error(), "connection reset")
}

func backoff(attempt int) time.Duration {
	base := 500 * time.Millisecond << (attempt - 1)
	return base + time.Duration(rand.Int64N(int64(250*time.Millisecond)))
}

func retryAfter(h http.Header, attempt int) time.Duration {
	if s := strings.TrimSpace(h.Get("Retry-After")); s != "" {
		if n, err := strconv.ParseFloat(s, 64); err == nil && n >= 0 {
			return time.Duration(n*float64(time.Second)) + 50*time.Millisecond
		}
		if t, err := http.ParseTime(s); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
		}
	}
	return backoff(attempt)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pacer spaces requests per rate-limit class.
type pacer struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newPacer() *pacer { return &pacer{last: map[string]time.Time{}} }

func paceClass(path string) (string, time.Duration) {
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/api/v1/domains/zones"):
		return "zones", time.Second
	case p == "/api/v1/domains/search" || p == "/api/v1/domains/bulk-search":
		return "search", 500 * time.Millisecond
	case strings.HasPrefix(p, "/api/v1/domains/info"), strings.HasPrefix(p, "/api/v1/products/tlds"):
		return "", 0 // not rate limited by the gateway
	}
	return "general", 200 * time.Millisecond
}

func (p *pacer) wait(ctx context.Context, path string) error {
	class, gap := paceClass(path)
	if gap == 0 {
		return nil
	}
	p.mu.Lock()
	now := time.Now()
	next := p.last[class].Add(gap)
	if next.Before(now) {
		next = now
	}
	p.last[class] = next
	if class == "search" { // search also counts against the general budget; zones has its own
		if g := p.last["general"]; g.Before(next) {
			p.last["general"] = next
		}
	}
	p.mu.Unlock()
	if d := time.Until(next); d > 0 {
		return sleep(ctx, d)
	}
	return nil
}
