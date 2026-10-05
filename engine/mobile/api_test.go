package zapret

import (
	"sync"
	"testing"
	"time"
)

type capLogger struct {
	mu    sync.Mutex
	lines []string
}

func (c *capLogger) Log(line string) {
	c.mu.Lock()
	c.lines = append(c.lines, line)
	c.mu.Unlock()
}

func (c *capLogger) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.lines)
}

// TestStartSocksDoesNotDeadlock guards against the reentrant-lock regression
// where StartTunnel/StartSocks held the state mutex while logf tried to take it
// again (the engine appeared to start but never returned to the caller).
func TestStartSocksDoesNotDeadlock(t *testing.T) {
	e := NewEngine()
	lg := &capLogger{}
	e.SetLogger(lg)

	done := make(chan error, 1)
	go func() {
		done <- e.StartSocks("127.0.0.1:0", "split:2")
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartSocks returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartSocks deadlocked (did not return within 3s)")
	}

	// The "listening" line is logged from inside StartSocks while the state lock
	// is held; reaching it proves logf does not contend on the same lock.
	if lg.count() == 0 {
		t.Fatal("expected a log line from StartSocks")
	}
	e.Stop()
}

func TestValidateStrategy(t *testing.T) {
	if msg := ValidateStrategy("tlsrec:sni,split:1"); msg != "" {
		t.Fatalf("expected valid, got %q", msg)
	}
	if msg := ValidateStrategy("nonsense:xyz"); msg == "" {
		t.Fatal("expected an error message for a bad strategy")
	}
}
