package hah

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/internal/resp"
)

func TestDefaultErrorReporterSkipsNilAndNonInternalReports(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := errorLogger.Writer()
	errorLogger.SetOutput(&logs)
	defer errorLogger.SetOutput(previousWriter)

	defaultErrorReporter(ErrorReport{})
	defaultErrorReporter(ErrorReport{
		Error:       errors.New("bad request"),
		PublicError: NewHTTPError(http.StatusBadRequest, "invalid_request", "invalid request"),
	})

	if logs.Len() != 0 {
		t.Fatalf("logs = %q, want empty", logs.String())
	}
}

func TestDefaultErrorReporterLogsInternalErrorContext(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := errorLogger.Writer()
	previousStackTrace := stackTrace
	errorLogger.SetOutput(&logs)
	stackTrace = func() []byte { return []byte("stack line 1\nstack line 2\n") }
	defer func() {
		errorLogger.SetOutput(previousWriter)
		stackTrace = previousStackTrace
	}()

	req := httptest.NewRequest(http.MethodPost, "/users?trace=1", nil)
	req.RemoteAddr = "127.0.0.1:8080"

	defaultErrorReporter(ErrorReport{
		Request:         req,
		Error:           errors.New("db down"),
		PublicError:     NewHTTPError(http.StatusInternalServerError, "internal_error", "internal server error"),
		RequestID:       "req_ctx",
		ResponseStarted: true,
	})

	output := logs.String()
	if !strings.Contains(output, "internal error handled: err=db down") {
		t.Fatalf("logs = %q, want internal log", output)
	}
	if !strings.Contains(output, "err_type=*errors.errorString") {
		t.Fatalf("logs = %q, want error type", output)
	}
	if !strings.Contains(output, "method=POST") {
		t.Fatalf("logs = %q, want method", output)
	}
	if !strings.Contains(output, "target=/users?trace=1") {
		t.Fatalf("logs = %q, want target", output)
	}
	if !strings.Contains(output, "remote=127.0.0.1:8080") {
		t.Fatalf("logs = %q, want remote addr", output)
	}
	if !strings.Contains(output, "internal error stack: request_id=req_ctx") {
		t.Fatalf("logs = %q, want internal stack header", output)
	}
	if !strings.Contains(output, "stack line 1") {
		t.Fatalf("logs = %q, want stack payload", output)
	}
}

func TestDefaultErrorReporterLogsSecurityEvent(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := errorLogger.Writer()
	errorLogger.SetOutput(&logs)
	defer errorLogger.SetOutput(previousWriter)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.RemoteAddr = "127.0.0.1:8080"

	defaultErrorReporter(ErrorReport{
		Request:         req,
		Error:           errors.New("missing role"),
		PublicError:     NewHTTPError(http.StatusForbidden, "forbidden", "forbidden"),
		RequestID:       "req_sec",
		ResponseStarted: false,
	})

	output := logs.String()
	if !strings.Contains(output, "security event handled: err=missing role") {
		t.Fatalf("logs = %q, want security log", output)
	}
	if !strings.Contains(output, "status=403") {
		t.Fatalf("logs = %q, want 403 status", output)
	}
	if !strings.Contains(output, "code=forbidden") {
		t.Fatalf("logs = %q, want forbidden code", output)
	}
	if !strings.Contains(output, "target=/admin") {
		t.Fatalf("logs = %q, want target", output)
	}
	if strings.Contains(output, "internal error stack") {
		t.Fatalf("logs = %q, want no stack for security event", output)
	}
}

func TestDefaultErrorReporterLogsWriteResponseDegradation(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := errorLogger.Writer()
	errorLogger.SetOutput(&logs)
	defer errorLogger.SetOutput(previousWriter)

	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.RemoteAddr = "127.0.0.1:8080"

	defaultErrorReporter(ErrorReport{
		Request:         req,
		Error:           &resp.ErrorWriteDegraded{Cause: errors.New("json: unsupported type: func()"), PreservedPublicResponse: true},
		PublicError:     NewHTTPError(http.StatusBadRequest, "invalid_request", "request is invalid"),
		RequestID:       "req_write",
		ResponseStarted: true,
	})

	output := logs.String()
	if !strings.Contains(output, "error response degraded") {
		t.Fatalf("logs = %q, want degraded log", output)
	}
	if !strings.Contains(output, "preserved=true") {
		t.Fatalf("logs = %q, want preserved=true", output)
	}
	if !strings.Contains(output, "code=invalid_request") {
		t.Fatalf("logs = %q, want invalid_request code", output)
	}
}

