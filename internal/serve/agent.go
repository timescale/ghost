package serve

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// agentIdleTimeout is how long the bridge waits without receiving any message
// (heartbeat or response) from the active client — both while waiting for a
// command response and while waiting for the first client to connect — before
// failing the in-flight request. The browser is expected to send heartbeats at
// a faster cadence (see the web orchestrator) while a command is in flight, so
// any genuine stall (tab closed, JS crashed) is caught within this window.
const agentIdleTimeout = 60 * time.Second

// agentClientBufferSize is the per-client outbound event buffer. Control
// events and commands are tiny and infrequent, so a small buffer is plenty.
const agentClientBufferSize = 8

// agentCancelGrace bounds how long sendCancelReliably waits to hand a cancel to
// a live client before giving up. The SSE handler drains the client channel
// continuously, so a healthy client accepts the cancel almost immediately; this
// only caps the wait for a wedged stream (a dead client cleaned up on
// disconnect), so the dispatching goroutine can't block on it indefinitely.
const agentCancelGrace = 5 * time.Second

// Bridge errors surfaced to MCP tools (and ultimately the agent).
var (
	// ErrNoActiveClient is returned when a request is attempted but no browser
	// client is connected to drive it.
	ErrNoActiveClient = errors.New("no active browser client is connected")
	// ErrClientDisconnected is returned when the active client disconnects
	// while a request is in flight.
	ErrClientDisconnected = errors.New("browser client disconnected")
	// ErrClientSuperseded is returned when control is taken over by another
	// client while a request was in flight.
	ErrClientSuperseded = errors.New("another browser tab took over control")
	// ErrAgentIdleTimeout is returned when the active client sends no message
	// (heartbeat or response) within agentIdleTimeout.
	ErrAgentIdleTimeout = errors.New("timed out waiting for the browser; is the tab still open?")
)

// agentServerEvent is a message sent from the server to a browser client over
// the SSE stream. Exactly one group of optional fields is populated depending
// on Type ("status", "command", or "cancel").
type agentServerEvent struct {
	Type string `json:"type"`
	// status events
	ClientID string `json:"clientId,omitempty"`
	Active   *bool  `json:"active,omitempty"`
	// command events
	Command *AgentCommand `json:"command,omitempty"`
	// cancel events: tells the client to abort the in-flight command with this
	// request ID (e.g. the MCP caller canceled, the request timed out, or
	// another tab took over). Without this, a browser-run query keeps going
	// after the bridge has already given up on the request.
	RequestID string `json:"requestId,omitempty"`
}

// sendCancelReliably notifies a client to abort the in-flight command with the
// given request ID, blocking until the cancel is enqueued, the client
// disconnects, or agentCancelGrace elapses. Unlike a best-effort (droppable)
// send, this guarantees delivery to a live client: the agent bridge relies on
// the cancel to stop an already-dispatched command, and a dropped cancel would
// let the browser run an abandoned command after its request had failed. The
// SSE handler drains the client's channel continuously, so this returns almost
// immediately in practice; the grace bound only protects against a wedged
// stream (such a client is dead and will be cleaned up on disconnect anyway).
// Must NOT be called while holding b.mu (it can block on the client channel).
func sendCancelReliably(c *agentClient, requestID string) {
	if c == nil {
		return
	}
	t := time.NewTimer(agentCancelGrace)
	defer t.Stop()
	select {
	case c.events <- agentServerEvent{Type: "cancel", RequestID: requestID}:
	case <-c.done:
	case <-t.C:
	}
}

// AgentCommand is a single command dispatched to the browser to execute (e.g.
// run a query, reconfigure the chart, read UI state). Payload is an opaque
// JSON blob whose shape depends on Type; the MCP tools and the web orchestrator
// agree on it.
type AgentCommand struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// agentClient is a single connected browser tab.
type agentClient struct {
	id     string
	events chan agentServerEvent
	done   chan struct{}
}

// pendingRequest tracks the single in-flight command and the client it was
// dispatched to.
type pendingRequest struct {
	id       string
	clientID string
	result   chan pendingResult
	beat     chan struct{}
}

type pendingResult struct {
	data json.RawMessage
	err  error
	// cancelBrowser is set when the request was abandoned while the browser is
	// still connected and may still be running the command (i.e. a takeover),
	// so Request must actively tell that client to abort. It is false when the
	// browser already delivered an outcome (success or error) or disconnected
	// (in which case there's nothing live to cancel).
	cancelBrowser bool
}

// Bridge is the communication channel between MCP tools (server side) and the
// browser UI (client side). Browser tabs subscribe to a Server-Sent Events
// stream; MCP tools dispatch commands to the single active client and await a
// response delivered back over a separate HTTP endpoint.
//
// At most one command is in flight at a time (callers serialize via Request).
// Exactly one connected client is "active" at any moment; commands are only
// dispatched to it and responses are only accepted from it.
type Bridge struct {
	mu       sync.Mutex
	clients  []*agentClient // ordered by connection time (oldest first)
	activeID string
	pending  *pendingRequest

	// activated is closed (and replaced) whenever a client becomes active, so
	// waiters in WaitForActiveClient wake without polling. Guarded by mu.
	activated chan struct{}

	// sem serializes Request calls to one in-flight command at a time.
	sem chan struct{}
}

