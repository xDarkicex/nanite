package sse

import (
	"context"
	"sync"
	"time"
)

// Hub manages a pool of SSE connections and their keep-alive ping schedules.
type Hub struct {
	mu          sync.RWMutex
	connections map[*Connection]struct{}
	wheel       *timingWheel
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewHub creates a new initialized Hub.
func NewHub(ctx context.Context, pingInterval time.Duration) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	h := &Hub{
		connections: make(map[*Connection]struct{}),
		ctx:         ctx,
		cancel:      cancel,
		wheel:       newTimingWheel(pingInterval),
	}
	go h.run()
	return h
}

// Register adds a connection to the hub.
func (h *Hub) Register(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[conn] = struct{}{}
	h.wheel.add(conn)
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, conn)
	h.wheel.remove(conn)
}

// Broadcast sends raw bytes to all connected clients.
func (h *Hub) Broadcast(raw []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.connections {
		_ = conn.SendBytes(raw)
	}
}

// Stop gracefully shuts down the hub and its timing wheel.
func (h *Hub) Stop() {
	h.cancel()
}

func (h *Hub) run() {
	ticker := time.NewTicker(time.Second) // Base tick interval
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.wheel.tick()
		}
	}
}

// timingWheel is a simple O(1) slotted wheel for keep-alive pings.
// For simplicity in keeping CC < 10, we implement a basic ring buffer.
type timingWheel struct {
	mu       sync.Mutex
	slots    [][]*Connection
	current  int
	interval time.Duration
	size     int
}

func newTimingWheel(interval time.Duration) *timingWheel {
	size := int(interval.Seconds())
	if size <= 0 {
		size = 1
	}
	return &timingWheel{
		slots:    make([][]*Connection, size),
		size:     size,
		interval: interval,
	}
}

func (tw *timingWheel) add(conn *Connection) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	
	// Add to the slot right before the current one (farthest in the future)
	target := (tw.current + tw.size - 1) % tw.size
	tw.slots[target] = append(tw.slots[target], conn)
	conn.wheelSlot = target
}

func (tw *timingWheel) remove(conn *Connection) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	
	slot := conn.wheelSlot
	if slot < 0 || slot >= tw.size {
		return
	}
	
	conns := tw.slots[slot]
	for i, c := range conns {
		if c == conn {
			// Fast remove
			conns[i] = conns[len(conns)-1]
			tw.slots[slot] = conns[:len(conns)-1]
			conn.wheelSlot = -1
			break
		}
	}
}

func (tw *timingWheel) tick() {
	tw.mu.Lock()
	connsToPing := tw.slots[tw.current]
	tw.slots[tw.current] = nil // Clear current slot
	
	// Move current pointer
	tw.current = (tw.current + 1) % tw.size
	tw.mu.Unlock()

	for _, conn := range connsToPing {
		_ = conn.ping()
		tw.add(conn) // Re-queue
	}
}
