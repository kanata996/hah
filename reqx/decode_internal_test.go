package reqx

import (
	"encoding/json"
	"io"
	"reflect"
	"testing"
)

func TestDescribeJSONTypeCoversKinds(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{name: "nil", typ: nil, want: "valid value"},
		{name: "bool", typ: reflect.TypeOf(true), want: "boolean"},
		{name: "int", typ: reflect.TypeOf(int64(1)), want: "number"},
		{name: "uint", typ: reflect.TypeOf(uint(1)), want: "number"},
		{name: "float", typ: reflect.TypeOf(float64(1)), want: "number"},
		{name: "string", typ: reflect.TypeOf(""), want: "string"},
		{name: "array", typ: reflect.TypeOf([1]string{}), want: "array"},
		{name: "slice", typ: reflect.TypeOf([]string{}), want: "array"},
		{name: "map", typ: reflect.TypeOf(map[string]any{}), want: "object"},
		{name: "struct", typ: reflect.TypeOf(struct{ Name string }{}), want: "object"},
		{name: "pointer", typ: reflect.TypeOf(&struct{ Name string }{}), want: "object"},
		{name: "default", typ: reflect.TypeOf((chan int)(nil)), want: "chan int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeJSONType(tt.typ); got != tt.want {
				t.Fatalf("describeJSONType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseUnknownField(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		field string
		ok    bool
	}{
		{
			name:  "other error",
			err:   simpleError("other error"),
			field: "",
			ok:    false,
		},
		{
			name:  "formatted unknown field",
			err:   simpleError(`json: unknown field "extra"`),
			field: "extra",
			ok:    true,
		},
		{
			name:  "nil",
			err:   nil,
			field: "",
			ok:    false,
		},
		{
			name:  "wrong suffix",
			err:   simpleError(`json: unknown field "extra`),
			field: "",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := parseUnknownField(tt.err)
			if field != tt.field || ok != tt.ok {
				t.Fatalf("parseUnknownField() = (%q, %t), want (%q, %t)", field, ok, tt.field, tt.ok)
			}
		})
	}
}

func TestReadBodyBranches(t *testing.T) {
	t.Run("nil body", func(t *testing.T) {
		body, err := readBody(nil, DefaultMaxBodyBytes)
		if err != nil {
			t.Fatalf("readBody() error = %v, want nil", err)
		}
		if body != nil {
			t.Fatalf("body = %#v, want nil", body)
		}
	})

	t.Run("non-positive limit uses default", func(t *testing.T) {
		body, err := readBody(io.NopCloser(stringsReader("ok")), 0)
		if err != nil {
			t.Fatalf("readBody() error = %v, want nil", err)
		}
		if string(body) != "ok" {
			t.Fatalf("body = %q, want ok", body)
		}
	})

	t.Run("read error", func(t *testing.T) {
		_, err := readBody(errReaderCloser{err: simpleError("read failed")}, DefaultMaxBodyBytes)
		if err == nil || err.Error() != "read failed" {
			t.Fatalf("readBody() error = %v, want read failed", err)
		}
	})
}

func TestValidateJSONContentTypeBranches(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantNil     bool
	}{
		{name: "empty", contentType: "", wantNil: true},
		{name: "invalid media type", contentType: `application/json; charset="`, wantNil: false},
		{name: "json", contentType: "application/json", wantNil: true},
		{name: "plus json", contentType: "application/problem+json", wantNil: true},
		{name: "non json", contentType: "text/plain", wantNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJSONContentType(tt.contentType)
			if tt.wantNil && err != nil {
				t.Fatalf("validateJSONContentType() error = %v, want nil", err)
			}
			if !tt.wantNil && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestMapDecodeErrorBranches(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		err := mapDecodeError(&json.SyntaxError{})
		if err == nil || err.Error() != "request body must be valid JSON" {
			t.Fatalf("mapDecodeError() = %v, want invalid json error", err)
		}
	})

	t.Run("invalid unmarshal", func(t *testing.T) {
		original := &json.InvalidUnmarshalError{Type: reflect.TypeOf(0)}
		err := mapDecodeError(original)
		if err != original {
			t.Fatalf("mapDecodeError() = %v, want original error", err)
		}
	})

	t.Run("eof", func(t *testing.T) {
		err := mapDecodeError(io.EOF)
		if err == nil || err.Error() != "request body must not be empty" {
			t.Fatalf("mapDecodeError() = %v, want empty body error", err)
		}
	})
}

type simpleError string

func (e simpleError) Error() string {
	return string(e)
}

type errReaderCloser struct {
	err error
}

func (r errReaderCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r errReaderCloser) Close() error {
	return nil
}

type stringsReader string

func (r stringsReader) Read(p []byte) (int, error) {
	if len(r) == 0 {
		return 0, io.EOF
	}
	n := copy(p, string(r))
	if n < len(r) {
		return n, nil
	}
	return n, io.EOF
}
