package reqx

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
		names := pathWildcardNames(pattern)
		if !slices.Equal(names, pathWildcardNames(pattern)) {
			t.Fatalf("pathWildcardNames(%q) is not deterministic", pattern)
		}

		for _, wildcard := range names {
			if wildcard == "" {
				t.Fatalf("pathWildcardNames(%q) returned an empty wildcard", pattern)
			}
			if wildcard != strings.TrimSpace(wildcard) {
				t.Fatalf("pathWildcardNames(%q) returned untrimmed wildcard %q", pattern, wildcard)
			}
			if !pathHasWildcard(pattern, wildcard) {
				t.Fatalf("pathHasWildcard(%q, %q) = false, want true", pattern, wildcard)
			}
		}

		if strings.TrimSpace(name) == "" && pathHasWildcard(pattern, name) {
			t.Fatalf("pathHasWildcard(%q, %q) = true, want false for blank target", pattern, name)
		}
	})
}
