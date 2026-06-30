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

func TestBridgeWaitForActiveClientAlreadyActive(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	drainStatus(t, c)

	// A client is already active, so the wait returns immediately.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := b.WaitForActiveClient(ctx); err != nil {
		t.Fatalf("WaitForActiveClient = %v, want nil", err)
	}
}

func TestBridgeWaitForActiveClientBecomesActive(t *testing.T) {
	b := NewBridge()

	// No client yet; the wait blocks until one connects and becomes active.
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- b.WaitForActiveClient(ctx)
	}()

	// Give the waiter a moment to block on the activation channel, then connect.
	time.Sleep(20 * time.Millisecond)
	c := b.addClient()
	drainStatus(t, c)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForActiveClient = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForActiveClient did not return after a client became active")
	}
}

func TestBridgeWaitForActiveClientContextCanceled(t *testing.T) {
	b := NewBridge()

	// No client ever connects; the wait unblocks when the context is canceled
	// (rather than blocking until the much longer idle timeout).
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.WaitForActiveClient(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForActiveClient = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForActiveClient did not return after context cancellation")
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

	// The browser reported the error itself, so it's already done with the
	// command — no cancel should be sent back to it.
	select {
	case ev := <-c.events:
		t.Fatalf("expected no cancel after a browser-reported error, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

// drainCancel waits for a "cancel" event for the expected request ID on the
// client's stream, skipping any interleaved status events. The cancel is sent
// asynchronously by the dispatching Request goroutine once it observes the
// failed result, so it can arrive before or after the takeover's status
// broadcast — the relative order is not guaranteed (and doesn't matter, since
// the browser handles status and cancel independently).
func drainCancel(t *testing.T, c *agentClient, requestID string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case ev := <-c.events:
			if ev.Type == "status" {
				continue
			}
			if ev.Type != "cancel" {
				t.Fatalf("expected cancel event, got %+v", ev)
			}
			if ev.RequestID != requestID {
				t.Fatalf("expected cancel for %q, got %q", requestID, ev.RequestID)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for cancel event")
		}
	}
}

// TestBridgeRequestCancelsBrowserOnContextCancel verifies that canceling the
// caller's context after the command was dispatched sends a cancel event to
// the client (so the browser aborts its in-flight query) and fails the request.
func TestBridgeRequestCancelsBrowserOnContextCancel(t *testing.T) {
	b := NewBridge()
	c := b.addClient()
	drainStatus(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(ctx, "visualize", nil)
		errCh <- err
	}()

	cmd := waitForCommand(t, c)
	cancel()

	drainCancel(t, c, cmd.ID)
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not fail on context cancel")
	}
}

// TestBridgeSupersedeCancelsBrowser verifies that a takeover sends a cancel
// event to the superseded (still-connected) client so it aborts its in-flight
// query, in addition to failing the request with ErrClientSuperseded.
func TestBridgeSupersedeCancelsBrowser(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	drainStatus(t, c1)
	c2 := b.addClient()
	drainStatus(t, c2)
	drainStatus(t, c1)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "visualize", nil)
		errCh <- err
	}()

	cmd := waitForCommand(t, c1)
	b.Activate(c2.id)

	// c1 receives a cancel for the in-flight command, interleaved with the
	// takeover's status broadcast (drainCancel skips status events).
	drainCancel(t, c1, cmd.ID)
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClientSuperseded) {
			t.Fatalf("expected ErrClientSuperseded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not fail on supersede")
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

// TestBridgeRequestDoesNotWedgeOnFullBuffer verifies that a request can still
// fail fast (here, via a takeover resolving p.result) even when the active
// client's outbound event buffer is full — i.e. the command-dispatch send step
// can't wedge the request. This models a browser whose SSE stream is stuck so
// its buffer never drains.
func TestBridgeRequestDoesNotWedgeOnFullBuffer(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	// Fill c1's event buffer to capacity (without draining it), so the command
	// dispatch send in Request would block.
	for {
		select {
		case c1.events <- agentServerEvent{Type: "status"}:
		default:
			goto full
		}
	}
full:
	// A second client connects so we have someone to take over control. The
	// broadcast to the full c1 buffer is dropped (non-blocking), as designed.
	c2 := b.addClient()

	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "uiState", nil)
		errCh <- err
	}()

	// Let the request reach (and block on) the dispatch send.
	time.Sleep(50 * time.Millisecond)
	// Take over with c2; this resolves the in-flight request even though the
	// dispatch send to c1 is blocked on its full buffer.
	if !b.Activate(c2.id) {
		t.Fatal("activate should succeed for a known client")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClientSuperseded) {
			t.Fatalf("expected ErrClientSuperseded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request wedged on a full client buffer")
	}
}

// TestBridgeSupersedeCancelSurvivesFullBuffer verifies that the cancel sent to
// a superseded client is delivered reliably even when that client's outbound
// buffer was full at supersede time. A best-effort (droppable) cancel would be
// lost here, letting the browser run an abandoned command once its buffer
// drained and the already-queued command was delivered; sendCancelReliably
// blocks until the cancel is enqueued instead.
func TestBridgeSupersedeCancelSurvivesFullBuffer(t *testing.T) {
	b := NewBridge()
	c1 := b.addClient()
	drainStatus(t, c1)
	c2 := b.addClient()
	drainStatus(t, c2)
	drainStatus(t, c1)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "visualize", nil)
		errCh <- err
	}()

	cmd := waitForCommand(t, c1)

	// Fill c1's buffer to capacity so a best-effort cancel would be dropped.
	for {
		select {
		case c1.events <- agentServerEvent{Type: "status"}:
		default:
			goto full
		}
	}
full:
	// Take over: the request fails and Request must reliably hand c1 a cancel.
	b.Activate(c2.id)

	// Drain c1's stream while awaiting the failed result. Both must be done
	// concurrently: Request's goroutine blocks in sendCancelReliably until c1's
	// (full) buffer makes room, so it can't report the error until this drain
	// loop — standing in for the SSE handler — consumes events. A best-effort
	// cancel would have been dropped at supersede time; here it must appear.
	deadline := time.After(2 * time.Second)
	var gotErr error
	var gotCancel bool
	for !(gotCancel && gotErr != nil) {
		select {
		case ev := <-c1.events:
			if ev.Type == "cancel" {
				if ev.RequestID != cmd.ID {
					t.Fatalf("cancel for %q, want %q", ev.RequestID, cmd.ID)
				}
				gotCancel = true
			}
		case err := <-errCh:
			gotErr = err
		case <-deadline:
			if !gotCancel {
				t.Fatal("cancel was dropped: never delivered to the superseded client")
			}
			t.Fatal("request did not fail on supersede")
		}
	}
	if !errors.Is(gotErr, ErrClientSuperseded) {
		t.Fatalf("expected ErrClientSuperseded, got %v", gotErr)
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
