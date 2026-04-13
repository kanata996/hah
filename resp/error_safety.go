package resp

import "errors"

func safeErrorsIs(err, target error) (matched bool) {
	defer func() {
		if recover() != nil {
			matched = false
		}
	}()
	return errors.Is(err, target)
}

func safeErrorsAs(err error, target any) (matched bool) {
	defer func() {
		if recover() != nil {
			matched = false
		}
	}()
	return errors.As(err, target)
}
