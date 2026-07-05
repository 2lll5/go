package pagination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// ErrNotModified is returned when the server legitimately answers 304 for the
// exact same request state that previously produced the cached ETag.
var ErrNotModified = errors.New("pagination: response not modified")

// RequestOptions describes the parts of a paginated request that affect the
// response representation. The pager uses these fields to scope cached ETags so
// a validator learned for one set of options is never sent with another.
type RequestOptions struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// Pager executes paginated HTTP requests and keeps ETags scoped to the full
// request state. It is safe for concurrent use.
type Pager struct {
	client  *http.Client
	baseURL *url.URL

	mu    sync.RWMutex
	etags map[string]string
}

// New creates a Pager using baseURL as the origin for request paths. If client
// is nil, http.DefaultClient is used.
func New(baseURL string, client *http.Client) (*Pager, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Pager{
		client:  client,
		baseURL: parsed,
		etags:   make(map[string]string),
	}, nil
}

// Do sends a request for the provided pagination options. Cached ETags are
// looked up by a stable key derived from method, URL, query, headers (except
// conditional validators), and body. Changing any of those options results in a
// different key, so stale If-None-Match values are omitted automatically.
func (p *Pager) Do(ctx context.Context, opts RequestOptions) (*http.Response, []byte, error) {
	if opts.Method == "" {
		opts.Method = http.MethodGet
	}

	requestURL, err := p.requestURL(opts.Path, opts.Query)
	if err != nil {
		return nil, nil, err
	}

	key := requestKey(opts, requestURL)

	var body io.Reader
	if len(opts.Body) > 0 {
		body = bytes.NewReader(opts.Body)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, requestURL, body)
	if err != nil {
		return nil, nil, err
	}
	copyHeaders(req.Header, opts.Header)

	if etag := p.cachedETag(key); etag != "" {
		req.Header.Set("If-None-Match", etag)
	} else {
		// Ensure callers cannot accidentally carry a stale validator in the
		// supplied options when the current request state has no matching cache.
		req.Header.Del("If-None-Match")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return resp, nil, ErrNotModified
	}

	data, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return resp, nil, readErr
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		p.storeETag(key, etag)
	}

	return resp, data, nil
}

func (p *Pager) requestURL(path string, query url.Values) (string, error) {
	rel, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	u := p.baseURL.ResolveReference(rel)
	if len(query) > 0 {
		q := u.Query()
		for key, values := range query {
			q.Del(key)
			for _, value := range values {
				q.Add(key, value)
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func (p *Pager) cachedETag(key string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.etags[key]
}

func (p *Pager) storeETag(key, etag string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.etags[key] = etag
}

func requestKey(opts RequestOptions, requestURL string) string {
	h := sha256.New()
	h.Write([]byte(strings.ToUpper(opts.Method)))
	h.Write([]byte("\n"))
	h.Write([]byte(requestURL))
	h.Write([]byte("\n"))
	writeStableHeaders(h, opts.Header)
	h.Write([]byte("\n"))
	h.Write(opts.Body)
	return hex.EncodeToString(h.Sum(nil))
}

func writeStableHeaders(w io.Writer, header http.Header) {
	keys := make([]string, 0, len(header))
	for key := range header {
		canonical := http.CanonicalHeaderKey(key)
		if strings.EqualFold(canonical, "If-None-Match") || strings.EqualFold(canonical, "If-Modified-Since") {
			continue
		}
		keys = append(keys, canonical)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := append([]string(nil), header.Values(key)...)
		sort.Strings(values)
		io.WriteString(w, key)
		io.WriteString(w, ":")
		io.WriteString(w, strings.Join(values, ","))
		io.WriteString(w, "\n")
	}
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
