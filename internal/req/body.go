package req

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

type key struct{}

type state struct {
	has bool
	err error
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}

// HasBody reports whether the request body contains at least one byte.
// It preserves the body stream for later consumers and caches the result on the request.
func HasBody(r *http.Request) (bool, error) {
	if r == nil {
		return false, nil
	}
	if r.Body == nil {
		return false, nil
	}

	if cached, ok := r.Context().Value(key{}).(state); ok {
		return cached.has, cached.err
	}

	has, err := detectBody(r)
	*r = *r.WithContext(context.WithValue(r.Context(), key{}, state{
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
