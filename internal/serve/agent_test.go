package serve

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// drainStatus reads the initial status event a client receives on connect,
// returning whether it is active. Fails if no event arrives promptly.
func drainStatus(t *testing.T, c *agentClient) bool {
	t.Helper()
	select {
	case ev := <-c.events:
		if ev.Type != "status" || ev.Active == nil {
			t.Fatalf("expected status event, got %+v", ev)
		}
		return *ev.Active
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for status event")
		return false
	}
}

func TestBridgeFirstClientBecomesActive(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	if !drainStatus(t, c) {
		t.Fatal("first client should be active")
	}
	if !b.HasActiveClient() {
		t.Fatal("bridge should report an active client")
	}
}

func TestBridgeIncumbentStaysActive(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	if !drainStatus(t, c1) {
		t.Fatal("c1 should be active")
	}

	// A second client connects; the incumbent stays active.
	c2 := b.addClient()
	if drainStatus(t, c2) {
		t.Fatal("c2 should be inactive (incumbent stays)")
	}
	// c1 receives an updated (still active) status on the broadcast.
	if !drainStatus(t, c1) {
		t.Fatal("c1 should remain active")
	}
}

func TestBridgeTakeover(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	drainStatus(t, c1)
	c2 := b.addClient()
	drainStatus(t, c2) // inactive
	drainStatus(t, c1) // still active

	if !b.Activate(c2.id) {
		t.Fatal("activate should succeed for a known client")
	}
	// Both clients get a new status; c2 active, c1 inactive (order: c1 then c2).
	if drainStatus(t, c1) {
		t.Fatal("c1 should be inactive after takeover")
	}
	if !drainStatus(t, c2) {
		t.Fatal("c2 should be active after takeover")
	}
}

func TestBridgePromotionOnDisconnect(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	drainStatus(t, c1)
	c2 := b.addClient()
	drainStatus(t, c2)
	drainStatus(t, c1)

	// Active client (c1) disconnects; c2 is promoted.
	b.removeClient(c1)
	if !drainStatus(t, c2) {
		t.Fatal("c2 should be promoted to active")
	}
}

func TestBridgeRequestRoundTrip(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	drainStatus(t, c)

	resultCh := make(chan struct {
		data json.RawMessage
		err  error
	}, 1)
	go func() {
		data, err := b.Request(context.Background(), "uiState", map[string]int{"limit": 50})
		resultCh <- struct {
			data json.RawMessage
			err  error
		}{data, err}
	}()

	// The client receives the command on its stream.
	var cmd AgentCommand
	select {
	case ev := <-c.events:
		if ev.Type != "command" || ev.Command == nil {
			t.Fatalf("expected command event, got %+v", ev)
		}
		cmd = *ev.Command
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command")
	}

	// Deliver a heartbeat then a result.
	if err := b.deliver(c.id, cmd.ID, "heartbeat", nil, ""); err != nil {
		t.Fatalf("heartbeat deliver failed: %v", err)
	}
	if err := b.deliver(c.id, cmd.ID, "result", json.RawMessage(`{"ok":true}`), ""); err != nil {
		t.Fatalf("result deliver failed: %v", err)
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("request failed: %v", res.err)
		}
		if string(res.data) != `{"ok":true}` {
			t.Fatalf("unexpected result data: %s", res.data)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not complete")
	}
}

func TestBridgeRequestErrorFromClient(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	drainStatus(t, c)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "chart", nil)
		errCh <- err
	}()

	cmd := waitForCommand(t, c)
	if err := b.deliver(c.id, cmd.ID, "error", nil, "boom"); err != nil {
		t.Fatalf("error deliver failed: %v", err)
	}
	select {
	case err := <-errCh:
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected 'boom' error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not complete")
	}
}

func TestBridgeRequestNoActiveClient(t *testing.T) {
	b := NewBridge()
	_, err := b.Request(context.Background(), "uiState", nil)
	if !errors.Is(err, ErrNoActiveClient) {
		t.Fatalf("expected ErrNoActiveClient, got %v", err)
	}
}

func TestBridgeRequestFailsOnDisconnect(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	drainStatus(t, c)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "uiState", nil)
		errCh <- err
	}()

	waitForCommand(t, c)
	b.removeClient(c)

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClientDisconnected) {
			t.Fatalf("expected ErrClientDisconnected, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not fail on disconnect")
	}
}

func TestBridgeRequestFailsOnSupersede(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	drainStatus(t, c1)
	c2 := b.addClient()
	drainStatus(t, c2)
	drainStatus(t, c1)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "uiState", nil)
		errCh <- err
	}()

	waitForCommand(t, c1)
	// c2 takes over while the request to c1 is in flight.
	b.Activate(c2.id)

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClientSuperseded) {
			t.Fatalf("expected ErrClientSuperseded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not fail on supersede")
	}
}

func TestBridgeStaleResponseRejected(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	drainStatus(t, c)

	// No in-flight request: any delivery is rejected.
	if err := b.deliver(c.id, "nonexistent", "result", nil, ""); err == nil {
		t.Fatal("expected error delivering to nonexistent request")
	}
}

// waitForCommand reads the next command event off a client's stream.
func waitForCommand(t *testing.T, c *agentClient) AgentCommand {
	t.Helper()
	select {
	case ev := <-c.events:
		if ev.Type != "command" || ev.Command == nil {
			t.Fatalf("expected command event, got %+v", ev)
		}
		return *ev.Command
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command")
		return AgentCommand{}
	}
}
