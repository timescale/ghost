package api

// Event is an interface representing the types that are streamed back from the
// GET /user/:userID/session/sessionID/events endpoint. The isEvent method is a
// no-op that gives us compile-time assurance that only the event types defined
// below implement the interface.,
type Event interface {
	isEvent()
}

type event struct{}

func (e event) isEvent() {}

// Status represents a session status value, as returned in a [StatusEvent].
type SessionStatus string

// Valid [SessionStatus] values.
const (
	StatusConnected SessionStatus = "connected"
	StatusClosed    SessionStatus = "closed"
	StatusError     SessionStatus = "error"
)

type ConnectedEvent struct {
	event
	Status SessionStatus `json:"status"`
}

type ClosedEvent struct {
	event
	Status SessionStatus `json:"status"`
}

type ErrorEvent struct {
	event
	Status SessionStatus    `json:"status"`
	Error  *NormalizedError `json:"error"`
}
