package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

func newTestService(t *testing.T, slots int) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	key := make([]byte, secure.KeySize)
	_, _ = rand.Read(key)
	aead, err := secure.NewAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st, aead, audit.New(st, io.Discard), Config{
		SessionIdleTimeout: time.Minute, SessionMaxLifetime: time.Hour, PendingTimeout: time.Minute,
		MaxLoginAttempts: 6, LockoutDuration: 30 * time.Minute, MaxConcurrentHashes: slots,
	})
}

func TestHashSlotsBoundConcurrency(t *testing.T) {
	t.Parallel()
	s := newTestService(t, 2)
	var cur, peak atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			err := s.withHashSlot(context.Background(), func() error {
				n := cur.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				cur.Add(-1)
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if p := peak.Load(); p > 2 {
		t.Errorf("peak concurrency %d, want at most 2", p)
	}
}

func TestHashSlotsBusy(t *testing.T) {
	t.Parallel()
	s := newTestService(t, 1)
	s.hashWait = 20 * time.Millisecond
	s.hashSlots <- struct{}{} // the only slot is taken

	if err := s.withHashSlot(context.Background(), func() error { return nil }); !errors.Is(err, ErrBusy) {
		t.Errorf("got %v, want ErrBusy", err)
	}
	// Every password path goes through the slots.
	if _, _, _, err := s.Login(context.Background(), "nobody", "password123456", Meta{}); !errors.Is(err, ErrBusy) {
		t.Errorf("Login: got %v, want ErrBusy", err)
	}
	if err := s.SetPassword(context.Background(), &store.User{Username: "alice"}, "password123456", true); !errors.Is(err, ErrBusy) {
		t.Errorf("SetPassword: got %v, want ErrBusy", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.hashWait = time.Minute
	if err := s.withHashSlot(ctx, func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled context: got %v", err)
	}

	<-s.hashSlots // free the slot
	if err := s.withHashSlot(context.Background(), func() error { return nil }); err != nil {
		t.Errorf("free slot: %v", err)
	}
}
