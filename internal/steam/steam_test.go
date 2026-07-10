package steam

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// setAPIBase points the package at a test server and restores the original on
// cleanup.
func setAPIBase(t *testing.T, base string) {
	t.Helper()
	orig := apiBase
	apiBase = base
	t.Cleanup(func() { apiBase = orig })
}

func TestEnabled(t *testing.T) {
	if New("", t.TempDir(), nil).Enabled() {
		t.Error("client with empty key should be disabled")
	}
	if New("  ", t.TempDir(), nil).Enabled() {
		t.Error("client with whitespace-only key should be disabled")
	}
	if !New("abc123", t.TempDir(), nil).Enabled() {
		t.Error("client with key should be enabled")
	}
}

func TestThumbnailPath_DisabledClient(t *testing.T) {
	c := New("", t.TempDir(), nil)
	if _, err := c.ThumbnailPath(context.Background(), "123"); err == nil {
		t.Fatal("expected error from disabled client")
	}
}

func TestThumbnailPath_InvalidID(t *testing.T) {
	c := New("key", t.TempDir(), nil)
	for _, bad := range []string{"", "abc", "12a", "../etc/passwd", "12/34"} {
		if _, err := c.ThumbnailPath(context.Background(), bad); err == nil {
			t.Errorf("expected error for invalid id %q", bad)
		}
	}
}

// newFakeSteam builds a fake Steam Web API + image CDN. It returns the httptest
// server and counters for how often each endpoint was hit.
func newFakeSteam(t *testing.T, imageBody string) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var detailsHits, imageHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&detailsHits, 1)
		id := r.URL.Query().Get("publishedfileids[0]")
		if r.URL.Query().Get("key") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		imgURL := "http://" + r.Host + "/preview/" + id
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":%q,"result":1,"preview_url":%q,"title":"Test Map"}]}}`, id, imgURL)
	})
	mux.HandleFunc("/preview/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&imageHits, 1)
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, imageBody)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &detailsHits, &imageHits
}

func TestThumbnailPath_DownloadsAndCaches(t *testing.T) {
	srv, detailsHits, imageHits := newFakeSteam(t, "JPEGDATA")
	setAPIBase(t, srv.URL)

	dir := t.TempDir()
	c := New("key", dir, nil)

	path, err := c.ThumbnailPath(context.Background(), "3070900859")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if path != filepath.Join(dir, "3070900859.jpg") {
		t.Errorf("unexpected cache path %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cached file: %v", err)
	}
	if string(data) != "JPEGDATA" {
		t.Errorf("cached body = %q, want JPEGDATA", data)
	}

	// Second call must be served from cache — no new network hits.
	if _, err := c.ThumbnailPath(context.Background(), "3070900859"); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if got := atomic.LoadInt32(detailsHits); got != 1 {
		t.Errorf("details endpoint hit %d times, want 1", got)
	}
	if got := atomic.LoadInt32(imageHits); got != 1 {
		t.Errorf("image endpoint hit %d times, want 1", got)
	}
}

func TestThumbnailPath_ConcurrentCoalesces(t *testing.T) {
	srv, detailsHits, imageHits := newFakeSteam(t, "JPEGDATA")
	setAPIBase(t, srv.URL)

	c := New("key", t.TempDir(), nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.ThumbnailPath(context.Background(), "999"); err != nil {
				t.Errorf("concurrent fetch: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(detailsHits); got != 1 {
		t.Errorf("details endpoint hit %d times under concurrency, want 1", got)
	}
	if got := atomic.LoadInt32(imageHits); got != 1 {
		t.Errorf("image endpoint hit %d times under concurrency, want 1", got)
	}
}

func TestPreviewURL_RejectsBadKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setAPIBase(t, srv.URL)

	c := New("badkey", t.TempDir(), nil)
	_, err := c.ThumbnailPath(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "rejected key") {
		t.Fatalf("expected rejected-key error, got %v", err)
	}
}

func TestPreviewURL_NoPreview(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("publishedfileids[0]")
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":%q,"result":1,"preview_url":""}]}}`, id)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setAPIBase(t, srv.URL)

	c := New("key", t.TempDir(), nil)
	_, err := c.ThumbnailPath(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "no preview image") {
		t.Fatalf("expected no-preview error, got %v", err)
	}
}

func TestPrefetch_DisabledIsNoop(t *testing.T) {
	// Should not panic or block.
	c := New("", t.TempDir(), nil)
	c.Prefetch([]string{"1", "2", "3"})
}
