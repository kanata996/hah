package reqx

import "net/http"

// Problem describes a structured request/input problem produced by reqx.
type Problem struct {
	status  int
	code    string
	message string
	details []any
}

// NewProblem constructs a structured request/input problem.
func NewProblem(status int, code, message string, details ...any) *Problem {
	return &Problem{
		status:  normalizeProblemStatus(status),
		code:    normalizeProblemCode(code),
		message: normalizeProblemMessage(message),
		details: cloneProblemDetails(details),
	}
}

func normalizeProblemStatus(status int) int {
	if status < 400 || status > 499 {
		return http.StatusBadRequest
	}
	return status
}

func normalizeProblemCode(code string) string {
	if code != "" {
		return code
	}
	return CodeRequestError
}

func normalizeProblemMessage(message string) string {
	if message != "" {
		return message
	}
	return "request error"
}

func cloneProblemDetails(details []any) []any {
	if len(details) == 0 {
		return nil
	}

	cloned := make([]any, len(details))
	copy(cloned, details)
	return cloned
}

// Error implements the error interface.
func (p *Problem) Error() string {
	if p == nil {
		return ""
	}
	return p.message
}

// Status returns the public HTTP status code.
func (p *Problem) Status() int {
	if p == nil {
		return http.StatusBadRequest
	}
	return p.status
}

// Code returns the stable machine-readable problem code.
func (p *Problem) Code() string {
	if p == nil {
		return normalizeProblemCode("")
	}
	return p.code
}

// Message returns the safe public problem message.
func (p *Problem) Message() string {
	if p == nil {
		return normalizeProblemMessage("")
	}
	return p.message
}

// Details returns a copy of the structured problem details.
func (p *Problem) Details() []any {
	if p == nil || len(p.details) == 0 {
		return nil
	}
	return cloneProblemDetails(p.details)
}
