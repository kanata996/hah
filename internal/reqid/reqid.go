package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const generatedRequestIDPrefix = "req_"

var fallbackRequestIDCounter atomic.Uint64
var requestIDGenerator = defaultRequestIDGenerator
var requestIDEntropyRead = rand.Read

type State struct {
	mu sync.Mutex
	id string
}

type stateKey struct{}

func Set(r *http.Request, id string) *http.Request {
	if r == nil {
		return nil
	}

	id = normalize(id)
	if id == "" {
		return r
	}

	if current := StateFrom(r); current != nil {
		current.Set(id)
		return r
	}

	current := NewState()
	current.Set(id)
	return withState(r, current)
}

func NewState() *State {
	return &State{}
}

func Ensure(r *http.Request) (*http.Request, string) {
	if r == nil {
		return nil, requestIDGenerator()
	}

	if current := StateFrom(r); current != nil {
		return r, EnsureID(current)
	}

	current := NewState()
	return withState(r, current), EnsureID(current)
}

func EnsureState(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}

	if StateFrom(r) != nil {
		return r
	}

	return withState(r, NewState())
}

func EnsureID(current *State) string {
	if current == nil {
		return requestIDGenerator()
	}

	current.mu.Lock()
	defer current.mu.Unlock()

	if current.id == "" {
		current.id = requestIDGenerator()
	}
	return current.id
}

func withState(r *http.Request, current *State) *http.Request {
	if r == nil || current == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), stateKey{}, current))
}

func StateFrom(r *http.Request) *State {
	if r == nil {
		return nil
	}

	current, _ := r.Context().Value(stateKey{}).(*State)
	return current
}

func normalize(id string) string {
	return strings.TrimSpace(id)
}

func defaultRequestIDGenerator() string {
	var raw [16]byte
	if _, err := requestIDEntropyRead(raw[:]); err == nil {
		return generatedRequestIDPrefix + hex.EncodeToString(raw[:])
	}

	return fmt.Sprintf(
		"%s%d_%d",
		generatedRequestIDPrefix,
		time.Now().UnixNano(),
		fallbackRequestIDCounter.Add(1),
	)
}

func (s *State) Get() string {
	if s == nil {
		return ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

func (s *State) Set(id string) {
	if s == nil {
		return
	}

	id = normalize(id)
	if id == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
}