// NewBridge creates an empty [Bridge].
func NewBridge() *Bridge {
	return &Bridge{
		activated: make(chan struct{}),
		sem:       make(chan struct{}, 1),
	}
}

// signalActivatedLocked wakes any WaitForActiveClient waiters by closing the
// current activation channel and installing a fresh one. Must be called with
// b.mu held, on every transition that makes a client active.
func (b *Bridge) signalActivatedLocked() {
	close(b.activated)
	b.activated = make(chan struct{})
}

// addClient registers a new browser connection and returns it. The first
// client to connect (when none is active) becomes active automatically;
// otherwise the incumbent active client is left in place (new tabs come up
// inactive and must explicitly take over). All clients are notified of their
// (possibly unchanged) active status.
func (b *Bridge) addClient() *agentClient {
	b.mu.Lock()
	defer b.mu.Unlock()

	c := &agentClient{
		id:     uuid.NewString(),
		events: make(chan agentServerEvent, agentClientBufferSize),
		done:   make(chan struct{}),
	}
	b.clients = append(b.clients, c)
	if b.activeID == "" {
		b.activeID = c.id
		b.signalActivatedLocked()
	}
	b.broadcastStatusLocked()
	return c
}

// removeClient deregisters a disconnected browser connection. If it was the
// active client, the most-recently-connected remaining client is promoted, and
// any in-flight request dispatched to the departed client is failed.
func (b *Bridge) removeClient(c *agentClient) {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := slices.Index(b.clients, c)
	if idx == -1 {
		return
	}
	// slices.Delete shifts the tail down and zeroes the vacated slot, so the
	// departed client (and its channels) can be garbage-collected.
	b.clients = slices.Delete(b.clients, idx, idx+1)
	close(c.done)

	if b.activeID != c.id {
		return
	}

	// The active client departed. Any in-flight request it owned is failed by
	// Request's own select on client.done (closed above) — that's the single
	// disconnect signal, so we don't resolve p.result here. Promote the
	// most-recently-connected remaining client (if any).
	if len(b.clients) > 0 {
		b.activeID = b.clients[len(b.clients)-1].id
		b.signalActivatedLocked()
	} else {
		b.activeID = ""
	}
	b.broadcastStatusLocked()
}

// Activate makes the given client the active one (the "take over" action). Any
// in-flight request dispatched to the previously-active client is failed. It is
// a no-op (returning false) if the client ID is unknown.
func (b *Bridge) Activate(clientID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !slices.ContainsFunc(b.clients, func(c *agentClient) bool {
		return c.id == clientID
	}) {
		return false
	}

	old := b.activeID
	if old == clientID {
		return true
	}
	b.activeID = clientID
	b.signalActivatedLocked()
	if b.pending != nil && b.pending.clientID == old {
		// Fail the request and flag it for browser cancellation: the superseded
		// client is still connected and may still be running the command, so the
		// dispatching Request goroutine reliably tells it to abort once it
		// observes this result. Doing the cancel there — rather than a
		// best-effort send here — guarantees it isn't dropped if the client's
		// buffer is momentarily full, which would otherwise let the browser run
		// a command whose request already failed.
		b.resolveLocked(b.pending, pendingResult{err: ErrClientSuperseded, cancelBrowser: true})
	}
	b.broadcastStatusLocked()
	return true
}

// HasActiveClient reports whether a client is currently connected and active.
func (b *Bridge) HasActiveClient() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activeClientLocked() != nil
}

// WaitForActiveClient blocks until an active client is connected, the context
// is canceled, or agentIdleTimeout elapses with no client. Returns nil once a
// client is active.
func (b *Bridge) WaitForActiveClient(ctx context.Context) error {
	timeout := time.NewTimer(agentIdleTimeout)
	defer timeout.Stop()
	for {
		// Grab the current activation channel under the lock and recheck after,
		// so an activation that races this check still wakes us (the channel is
		// only ever closed, never sent on, while mu is held).
		b.mu.Lock()
		if b.activeClientLocked() != nil {
			b.mu.Unlock()
			return nil
		}
		activated := b.activated
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return ErrNoActiveClient
		case <-activated:
			// A client became active (or the set changed); loop to recheck.
		}
	}
}

