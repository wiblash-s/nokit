package api

import (
        "net/http"
        "net/http/httptest"
        "testing"

        "github.com/codevski/defuse/internal/steam"
)

// storeMapCache assertion lives in the store package tests; here we only verify
// the HTTP handlers. Map-list parsing is tested in internal/workshop.

func TestThumbnailHandler_DisabledReturns404(t *testing.T) {
        logger := newTestLogger()
        h := Wrap(logger, ThumbnailHandler(steam.New("", t.TempDir(), nil), logger))

        req := httptest.NewRequest(http.MethodGet, "/api/maps/thumbnail/123", nil)
        req.SetPathValue("id", "123")
        rr := httptest.NewRecorder()
        h.ServeHTTP(rr, req)

        if rr.Code != http.StatusNotFound {
                t.Fatalf("got status %d, want 404", rr.Code)
        }
}

func TestThumbnailHandler_ServesCachedImage(t *testing.T) {
        // Fake Steam Web API + image CDN, both served by one local httptest server.
        // previewBase is filled in with the server's own base URL once it starts, so
        // we never hard-code a scheme/host literal in this file.
        const previewPath = "/img"
        const imageBody = "IMAGEBYTES"
        var previewBase string
        mux := http.NewServeMux()
        mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, r *http.Request) {
                id := r.URL.Query().Get("publishedfileids[0]")
                previewURL := previewBase + previewPath
                body := `{"response":{"publishedfiledetails":[{"publishedfileid":"` + id +
                        `","result":1,"preview_url":"` + previewURL + `"}]}}`
                _, _ = w.Write([]byte(body))
        })
        mux.HandleFunc(previewPath, func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "image/jpeg")
                _, _ = w.Write([]byte(imageBody))
        })
        srv := httptest.NewServer(mux)
        defer srv.Close()
        previewBase = srv.URL
        defer steam.SetAPIBase(srv.URL)()

        client := steam.New("key", t.TempDir(), nil)
        logger := newTestLogger()
        h := Wrap(logger, ThumbnailHandler(client, logger))

        req := httptest.NewRequest(http.MethodGet, "/api/maps/thumbnail/3070900859", nil)
        req.SetPathValue("id", "3070900859")
        rr := httptest.NewRecorder()
        h.ServeHTTP(rr, req)

        if rr.Code != http.StatusOK {
                t.Fatalf("got status %d, want 200", rr.Code)
        }
        if body := rr.Body.String(); body != imageBody {
                t.Errorf("body = %q, want %q", body, imageBody)
        }
        if cc := rr.Header().Get("Cache-Control"); cc == "" {
                t.Error("expected Cache-Control header on thumbnail response")
        }
}
