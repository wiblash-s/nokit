// Package steam fetches Counter-Strike 2 Workshop item metadata from the Steam
// Web API and caches map preview thumbnails on disk.
//
// The panel only needs one piece of information from Steam for each installed
// workshop map: the item's preview image URL. Given a STEAM_API_KEY, the client
// queries IPublishedFileService/GetDetails to resolve a workshop file ID to its
// preview_url, downloads the image once, and stores it under a local cache
// directory. Subsequent requests are served straight from disk, so the Steam
// API is hit at most once per workshop item.
//
// If no API key is configured the client is disabled: Enabled reports false and
// the thumbnail handlers degrade gracefully (the UI falls back to placeholder
// gradients).
package steam

import (
        "context"
        "encoding/json"
        "fmt"
        "io"
        "log/slog"
        "net/http"
        "net/url"
        "os"
        "path/filepath"
        "regexp"
        "strings"
        "sync"
        "time"
)

// apiBase is the Steam Web API host. It is a package variable so tests can point
// the client at an httptest server.
var apiBase = "https://api.steampowered.com"

// SetAPIBase overrides the Steam Web API host and returns a function that
// restores the previous value. It exists so tests in other packages (which
// cannot see unexported symbols) can point the client at a local httptest
// server. It must not be used outside of tests.
func SetAPIBase(base string) (restore func()) {
        prev := apiBase
        apiBase = base
        return func() { apiBase = prev }
}

// validID matches a bare numeric Steam workshop file ID. Everything else is
// rejected before it reaches the filesystem or the Steam API.
var validID = regexp.MustCompile(`^\d+$`)

// maxThumbnailBytes caps how much of a preview image we will download. Steam
// preview images are small (typically well under 1 MB); this guards against a
// hostile or misbehaving upstream streaming an unbounded body.
const maxThumbnailBytes = 8 << 20 // 8 MiB

// Client resolves and caches Steam Workshop map thumbnails.
//
// The zero value is not usable; construct one with New. A Client is safe for
// concurrent use.
type Client struct {
        apiKey   string
        cacheDir string
        http     *http.Client
        logger   *slog.Logger

        // locks serialises work per workshop ID so concurrent requests for the
        // same thumbnail result in a single download rather than a thundering herd.
        locksMu sync.Mutex
        locks   map[string]*sync.Mutex
}

// New constructs a Client.
//
// apiKey is the Steam Web API key (https://steamcommunity.com/dev/apikey). When
// empty the client is disabled. cacheDir is where downloaded thumbnails are
// stored; it is created on demand. A nil logger is replaced with a no-op logger.
func New(apiKey, cacheDir string, logger *slog.Logger) *Client {
        if logger == nil {
                logger = slog.New(slog.NewTextHandler(io.Discard, nil))
        }
        if cacheDir == "" {
                cacheDir = "thumbnails"
        }
        return &Client{
                apiKey:   strings.TrimSpace(apiKey),
                cacheDir: cacheDir,
                http:     &http.Client{Timeout: 20 * time.Second},
                logger:   logger,
                locks:    make(map[string]*sync.Mutex),
        }
}

// Enabled reports whether a Steam API key is configured. When false, callers
// should skip thumbnail resolution entirely.
func (c *Client) Enabled() bool {
        return c != nil && c.apiKey != ""
}

// lockFor returns the per-ID mutex, creating it on first use.
func (c *Client) lockFor(id string) *sync.Mutex {
        c.locksMu.Lock()
        defer c.locksMu.Unlock()
        mu, ok := c.locks[id]
        if !ok {
                mu = &sync.Mutex{}
                c.locks[id] = mu
        }
        return mu
}

// cachePath returns the on-disk path a thumbnail for id would be stored at.
func (c *Client) cachePath(id string) string {
        return filepath.Join(c.cacheDir, id+".jpg")
}

// cached reports whether a non-empty thumbnail file already exists for id.
func (c *Client) cached(id string) bool {
        info, err := os.Stat(c.cachePath(id))
        return err == nil && info.Size() > 0
}

// ThumbnailPath returns the local filesystem path to the cached thumbnail for
// the given workshop ID, downloading it first if necessary.
//
// It returns an error if the client is disabled, the ID is malformed, Steam has
// no preview image for the item, or the download fails. Concurrent calls for the
// same ID are coalesced so the image is fetched at most once.
func (c *Client) ThumbnailPath(ctx context.Context, id string) (string, error) {
        if !c.Enabled() {
                return "", fmt.Errorf("steam: client disabled (no API key)")
        }
        if !validID.MatchString(id) {
                return "", fmt.Errorf("steam: invalid workshop id %q", id)
        }

        c.logger.Debug("thumbnail request", "workshop_id", id)

        path := c.cachePath(id)
        if c.cached(id) {
                c.logger.Debug("thumbnail cache hit", "workshop_id", id, "path", path)
                return path, nil
        }

        c.logger.Info("thumbnail cache miss, fetching from Steam", "workshop_id", id)

        mu := c.lockFor(id)
        mu.Lock()
        defer mu.Unlock()

        // Another goroutine may have populated the cache while we waited.
        if c.cached(id) {
                c.logger.Debug("thumbnail cache hit after lock (concurrent fetch)", "workshop_id", id)
                return path, nil
        }

        previewURL, err := c.previewURL(ctx, id)
        if err != nil {
                c.logger.Error("failed to get preview URL from Steam", "workshop_id", id, "error", err)
                return "", err
        }
        if previewURL == "" {
                c.logger.Warn("Steam has no preview image for workshop item", "workshop_id", id)
                return "", fmt.Errorf("steam: no preview image for workshop id %s", id)
        }

        c.logger.Info("downloading thumbnail", "workshop_id", id, "preview_url", previewURL)
        if err := c.download(ctx, previewURL, path); err != nil {
                c.logger.Error("thumbnail download failed", "workshop_id", id, "error", err)
                return "", err
        }
        c.logger.Info("thumbnail cached successfully", "workshop_id", id, "path", path)
        return path, nil
}

