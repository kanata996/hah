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

type state struct {
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

	if current := stateFrom(r); current != nil {
		current.set(id)
		return r
	}

	current := &state{}
	current.set(id)
	return withState(r, current)
}

func EnsureState(r *http.Request) *http.Request {
	if r == nil || stateFrom(r) != nil {
		return r
	}
	return withState(r, &state{})
}

func Ensure(r *http.Request) (*http.Request, string) {
	if r == nil {
		return nil, requestIDGenerator()
	}

	r = EnsureState(r)
	current := stateFrom(r)
	if id := current.get(); id != "" {
		return r, id
	}

	id := requestIDGenerator()
	current.set(id)
	return r, id
}

func withState(r *http.Request, current *state) *http.Request {
	if r == nil || current == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), stateKey{}, current))
}

func stateFrom(r *http.Request) *state {
	if r == nil {
		return nil
	}

	current, _ := r.Context().Value(stateKey{}).(*state)
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

func (s *state) get() string {
	if s == nil {
		return ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

func (s *state) set(id string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
}
