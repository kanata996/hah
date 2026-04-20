package reqx

import "fmt"

func usageErrorf(format string, args ...any) error {
	return fmt.Errorf("reqx: "+format, args...)
}
