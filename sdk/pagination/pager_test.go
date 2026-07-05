package pagination

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestOptionChangeClearsETag(t *testing.T) {
	var seenSecondIfNoneMatch atomic.Bool
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch call {
		case 1:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("first request unexpectedly sent If-None-Match %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != "10" {
				t.Fatalf("first request limit = %q, want 10", got)
			}
			w.Header().Set("ETag", `"hash123"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[1]}`))
		case 2:
			seenSecondIfNoneMatch.Store(r.Header.Get("If-None-Match") != "")
			if got := r.URL.Query().Get("limit"); got != "20" {
				t.Fatalf("second request limit = %q, want 20", got)
			}
			w.Header().Set("ETag", `"hash456"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[1,2]}`))
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer server.Close()

	pager, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := pager.Do(context.Background(), RequestOptions{Path: "/items", Query: values("page", "1", "limit", "10")}); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	if _, _, err := pager.Do(context.Background(), RequestOptions{Path: "/items", Query: values("page", "1", "limit", "20")}); err != nil {
		t.Fatalf("second Do failed: %v", err)
	}

	if seenSecondIfNoneMatch.Load() {
		t.Fatal("changed request options reused stale If-None-Match header")
	}
}

func TestUnchangedOptionsRetainETag(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch call {
		case 1:
			w.Header().Set("ETag", `"hash123"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[1]}`))
		case 2:
			if got := r.Header.Get("If-None-Match"); got != `"hash123"` {
				t.Fatalf("If-None-Match = %q, want %q", got, `"hash123"`)
			}
			w.WriteHeader(http.StatusNotModified)
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer server.Close()

	pager, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	opts := RequestOptions{Path: "/items", Query: values("page", "1", "limit", "10")}
	if _, _, err := pager.Do(context.Background(), opts); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	_, _, err = pager.Do(context.Background(), opts)
	if !errors.Is(err, ErrNotModified) {
		t.Fatalf("second Do error = %v, want ErrNotModified", err)
	}
}

func TestChangedOptionsAvoidStale304AndFetchNewDataset(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			w.Header().Set("ETag", `"shared"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`old-data`))
			return
		}

		if got := r.URL.Query().Get("filter"); got != "new" {
			t.Fatalf("filter = %q, want new", got)
		}
		if got := r.Header.Get("If-None-Match"); got != "" {
			// This simulates a server that would incorrectly return 304 if the
			// stale validator crossed request-option boundaries.
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"new"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`new-data`))
	}))
	defer server.Close()

	pager, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := pager.Do(context.Background(), RequestOptions{Path: "/items", Query: values("filter", "old")}); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	resp, body, err := pager.Do(context.Background(), RequestOptions{Path: "/items", Query: values("filter", "new")})
	if err != nil {
		t.Fatalf("second Do failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := string(body); got != "new-data" {
		t.Fatalf("body = %q, want new-data", got)
	}
}

func values(kv ...string) url.Values {
	v := make(url.Values)
	for i := 0; i < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}
