package api

import (
        "net/http"
        "net/http/httptest"
        "testing"

        "github.com/codevski/defuse/internal/steam"
)

func TestParseWorkshopMaps(t *testing.T) {
        out := `
de_dust2
workshop/3070900859/de_cache_redux
workshop/3070900859/de_cache_redux
workshop\1234567890\aim_map.vpk
garbage line
`
        maps := parseWorkshopMaps(out)
        if len(maps) != 2 {
                t.Fatalf("got %d maps, want 2: %+v", len(maps), maps)
        }
        if maps[0].WorkshopID != "3070900859" || maps[0].Name != "de_cache_redux" {
                t.Errorf("unexpected first map: %+v", maps[0])
        }
        if maps[1].WorkshopID != "1234567890" || maps[1].Name != "aim_map" {
                t.Errorf("unexpected second map (want .vpk stripped): %+v", maps[1])
        }
}

func TestParseWorkshopMaps_Empty(t *testing.T) {
        maps := parseWorkshopMaps("no workshop maps here")
        if maps == nil {
                t.Fatal("expected non-nil empty slice for JSON encoding")
        }
        if len(maps) != 0 {
                t.Errorf("expected 0 maps, got %d", len(maps))
        }
}

func TestThumbnailHandler_DisabledReturns404(t *testing.T) {
        h := Wrap(newTestLogger(), ThumbnailHandler(steam.New("", t.TempDir(), nil)))

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
        h := Wrap(newTestLogger(), ThumbnailHandler(client))

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
