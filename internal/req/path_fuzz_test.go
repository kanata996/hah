package req

import (
	"slices"
	"strings"
	"testing"
)

func FuzzPathPatternHelpers(f *testing.F) {
	seeds := []struct {
		pattern string
		name    string
	}{
		{pattern: "", name: "id"},
		{pattern: "GET /accounts/{id}", name: "id"},
		{pattern: "/files/{path...}", name: "path"},
		{pattern: "/accounts/{id:[0-9]+}", name: "id"},
		{pattern: "/{$}", name: "$"},
		{pattern: "/accounts/{id", name: "id"},
		{pattern: "/{{id}}", name: "id"},
		{pattern: "/files/{path...}/{", name: "path"},
	}

	for _, seed := range seeds {
		f.Add(seed.pattern, seed.name)
	}

	f.Fuzz(func(t *testing.T, pattern, name string) {
		names := PathWildcardNames(pattern)
		if !slices.Equal(names, PathWildcardNames(pattern)) {
			t.Fatalf("PathWildcardNames(%q) is not deterministic", pattern)
		}

		for _, wildcard := range names {
			if wildcard == "" {
				t.Fatalf("PathWildcardNames(%q) returned an empty wildcard", pattern)
			}
			if wildcard != strings.TrimSpace(wildcard) {
				t.Fatalf("PathWildcardNames(%q) returned untrimmed wildcard %q", pattern, wildcard)
			}
			if !PathHasWildcard(pattern, wildcard) {
				t.Fatalf("PathHasWildcard(%q, %q) = false, want true", pattern, wildcard)
			}
		}

		if strings.TrimSpace(name) == "" && PathHasWildcard(pattern, name) {
			t.Fatalf("PathHasWildcard(%q, %q) = true, want false for blank target", pattern, name)
		}
	})
}