// Prefetch downloads thumbnails for the given workshop IDs in the background,
// coalescing with any in-flight requests. It returns immediately; failures are
// logged and otherwise ignored. This lets the panel warm the cache as soon as it
// learns which maps are installed, so the UI renders real thumbnails without
// waiting on Steam.
func (c *Client) Prefetch(ids []string) {
        if !c.Enabled() || len(ids) == 0 {
                return
        }
        c.logger.Info("prefetching thumbnails in background", "count", len(ids), "workshop_ids", ids)
        for _, id := range ids {
                if !validID.MatchString(id) {
                        c.logger.Warn("skipping invalid workshop ID in prefetch", "workshop_id", id)
                        continue
                }
                if c.cached(id) {
                        c.logger.Debug("skipping prefetch for already-cached thumbnail", "workshop_id", id)
                        continue
                }
                go func(id string) {
                        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
                        defer cancel()
                        if _, err := c.ThumbnailPath(ctx, id); err != nil {
                                c.logger.Warn("thumbnail prefetch failed", "workshop_id", id, "error", err)
                        }
                }(id)
        }
}

// publishedFileDetailsResponse models the subset of the
// IPublishedFileService/GetDetails response we care about.
type publishedFileDetailsResponse struct {
        Response struct {
                PublishedFileDetails []struct {
                        PublishedFileID string `json:"publishedfileid"`
                        Result          int    `json:"result"`
                        PreviewURL      string `json:"preview_url"`
                        Title           string `json:"title"`
                } `json:"publishedfiledetails"`
        } `json:"response"`
}

// previewURL resolves a workshop file ID to its preview image URL via the Steam
// Web API.
func (c *Client) previewURL(ctx context.Context, id string) (string, error) {
        endpoint := apiBase + "/IPublishedFileService/GetDetails/v1/"
        q := url.Values{}
        q.Set("key", c.apiKey)
        q.Set("publishedfileids[0]", id)

        reqURL := endpoint + "?" + q.Encode()
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
        if err != nil {
                return "", fmt.Errorf("steam: build request: %w", err)
        }

        resp, err := c.http.Do(req)
        if err != nil {
                return "", fmt.Errorf("steam: request details: %w", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
                return "", fmt.Errorf("steam: API rejected key (status %d) — check STEAM_API_KEY", resp.StatusCode)
        }
        if resp.StatusCode != http.StatusOK {
                return "", fmt.Errorf("steam: details request returned status %d", resp.StatusCode)
        }

        var details publishedFileDetailsResponse
        if err := json.NewDecoder(io.LimitReader(resp.Body, maxThumbnailBytes)).Decode(&details); err != nil {
                return "", fmt.Errorf("steam: decode details: %w", err)
        }
        if len(details.Response.PublishedFileDetails) == 0 {
                return "", fmt.Errorf("steam: no details returned for workshop id %s", id)
        }
        return strings.TrimSpace(details.Response.PublishedFileDetails[0].PreviewURL), nil
}

// download fetches src and writes it atomically to dst, creating the cache
// directory as needed. The write goes to a temp file that is renamed into place
// so readers never observe a partially written thumbnail.
func (c *Client) download(ctx context.Context, src, dst string) error {
        if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
                return fmt.Errorf("steam: create cache dir: %w", err)
        }

        req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
        if err != nil {
                return fmt.Errorf("steam: build image request: %w", err)
        }
        resp, err := c.http.Do(req)
        if err != nil {
                return fmt.Errorf("steam: download image: %w", err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
                return fmt.Errorf("steam: image download returned status %d", resp.StatusCode)
        }

        tmp, err := os.CreateTemp(filepath.Dir(dst), ".thumb-*")
        if err != nil {
                return fmt.Errorf("steam: create temp file: %w", err)
        }
        tmpName := tmp.Name()
        defer os.Remove(tmpName) // no-op after a successful rename

        n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxThumbnailBytes))
        if err != nil {
                tmp.Close()
                return fmt.Errorf("steam: write image: %w", err)
        }
        if err := tmp.Close(); err != nil {
                return fmt.Errorf("steam: close temp file: %w", err)
        }
        if n == 0 {
                return fmt.Errorf("steam: empty image body for %s", src)
        }

        if err := os.Rename(tmpName, dst); err != nil {
                return fmt.Errorf("steam: finalize thumbnail: %w", err)
        }
        return nil
}
