package reqx

import "fmt"

func errorsf(format string, args ...any) error {
	return fmt.Errorf("reqx: "+format, args...)
}
