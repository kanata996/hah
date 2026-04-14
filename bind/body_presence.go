package bind

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

type bodyPresenceKey struct{}

type bodyPresenceState struct {
	has bool
	err error
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}

// hasBody reports whether the request body contains at least one byte.
// It preserves the body stream for later consumers and caches the result on the request.
func hasBody(r *http.Request) (bool, error) {
	if r == nil {
		return false, nil
	}
	if r.Body == nil {
		return false, nil
	}

	if state, ok := r.Context().Value(bodyPresenceKey{}).(bodyPresenceState); ok {
		return state.has, state.err
	}

	has, err := detectBody(r)
	*r = *r.WithContext(context.WithValue(r.Context(), bodyPresenceKey{}, bodyPresenceState{
		has: has,
		err: err,
	}))
	return has, err
}

func detectBody(r *http.Request) (bool, error) {
	if r.Body == nil {
		return false, nil
	}

	body := r.Body
	var prefix [1]byte
	n, err := body.Read(prefix[:])
	if err != nil && err != io.EOF {
		if n > 0 {
			r.Body = &replayReadCloser{
				Reader: io.MultiReader(bytes.NewReader(prefix[:n]), body),
				Closer: body,
			}
		}
		return false, err
	}

	r.Body = &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(prefix[:n]), body),
		Closer: body,
	}
	return n > 0, nil
}
