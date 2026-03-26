package hah

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
)

var errorLogger = log.New(os.Stderr, "", log.LstdFlags)
var stackTrace = debug.Stack

type defaultReportKind uint8

const (
	defaultReportKindSkip defaultReportKind = iota
	defaultReportKindSecurity
	defaultReportKindInternal
)

type defaultReportContext struct {
	report     ErrorReport
	method     string
	target     string
	remoteAddr string
}

func defaultErrorReporter(report ErrorReport) {
	kind := classifyDefaultReport(report)
	if kind == defaultReportKindSkip {
		return
	}

	ctx := newDefaultReportContext(report)

	switch kind {
	case defaultReportKindSecurity:
		errorLogger.Print(formatSecurityEventLog(ctx))
	case defaultReportKindInternal:
		errorLogger.Print(formatInternalErrorLog(ctx))
		errorLogger.Print(formatInternalErrorStackLog(ctx, stackTrace()))
	}
}

func classifyDefaultReport(report ErrorReport) defaultReportKind {
	if report.PublicError == nil {
		return defaultReportKindSkip
	}

	status := report.PublicError.Status()
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return defaultReportKindSecurity
	}
	if status < 500 {
		return defaultReportKindSkip
	}

	return defaultReportKindInternal
}

func newDefaultReportContext(report ErrorReport) defaultReportContext {
	method, target, remoteAddr := requestContextFields(report.Request)
	return defaultReportContext{
		report:     report,
		method:     method,
		target:     target,
		remoteAddr: remoteAddr,
	}
}

func formatInternalErrorLog(ctx defaultReportContext) string {
	return fmt.Sprintf(
		"hah: internal error handled: err=%v err_type=%T status=%d code=%s method=%s target=%s remote=%s request_id=%s started=%t",
		ctx.report.Error,
		ctx.report.Error,
		ctx.report.PublicError.Status(),
		ctx.report.PublicError.Code(),
		ctx.method,
		ctx.target,
		ctx.remoteAddr,
		ctx.report.RequestID,
		ctx.report.ResponseStarted,
	)
}

func formatInternalErrorStackLog(ctx defaultReportContext, stack []byte) string {
	return fmt.Sprintf(
		"hah: internal error stack: request_id=%s\n%s",
		ctx.report.RequestID,
		stack,
	)
}

func formatSecurityEventLog(ctx defaultReportContext) string {
	return fmt.Sprintf(
		"hah: security event handled: err=%v err_type=%T status=%d code=%s method=%s target=%s remote=%s request_id=%s started=%t",
		ctx.report.Error,
		ctx.report.Error,
		ctx.report.PublicError.Status(),
		ctx.report.PublicError.Code(),
		ctx.method,
		ctx.target,
		ctx.remoteAddr,
		ctx.report.RequestID,
		ctx.report.ResponseStarted,
	)
}

func requestContextFields(r *http.Request) (method, target, remoteAddr string) {
	if r != nil {
		method = r.Method
		switch {
		case r.Pattern != "":
			target = r.Pattern
		case r.RequestURI != "":
			target = r.RequestURI
		case r.URL != nil:
			target = r.URL.RequestURI()
		}
		remoteAddr = r.RemoteAddr
	}
	return method, target, remoteAddr
}
