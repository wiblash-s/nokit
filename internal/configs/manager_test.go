package configs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeStore is an in-memory ConfigStore for panel-mode tests.
type fakeStore struct {
	data map[string]map[string]string // serverID -> name -> content
}

func newFakeStore() *fakeStore { return &fakeStore{data: map[string]map[string]string{}} }

func (f *fakeStore) ListPanelConfigs(serverID string) ([]PanelConfig, error) {
	var out []PanelConfig
	for name, content := range f.data[serverID] {
		out = append(out, PanelConfig{ServerID: serverID, Name: name, Content: content})
	}
	return out, nil
}

func (f *fakeStore) GetPanelConfig(serverID, name string) (PanelConfig, error) {
	if c, ok := f.data[serverID][name]; ok {
		return PanelConfig{ServerID: serverID, Name: name, Content: c}, nil
	}
	return PanelConfig{}, os.ErrNotExist
}

func (f *fakeStore) SavePanelConfig(serverID, name, content string) error {
	if f.data[serverID] == nil {
		f.data[serverID] = map[string]string{}
	}
	f.data[serverID][name] = content
	return nil
}

func (f *fakeStore) DeletePanelConfig(serverID, name string) error {
	delete(f.data[serverID], name)
	return nil
}

func TestPanelModeRoundTrip(t *testing.T) {
	t.Setenv("CONFIG_BASE", t.TempDir()) // empty base -> no mount for "srv"
	m := NewManager(newFakeStore())

	mode, writable := m.GetMode("srv")
	if mode != ModePanel || !writable {
		t.Fatalf("expected panel/writable, got %s/%v", mode, writable)
	}

	if err := m.SaveConfig("srv", "practice.cfg", "sv_cheats 1\n"); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := m.GetConfig("srv", "practice.cfg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "sv_cheats 1\n" || got.Mode != ModePanel {
		t.Fatalf("unexpected config: %+v", got)
	}
	list, err := m.ListConfigs("srv")
	if err != nil || len(list) != 1 || list[0].Name != "practice.cfg" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if err := m.DeleteConfig("srv", "practice.cfg"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestMountedMode(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CONFIG_BASE", base)
	srvDir := filepath.Join(base, "srv")
	if err := os.MkdirAll(srvDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)

	mode, writable := m.GetMode("srv")
	if mode != ModeMount || !writable {
		t.Fatalf("expected mounted/writable, got %s/%v", mode, writable)
	}
	if err := m.SaveConfig("srv", "server.cfg", "mp_roundtime 1.92\n"); err != nil {
		t.Fatalf("save: %v", err)
	}
	// File must exist on disk.
	if _, err := os.Stat(filepath.Join(srvDir, "server.cfg")); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}
	list, err := m.ListConfigs("srv")
	if err != nil || len(list) != 1 || list[0].Mode != ModeMount {
		t.Fatalf("list: %v %+v", err, list)
	}
}

func TestValidNameRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"", "../etc/passwd", "sub/x.cfg", "x.txt", "..\\x.cfg"} {
		if err := validName(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
	if err := validName("practice.cfg"); err != nil {
		t.Errorf("practice.cfg should be valid: %v", err)
	}
}

func TestExecLines(t *testing.T) {
	content := `// header comment
sv_cheats 1

mp_roundtime 1.92  // inline comment
say "hello // not a comment"
   // indented comment
bot_add_ct
`
	got := ExecLines(content)
	want := []string{
		"sv_cheats 1",
		"mp_roundtime 1.92",
		`say "hello // not a comment"`,
		"bot_add_ct",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExecLines mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestStripExt(t *testing.T) {
	if StripExt("practice.cfg") != "practice" {
		t.Fatal("expected practice")
	}
	if StripExt("noext") != "noext" {
		t.Fatal("expected noext")
	}
}
