package req

import (
	"reflect"
	"testing"
)

func TestPathWildcardNames(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{name: "blank", pattern: "   ", want: nil},
		{name: "no wildcard", pattern: "/accounts", want: []string{}},
		{name: "basic", pattern: "/accounts/{id}", want: []string{"id"}},
		{name: "method prefix", pattern: "GET /accounts/{id}", want: []string{"id"}},
		{name: "catch all", pattern: "/files/{path...}", want: []string{"path"}},
		{name: "typed wildcard", pattern: "/accounts/{id:[0-9]+}", want: []string{"id"}},
		{name: "skip dollar", pattern: "/{$}", want: []string{}},
		{name: "skip blank token", pattern: "/{ }", want: []string{}},
		{name: "multi", pattern: "/users/{user_id}/files/{path...}/{$}/{id:rest}", want: []string{"user_id", "path", "id"}},
		{name: "malformed", pattern: "/accounts/{id", want: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathWildcardNames(tc.pattern); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("PathWildcardNames(%q) = %#v, want %#v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestPathHasWildcard(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    bool
	}{
		{name: "blank pattern", pattern: "   ", target: "id", want: false},
		{name: "blank target", pattern: "/accounts/{id}", target: " ", want: false},
		{name: "basic hit", pattern: "/accounts/{id}", target: "id", want: true},
		{name: "basic miss", pattern: "/accounts/{id}", target: "slug", want: false},
		{name: "method prefix", pattern: "GET /accounts/{id}", target: "id", want: true},
		{name: "catch all", pattern: "/files/{path...}", target: "path", want: true},
		{name: "typed wildcard", pattern: "/accounts/{id:[0-9]+}", target: "id", want: true},
		{name: "skip dollar", pattern: "/{$}", target: "$", want: false},
		{name: "malformed", pattern: "/accounts/{id", target: "id", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathHasWildcard(tc.pattern, tc.target); got != tc.want {
				t.Fatalf("PathHasWildcard(%q, %q) = %v, want %v", tc.pattern, tc.target, got, tc.want)
			}
		})
	}
}
