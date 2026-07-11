package store

import (
        "path/filepath"
        "testing"
)

func openTestStore(t *testing.T) *Store {
        t.Helper()
        st, err := Open(filepath.Join(t.TempDir(), "test.db"))
        if err != nil {
                t.Fatalf("open store: %v", err)
        }
        t.Cleanup(func() { _ = st.Close() })
        return st
}

func TestWorkshopMapCache_UpsertLookupDelete(t *testing.T) {
        st := openTestStore(t)

        // Record an ID with no name yet (as the load handler does immediately).
        if err := st.UpsertWorkshopMap("srv", "111", ""); err != nil {
                t.Fatal(err)
        }
        if _, ok, err := st.WorkshopIDForName("srv", "de_test"); err != nil || ok {
                t.Fatalf("no name cached yet: ok=%v err=%v", ok, err)
        }

        // Reconcile fills the name in; empty upsert must not have clobbered anything.
        if err := st.SetWorkshopMapName("srv", "111", "de_test"); err != nil {
                t.Fatal(err)
        }
        id, ok, err := st.WorkshopIDForName("srv", "DE_TEST") // case-insensitive
        if err != nil || !ok || id != "111" {
                t.Fatalf("lookup = %q ok=%v err=%v, want 111", id, ok, err)
        }

        // An empty-name upsert on the same key must preserve the existing name.
        if err := st.UpsertWorkshopMap("srv", "111", ""); err != nil {
                t.Fatal(err)
        }
        if id, ok, _ := st.WorkshopIDForName("srv", "de_test"); !ok || id != "111" {
                t.Fatalf("name was clobbered by empty upsert: id=%q ok=%v", id, ok)
        }

        // Scoped per server.
        if _, ok, _ := st.WorkshopIDForName("other", "de_test"); ok {
                t.Fatal("lookup leaked across servers")
        }

        // Newest wins when two IDs share a name.
        if err := st.UpsertWorkshopMap("srv", "222", "de_test"); err != nil {
                t.Fatal(err)
        }
        if id, _, _ := st.WorkshopIDForName("srv", "de_test"); id != "222" {
                t.Errorf("newest-wins failed: got %q, want 222", id)
        }

        recs, err := st.ListWorkshopMaps("srv")
        if err != nil || len(recs) != 2 {
                t.Fatalf("list = %d recs err=%v, want 2", len(recs), err)
        }

        // Delete forgets the ID.
        if err := st.DeleteWorkshopMap("srv", "222"); err != nil {
                t.Fatal(err)
        }
        if id, _, _ := st.WorkshopIDForName("srv", "de_test"); id != "111" {
                t.Errorf("after delete, got %q, want 111", id)
        }
}
