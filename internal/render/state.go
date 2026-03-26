package render

import (
	"context"
	"net/http"
)

type State struct {
	Status      int
	ContentType string
	Responded   bool
}

type stateKey struct{}

func EnsureState(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	if StateFrom(r) != nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), stateKey{}, &State{}))
}

func StateFrom(r *http.Request) *State {
	if r == nil {
		return nil
	}
	state, _ := r.Context().Value(stateKey{}).(*State)
	return state
}

func Status(r *http.Request, status int) {
	if status == 0 {
		return
	}
	if state := ensureAttachedState(r); state != nil {
		state.Status = status
	}
}

func ResponseStarted(r *http.Request) bool {
	state := StateFrom(r)
	return state != nil && state.Responded
}

func MarkResponseStarted(r *http.Request) {
	if state := ensureAttachedState(r); state != nil {
		state.Responded = true
	}
}

func statusOrDefault(r *http.Request, fallback int) int {
	state := StateFrom(r)
	if state != nil && state.Status != 0 {
		return state.Status
	}
	return fallback
}

func contentTypeOrDefault(r *http.Request, fallback string) string {
	state := StateFrom(r)
	if state != nil && state.ContentType != "" {
		return state.ContentType
	}
	return fallback
}

func ensureAttachedState(r *http.Request) *State {
	if r == nil {
		return nil
	}

	if state := StateFrom(r); state != nil {
		return state
	}

	withState := EnsureState(r)
	*r = *withState
	return StateFrom(r)
}