// Request dispatches a command of the given type (with the given JSON-
// serializable payload) to the active browser client and waits for its
// response. Calls are serialized: a second Request blocks until the first
// completes. The request fails if the active client disconnects, another tab
// takes over, or no message (heartbeat or response) arrives within
// agentIdleTimeout.
func (b *Bridge) Request(ctx context.Context, commandType string, payload any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Serialize to a single in-flight command.
	select {
	case b.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-b.sem }()

	cmd := AgentCommand{ID: uuid.NewString(), Type: commandType, Payload: raw}

	b.mu.Lock()
	client := b.activeClientLocked()
	if client == nil {
		b.mu.Unlock()
		return nil, ErrNoActiveClient
	}
	p := &pendingRequest{
		id:       cmd.ID,
		clientID: client.id,
		result:   make(chan pendingResult, 1),
		beat:     make(chan struct{}, 1),
	}
	b.pending = p
	b.mu.Unlock()

	// Ensure the pending slot is cleared on every exit path.
	defer b.clearPending(p)

	// Start the idle timer before dispatch so a stalled active client can't
	// wedge the request at the send step. If the client's event buffer is full
	// (its SSE handler is stuck writing to a slow/dead browser connection), the
	// send below would block; including the timer, p.result, and client.done in
	// this select guarantees the request still fails fast — on idle timeout, a
	// takeover/disconnect that resolves p.result, or the stream closing.
	timer := time.NewTimer(agentIdleTimeout)
	defer timer.Stop()

	// Dispatch the command to the client's SSE stream. The ctx/timeout paths
	// here return before the command was delivered, so there's nothing for the
	// browser to cancel yet.
	select {
	case client.events <- agentServerEvent{Type: "command", Command: &cmd}:
	case res := <-p.result:
		return res.data, res.err
	case <-client.done:
		return nil, ErrClientDisconnected
	case <-timer.C:
		return nil, ErrAgentIdleTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Past this point the command has been delivered to the client, so a path
	// that abandons it while the browser is still running it must reliably tell
	// the browser to abort — otherwise the browser would run a command whose
	// request has already failed. Request is the single owner of cancel
	// delivery; the resolving paths never send their own cancel.
	for {
		select {
		case res := <-p.result:
			// A browser-delivered outcome (success or error) needs no cancel —
			// the browser is already done. Only a takeover (cancelBrowser) leaves
			// the still-connected client running an abandoned command, so abort it.
			if res.cancelBrowser {
				sendCancelReliably(client, cmd.ID)
			}
			return res.data, res.err
		case <-p.beat:
			timer.Reset(agentIdleTimeout)
		case <-client.done:
			return nil, ErrClientDisconnected
		case <-timer.C:
			// The command was dispatched and the browser may still be running it
			// (e.g. a long query); tell it to abort before we give up.
			sendCancelReliably(client, cmd.ID)
			return nil, ErrAgentIdleTimeout
		case <-ctx.Done():
			// The MCP caller canceled/disconnected; stop the browser's work.
			sendCancelReliably(client, cmd.ID)
			return nil, ctx.Err()
		}
	}
}

// deliver routes a message received from a client (via POST /api/agent/respond)
// to the in-flight request. Messages are only accepted from the client the
// pending request was dispatched to. Heartbeats reset the idle timer; results
// and errors resolve the request.
func (b *Bridge) deliver(clientID, requestID string, msgType agentResponseType, data json.RawMessage, errMsg string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	p := b.pending
	if p == nil || p.id != requestID || p.clientID != clientID {
		return errors.New("no matching in-flight request")
	}

	switch msgType {
	case agentResponseHeartbeat:
		select {
		case p.beat <- struct{}{}:
		default:
		}
	case agentResponseResult:
		b.resolveLocked(p, pendingResult{data: data})
	case agentResponseError:
		msg := errMsg
		if msg == "" {
			msg = "browser reported an error"
		}
		b.resolveLocked(p, pendingResult{err: errors.New(msg)})
	default:
		return errors.New("unknown message type")
	}
	return nil
}

// resolveLocked delivers a result to the pending request and clears it. Must be
// called with b.mu held. Safe to call only once per request (subsequent calls
// no-op because b.pending is cleared).
func (b *Bridge) resolveLocked(p *pendingRequest, res pendingResult) {
	if b.pending != p {
		return
	}
	b.pending = nil
	p.result <- res
}

// clearPending drops the pending request if it is still the given one (e.g.
// after a timeout or context cancellation in Request).
func (b *Bridge) clearPending(p *pendingRequest) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == p {
		b.pending = nil
	}
}

// activeClientLocked returns the active client, or nil if none. Must be called
// with b.mu held.
func (b *Bridge) activeClientLocked() *agentClient {
	return b.clientByIDLocked(b.activeID)
}

// clientByIDLocked returns the client with the given ID, or nil if none (or the
// ID is empty). Must be called with b.mu held.
func (b *Bridge) clientByIDLocked(id string) *agentClient {
	if id == "" {
		return nil
	}
	for _, c := range b.clients {
		if c.id == id {
			return c
		}
	}
	return nil
}

// broadcastStatusLocked pushes a status event to every connected client
// reflecting whether it is currently the active client. Must be called with
// b.mu held. Uses a non-blocking send so a slow/stuck client can't wedge the
// bridge; such a client will fall behind and ultimately be cleaned up on
// disconnect.
func (b *Bridge) broadcastStatusLocked() {
	for _, c := range b.clients {
		active := c.id == b.activeID
		event := agentServerEvent{Type: "status", ClientID: c.id, Active: &active}
		select {
		case c.events <- event:
		default:
		}
	}
}