func TestClassifyDefaultReport(t *testing.T) {
	tests := []struct {
		name         string
		report       ErrorReport
		wantKind     defaultReportKind
		wantDegraded bool
	}{
		{
			name:         "skip nil public error",
			report:       ErrorReport{},
			wantKind:     defaultReportKindSkip,
			wantDegraded: false,
		},
		{
			name: "classify write degradation first",
			report: ErrorReport{
				Error:       &resp.ErrorWriteDegraded{Cause: errors.New("json: unsupported type: func()"), PreservedPublicResponse: true},
				PublicError: NewHTTPError(http.StatusBadRequest, "invalid_request", "request is invalid"),
			},
			wantKind:     defaultReportKindDegradation,
			wantDegraded: true,
		},
		{
			name: "classify security event",
			report: ErrorReport{
				Error:       errors.New("missing role"),
				PublicError: NewHTTPError(http.StatusForbidden, "forbidden", "forbidden"),
			},
			wantKind:     defaultReportKindSecurity,
			wantDegraded: false,
		},
		{
			name: "skip ordinary client error",
			report: ErrorReport{
				Error:       errors.New("bad request"),
				PublicError: NewHTTPError(http.StatusBadRequest, "invalid_request", "invalid request"),
			},
			wantKind:     defaultReportKindSkip,
			wantDegraded: false,
		},
		{
			name: "classify internal error",
			report: ErrorReport{
				Error:       errors.New("db down"),
				PublicError: NewHTTPError(http.StatusInternalServerError, "internal_error", "internal server error"),
			},
			wantKind:     defaultReportKindInternal,
			wantDegraded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyDefaultReport(tt.report)
			if got.kind != tt.wantKind {
				t.Fatalf("classifyDefaultReport().kind = %v, want %v", got.kind, tt.wantKind)
			}
			if (got.degraded != nil) != tt.wantDegraded {
				t.Fatalf("classifyDefaultReport().degraded present = %t, want %t", got.degraded != nil, tt.wantDegraded)
			}
			if shouldLogStack(got) != (tt.wantKind == defaultReportKindInternal) {
				t.Fatalf("shouldLogStack() = %t, want %t", shouldLogStack(got), tt.wantKind == defaultReportKindInternal)
			}
		})
	}
}

func TestFormatInternalErrorLogIncludesDerivedRequestContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users?trace=1", nil)
	req.RemoteAddr = "127.0.0.1:8080"

	ctx := newDefaultReportContext(ErrorReport{
		Request:         req,
		Error:           errors.New("db down"),
		PublicError:     NewHTTPError(http.StatusInternalServerError, "internal_error", "internal server error"),
		RequestID:       "req_ctx",
		ResponseStarted: true,
	})

	line := formatInternalErrorLog(ctx)
	if !strings.Contains(line, "method=POST") {
		t.Fatalf("log line = %q, want method", line)
	}
	if !strings.Contains(line, "target=/users?trace=1") {
		t.Fatalf("log line = %q, want target", line)
	}
	if !strings.Contains(line, "remote=127.0.0.1:8080") {
		t.Fatalf("log line = %q, want remote", line)
	}
}

func TestRequestContextFieldsPrefersRoutePatternOverRequestTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/123?trace=1", nil)
	req.Pattern = "GET /users/{userID}"
	req.RemoteAddr = "127.0.0.1:8080"

	method, target, remoteAddr := requestContextFields(req)
	if method != http.MethodGet {
		t.Fatalf("method = %q, want %q", method, http.MethodGet)
	}
	if target != "GET /users/{userID}" {
		t.Fatalf("target = %q, want route pattern", target)
	}
	if remoteAddr != "127.0.0.1:8080" {
		t.Fatalf("remoteAddr = %q, want 127.0.0.1:8080", remoteAddr)
	}
}

func TestRequestContextFieldsFallsBackToRequestURI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?trace=1", nil)
	req.RequestURI = "/users?trace=1"

	_, target, _ := requestContextFields(req)
	if target != "/users?trace=1" {
		t.Fatalf("target = %q, want request URI", target)
	}
}

func TestRequestContextFieldsFallsBackToURLWhenRequestURIMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?trace=1", nil)
	req.RequestURI = ""

	_, target, _ := requestContextFields(req)
	if target != "/users?trace=1" {
		t.Fatalf("target = %q, want URL request URI", target)
	}
}

func TestRequestContextFieldsGuardsNil(t *testing.T) {
	method, target, remoteAddr := requestContextFields(nil)
	if method != "" || target != "" || remoteAddr != "" {
		t.Fatalf("requestContextFields(nil) = (%q, %q, %q), want empty values", method, target, remoteAddr)
	}
}
