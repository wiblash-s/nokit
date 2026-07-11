package store

import (
        "errors"
        "testing"
)

func TestPanelConfigCRUD(t *testing.T) {
        st := openTestStore(t)

        // Missing config -> ErrNotFound.
        if _, err := st.GetPanelConfig("srv", "missing.cfg"); !errors.Is(err, ErrNotFound) {
                t.Fatalf("expected ErrNotFound, got %v", err)
        }

        // Create.
        if err := st.SavePanelConfig("srv", "practice.cfg", "sv_cheats 1\n"); err != nil {
                t.Fatalf("save: %v", err)
        }
        got, err := st.GetPanelConfig("srv", "practice.cfg")
        if err != nil {
                t.Fatalf("get: %v", err)
        }
        if got.Content != "sv_cheats 1\n" || got.Name != "practice.cfg" || got.ServerID != "srv" {
                t.Fatalf("unexpected record: %+v", got)
        }
        if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
                t.Fatalf("timestamps not parsed: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
        }

        // Update (upsert) content.
        if err := st.SavePanelConfig("srv", "practice.cfg", "sv_cheats 0\n"); err != nil {
                t.Fatalf("update: %v", err)
        }
        got, _ = st.GetPanelConfig("srv", "practice.cfg")
        if got.Content != "sv_cheats 0\n" {
                t.Fatalf("expected updated content, got %q", got.Content)
        }

        // Second config + list is scoped and ordered by name.
        if err := st.SavePanelConfig("srv", "aim.cfg", "bot_kick\n"); err != nil {
                t.Fatalf("save aim: %v", err)
        }
        if err := st.SavePanelConfig("other", "x.cfg", ""); err != nil {
                t.Fatalf("save other: %v", err)
        }
        list, err := st.ListPanelConfigs("srv")
        if err != nil {
                t.Fatalf("list: %v", err)
        }
        if len(list) != 2 || list[0].Name != "aim.cfg" || list[1].Name != "practice.cfg" {
                t.Fatalf("unexpected list: %+v", list)
        }

        // Delete.
        if err := st.DeletePanelConfig("srv", "practice.cfg"); err != nil {
                t.Fatalf("delete: %v", err)
        }
        if _, err := st.GetPanelConfig("srv", "practice.cfg"); !errors.Is(err, ErrNotFound) {
                t.Fatalf("expected ErrNotFound after delete, got %v", err)
        }
}
