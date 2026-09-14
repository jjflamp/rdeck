// Package events is the central event bus bridging Go core to the frontend,
// the counterpart of RESP.app's `Events` signal hub (src/app/events.h).
//
// Contract-first: every event name and payload shape listed here must have a
// matching TypeScript type in frontend/src/api/events.ts (see plan §4.3-2).
package events

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Bus struct {
	ctx      context.Context
	mu       sync.RWMutex
	silent   bool // true in unit tests / CLI mode when wails ctx is absent
}

func New() *Bus { return &Bus{} }

// Startup binds the wails context; called once from app:startup.
func (b *Bus) Startup(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ctx = ctx
}

// Emit publishes an event to the frontend. Safe before startup (no-op).
func (b *Bus) Emit(name string, data ...any) {
	b.mu.RLock()
	ctx, silent := b.ctx, b.silent
	b.mu.RUnlock()
	if ctx == nil || silent {
		return
	}
	runtime.EventsEmit(ctx, name, data...)
}

// Event names (keep in sync with frontend/src/api/events.ts).
const (
	ConnectionTested = "connection:test"
	Error            = "app:error"
	ConsoleMonitor   = "console:monitor" // [sessionID, []lines]
	PubsubMessage    = "pubsub:message"  // [connID, channel, [kind, channel, payload]]
	BulkProgress     = "bulk:progress"   // [runID, done, total, msg]
	BulkDone         = "bulk:done"       // [runID, summary map]
)
