package workshop

import (
        "context"
        "log/slog"
        "os"
        "path/filepath"
        "reflect"
        "sort"
        "testing"
)

func discardLogger() *slog.Logger {
        return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), nil))
}

// --- parsers ---------------------------------------------------------------

func TestParseListmaps(t *testing.T) {
        out := `
de_drachenschanze
aim_redline
de_drachenschanze
workshop/3070900859/de_cache_redux
some_map.vpk
Unknown thing with spaces
`
        got := ParseListmaps(out)
        want := []string{"de_drachenschanze", "aim_redline", "de_cache_redux", "some_map"}
        if !reflect.DeepEqual(got, want) {
                t.Fatalf("ParseListmaps = %v, want %v", got, want)
        }
}

func TestParseListmaps_Empty(t *testing.T) {
        if got := ParseListmaps(""); len(got) != 0 {
                t.Fatalf("expected empty, got %v", got)
        }
}

func TestParsePathMaps(t *testing.T) {
        out := `
de_dust2
workshop/3070900859/de_cache_redux
workshop/3070900859/de_cache_redux
workshop\1234567890\aim_map.vpk
garbage
`
        maps := ParsePathMaps(out)
        if len(maps) != 2 {
                t.Fatalf("got %d, want 2: %+v", len(maps), maps)
        }
        if maps[0].WorkshopID != "3070900859" || maps[0].Name != "de_cache_redux" {
                t.Errorf("first: %+v", maps[0])
        }
        if maps[1].WorkshopID != "1234567890" || maps[1].Name != "aim_map" {
                t.Errorf("second (want .vpk stripped): %+v", maps[1])
        }
}

func TestLooksLikeUnknownCommand(t *testing.T) {
        if !looksLikeUnknownCommand("Unknown command: ds_workshop_listmaps") {
                t.Error("should detect unknown command")
        }
        if looksLikeUnknownCommand("de_drachenschanze") {
                t.Error("false positive")
        }
}

func TestParseStatusMap(t *testing.T) {
        cases := map[string]string{
                "hostname: x\nmap     : de_dust2\nplayers : 0": "de_dust2",
                "map     : workshop/3070900859/de_cache_redux": "de_cache_redux",
                "map: aim_redline.vpk":                         "aim_redline",
                "no map line here":                             "",
        }
        for in, want := range cases {
                if got := ParseStatusMap(in); got != want {
                        t.Errorf("ParseStatusMap(%q) = %q, want %q", in, got, want)
                }
        }
}

// --- filesystem provider ---------------------------------------------------

func makeWorkshopDir(t *testing.T, root, id, mapName string) {
        t.Helper()
        dir := filepath.Join(root, id)
        if err := os.MkdirAll(dir, 0o755); err != nil {
                t.Fatal(err)
        }
        if err := os.WriteFile(filepath.Join(dir, mapName+".vpk"), []byte("x"), 0o644); err != nil {
                t.Fatal(err)
        }
}

func TestFilesystemProvider_ListAndMultiVersion(t *testing.T) {
        root := t.TempDir()
        makeWorkshopDir(t, root, "111", "de_drachenschanze")
        makeWorkshopDir(t, root, "222", "aim_redline")
        makeWorkshopDir(t, root, "333", "aim_redline") // second version of same name
        // a non-numeric dir that must be ignored
        _ = os.MkdirAll(filepath.Join(root, "notanid"), 0o755)

        p := newFilesystemProvider(root, discardLogger())
        if p.Mode() != "filesystem" {
                t.Fatalf("mode = %q", p.Mode())
        }
        maps, err := p.List(context.Background())
        if err != nil {
                t.Fatal(err)
        }

        byName := map[string]Map{}
        for _, m := range maps {
                byName[m.Name] = m
                if m.Source != SourceScanned {
                        t.Errorf("%s source = %q, want scanned", m.Name, m.Source)
                }
        }
        if len(maps) != 2 {
                t.Fatalf("got %d unique names, want 2: %+v", len(maps), maps)
        }
        if byName["de_drachenschanze"].WorkshopID != "111" {
                t.Errorf("drachenschanze id = %q", byName["de_drachenschanze"].WorkshopID)
        }
        redline := byName["aim_redline"]
        got := append([]string{}, redline.Versions...)
        sort.Strings(got)
        if !reflect.DeepEqual(got, []string{"222", "333"}) {
                t.Errorf("aim_redline versions = %v, want [222 333]", redline.Versions)
        }
}

func TestFilesystemProvider_WriteProbeAndUninstall(t *testing.T) {
        root := t.TempDir()
        makeWorkshopDir(t, root, "111", "de_test")

        p := newFilesystemProvider(root, discardLogger())
        if !p.Writable() {
                t.Fatal("temp dir should be writable")
        }
        if err := p.Uninstall(context.Background(), "111"); err != nil {
                t.Fatalf("uninstall: %v", err)
        }
        if _, err := os.Stat(filepath.Join(root, "111")); !os.IsNotExist(err) {
                t.Error("workshop dir should be gone after uninstall")
        }
        // uninstalling a missing id errors
        if err := p.Uninstall(context.Background(), "999"); err == nil {
                t.Error("expected error uninstalling missing id")
        }
        // non-numeric id rejected
        if err := p.Uninstall(context.Background(), "abc"); err == nil {
                t.Error("expected error for non-numeric id")
        }
}

