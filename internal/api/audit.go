package api

import (
        "net/http"
        "strconv"

        "github.com/codevski/defuse/internal/store"
)

// AuditStore is the subset of the store used for audit logging and querying.
type AuditStore interface {
        AppendAudit(sub, username, action, target, detail string) error
        ListAudit(limit int) ([]store.AuditEntry, error)
}

type auditEntryDTO struct {
        ID        int64  `json:"id"`
        Timestamp int64  `json:"timestamp"`
        Username  string `json:"username"`
        Sub       string `json:"sub"`
        Action    string `json:"action"`
        Target    string `json:"target"`
        Detail    string `json:"detail"`
}

// AuditLogHandler lists recent audit entries. Gated on the view_audit
// permission (superadmin) by the router.
func AuditLogHandler(st AuditStore) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                limit := 200
                if v := r.URL.Query().Get("limit"); v != "" {
                        if n, err := strconv.Atoi(v); err == nil && n > 0 {
                                limit = n
                        }
                }
                entries, err := st.ListAudit(limit)
                if err != nil {
                        return WrapHTTP(err, http.StatusInternalServerError, "could not load audit log")
                }
                out := make([]auditEntryDTO, 0, len(entries))
                for _, e := range entries {
                        out = append(out, auditEntryDTO{
                                ID:        e.ID,
                                Timestamp: e.CreatedAt.Unix(),
                                Username:  e.Username,
                                Sub:       e.Sub,
                                Action:    e.Action,
                                Target:    e.Target,
                                Detail:    e.Detail,
                        })
                }
                return JSON(w, http.StatusOK, map[string]any{"entries": out})
        }
}

// audit records an action for the current actor (best effort).
func audit(st AuditStore, r *http.Request, action, target, detail string) {
        sub, username := actor(r)
        _ = st.AppendAudit(sub, username, action, target, detail)
}

// Audited wraps a mutating handler so that, on success, the action is recorded
// in the audit log with the acting user's identity. The target is derived from
// the request path so each entry identifies what was changed.
func Audited(st AuditStore, action string, h Handler) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                err := h(w, r)
                if err == nil {
                        audit(st, r, action, r.URL.Path, "")
                }
                return err
        }
}
