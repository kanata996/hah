package reqx

import "fmt"

type usageError struct {
	err error
}

func (e usageError) Error() string {
	return e.err.Error()
}

func (e usageError) Unwrap() error {
	return e.err
}

func usageErrorf(format string, args ...any) error {
	return usageError{err: fmt.Errorf("reqx: "+format, args...)}
}