func TestFilesystemProvider_ReadOnlyBlocksUninstall(t *testing.T) {
        root := t.TempDir()
        makeWorkshopDir(t, root, "111", "de_test")
        p := &fsProvider{root: root, writable: false, logger: discardLogger()}
        if err := p.Uninstall(context.Background(), "111"); err == nil {
                t.Error("read-only mount must refuse uninstall")
        }
}

// --- rcon provider ---------------------------------------------------------

type fakeRCON struct {
        responses map[string]string
        err       error
}

func (f *fakeRCON) Execute(id, cmd string) (string, error)      { return f.responses[cmd], f.err }
func (f *fakeRCON) ExecuteMulti(id, cmd string) (string, error) { return f.responses[cmd], f.err }

type fakeCache struct {
        names map[string]string // name(lower) -> id
}

func (c *fakeCache) WorkshopIDForName(_, name string) (string, bool, error) {
        id, ok := c.names[name]
        return id, ok, nil
}
func (c *fakeCache) UpsertWorkshopMap(_, _, _ string) error  { return nil }
func (c *fakeCache) SetWorkshopMapName(_, _, _ string) error { return nil }
func (c *fakeCache) DeleteWorkshopMap(_, _ string) error     { return nil }

func TestRCONProvider_InstantVsInstalled(t *testing.T) {
        rc := &fakeRCON{responses: map[string]string{
                "ds_workshop_listmaps": "de_drachenschanze\naim_redline\n",
        }}
        cache := &fakeCache{names: map[string]string{"aim_redline": "3123456789"}}

        p := newRCONProvider(rc, cache, "srv", discardLogger())
        if p.Mode() != "rcon" || p.Writable() {
                t.Fatalf("mode/writable wrong: %q %v", p.Mode(), p.Writable())
        }
        maps, err := p.List(context.Background())
        if err != nil {
                t.Fatal(err)
        }
        if len(maps) != 2 {
                t.Fatalf("got %d maps: %+v", len(maps), maps)
        }
        got := map[string]Map{}
        for _, m := range maps {
                got[m.Name] = m
        }
        if got["de_drachenschanze"].Source != SourceInstalled || got["de_drachenschanze"].WorkshopID != "" {
                t.Errorf("drachenschanze should be installed/no-id: %+v", got["de_drachenschanze"])
        }
        if got["aim_redline"].Source != SourceInstant || got["aim_redline"].WorkshopID != "3123456789" {
                t.Errorf("aim_redline should be instant with id: %+v", got["aim_redline"])
        }
}

func TestRCONProvider_FallsBackToMapsStar(t *testing.T) {
        rc := &fakeRCON{responses: map[string]string{
                "ds_workshop_listmaps": "Unknown command: ds_workshop_listmaps",
                "maps *":               "workshop/555/de_fallback\n",
        }}
        p := newRCONProvider(rc, &fakeCache{names: map[string]string{}}, "srv", discardLogger())
        maps, err := p.List(context.Background())
        if err != nil {
                t.Fatal(err)
        }
        if len(maps) != 1 || maps[0].WorkshopID != "555" || maps[0].Name != "de_fallback" {
                t.Fatalf("fallback parse wrong: %+v", maps)
        }
        if maps[0].Source != SourceInstant {
                t.Errorf("fallback map should be instant (has id): %+v", maps[0])
        }
}

func TestRCONProvider_UninstallRefused(t *testing.T) {
        p := newRCONProvider(&fakeRCON{}, &fakeCache{}, "srv", discardLogger())
        if err := p.Uninstall(context.Background(), "111"); err == nil {
                t.Error("rcon mode must refuse uninstall")
        }
}

// --- resolver --------------------------------------------------------------

func TestResolve_PicksFilesystemWhenMounted(t *testing.T) {
        root := t.TempDir()
        t.Setenv(envKeyForID("my-srv"), root)
        p := Resolve("my-srv", &fakeRCON{}, &fakeCache{}, discardLogger())
        if p.Mode() != "filesystem" {
                t.Fatalf("mode = %q, want filesystem", p.Mode())
        }
}

func TestResolve_FallsBackToRCON(t *testing.T) {
        t.Setenv("WORKSHOP_BASE", filepath.Join(t.TempDir(), "does-not-exist"))
        p := Resolve("ghost", &fakeRCON{}, &fakeCache{}, discardLogger())
        if p.Mode() != "rcon" {
                t.Fatalf("mode = %q, want rcon", p.Mode())
        }
}
