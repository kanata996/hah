package reqx

import (
	"errors"
	"testing"
)

func TestUsageErrorf_PrefixesAndSupportsUnwrap(t *testing.T) {
	cause := errors.New("boom")

	err := usageErrorf("invalid config: %w", cause)
	if err == nil {
		t.Fatal("usageErrorf() error = nil, want non-nil")
	}
	if got := err.Error(); got != "reqx: invalid config: boom" {
		t.Fatalf("usageErrorf() error = %q, want %q", got, "reqx: invalid config: boom")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true")
	}
}
