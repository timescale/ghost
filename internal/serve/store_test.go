package serve

import (
	"testing"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

func TestRunStore_AddGetDelete(t *testing.T) {
	s := newRunStore()
	if got := s.get("missing"); got != nil {
		t.Errorf("get(missing) = %v, want nil", got)
	}

	r := &Run{id: "abc"}
	s.add(r)
	if got := s.get("abc"); got != r {
		t.Errorf("get(abc) = %v, want %v", got, r)
	}

	s.delete("abc")
	if got := s.get("abc"); got != nil {
		t.Errorf("get(abc) after delete = %v, want nil", got)
	}
}

func TestRun_SetErrorIsIdempotent(t *testing.T) {
	r := &Run{done: make(chan struct{})}
	first := &dbdriver.NormalizedError{Message: "first"}
	second := &dbdriver.NormalizedError{Message: "second"}

	r.setError(first)
	r.setError(second)

	if r.err != first {
		t.Errorf("err = %v, want first call to win", r.err)
	}
}

func TestRun_CloseDoneIsIdempotent(t *testing.T) {
	r := &Run{done: make(chan struct{})}
	r.closeDone()
	r.closeDone() // must not panic on double-close
	select {
	case <-r.done:
	default:
		t.Fatal("done channel should be closed")
	}
}

func TestSessionStore_CloseAllReleasesEverything(t *testing.T) {
	s := newSessionStore()
	s.add(&Session{id: "a", closed: make(chan struct{})})
	s.add(&Session{id: "b", closed: make(chan struct{})})

	s.closeAll()

	if s.get("a") != nil || s.get("b") != nil {
		t.Errorf("sessions remain after closeAll")
	}
}
