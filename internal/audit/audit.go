// Package audit records security relevant events (PCI DSS requirement 10).
//
// Events are persisted in the local database (shown in the UI) and mirrored
// as JSON lines to a writer, so they can be shipped to a SIEM.
package audit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

const (
	Success = "success"
	Failure = "failure"
)

type Logger struct {
	store *store.Store
	out   io.Writer
	mu    sync.Mutex
}

func New(st *store.Store, out io.Writer) *Logger {
	return &Logger{store: st, out: out}
}

func (l *Logger) Record(ctx context.Context, e store.AuditEvent) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	// The event must be kept even if the client went away mid-request.
	ctx = context.WithoutCancel(ctx)
	if err := l.store.InsertAudit(ctx, &e); err != nil {
		slog.Error("audit: failed to persist event", "err", err, "action", e.Action)
	}
	if l.out == nil {
		return
	}
	line, err := json.Marshal(struct {
		Type string `json:"type"`
		store.AuditEvent
	}{"audit", e})
	if err != nil {
		slog.Error("audit: failed to encode event", "err", err)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.out.Write(append(line, '\n')); err != nil {
		slog.Error("audit: failed to write event", "err", err)
	}
}
