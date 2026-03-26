package hah

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/reqx"
)

// The helpers in this file are a thin facade over the reqx subpackage.
//
// The intended user-facing path is to keep HTTP boundary code on the hah
// import surface, for example via hah.DecodeJSON/hah.DecodeQuery/hah.Validate.
// reqx remains the implementation package for request decoding and validation,
// while hah adapts reqx.Problem into boundary errors that can be written via
// WriteError like any other public HTTP error.

// DecodeOption customizes JSON decoding behavior.
type DecodeOption = reqx.DecodeOption

// QueryOption customizes URL query decoding behavior.
type QueryOption = reqx.QueryOption

// Violation describes a single request field validation problem.
type Violation = reqx.Violation

// ValidateFunc validates a decoded request value.
type ValidateFunc[T any] func(*T) []Violation

// WithMaxBodyBytes limits the number of bytes read from the request body.
func WithMaxBodyBytes(limit int64) DecodeOption {
	return reqx.WithMaxBodyBytes(limit)
}

// AllowUnknownFields disables strict unknown-field rejection for JSON decoding.
func AllowUnknownFields() DecodeOption {
	return reqx.AllowUnknownFields()
}

// AllowEmptyBody permits an empty JSON request body.
func AllowEmptyBody() DecodeOption {
	return reqx.AllowEmptyBody()
}

// AllowUnknownQueryFields disables strict unknown-field rejection for query
// parameters.
func AllowUnknownQueryFields() QueryOption {
	return reqx.AllowUnknownQueryFields()
}

// DecodeJSON decodes a JSON request body into dst and returns hah-compatible
// public errors for request-shape failures.
func DecodeJSON[T any](r *http.Request, dst *T, opts ...DecodeOption) error {
	return adaptReqxProblem(reqx.DecodeJSON(r, dst, opts...))
}

// DecodeAndValidateJSON decodes a JSON request body, then runs validation.
func DecodeAndValidateJSON[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...DecodeOption) error {
	if err := DecodeJSON(r, dst, opts...); err != nil {
		return err
	}
	return Validate(dst, fn)
}

// DecodeQuery decodes URL query parameters into `query`-tagged struct fields in
// dst and returns hah-compatible public errors for request-shape failures.
func DecodeQuery[T any](r *http.Request, dst *T, opts ...QueryOption) error {
	return adaptReqxProblem(reqx.DecodeQuery(r, dst, opts...))
}

// DecodeAndValidateQuery decodes URL query parameters, then runs validation.
func DecodeAndValidateQuery[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...QueryOption) error {
	if err := DecodeQuery(r, dst, opts...); err != nil {
		return err
	}
	return Validate(dst, fn)
}

// Validate applies a validation function and returns a standardized 422
// error when violations are present.
func Validate[T any](dst *T, fn ValidateFunc[T]) error {
	return adaptReqxProblem(reqx.Validate(dst, reqx.ValidateFunc[T](fn)))
}

// adaptReqxProblem keeps the root-package facade thin: reqx owns request
// decoding/validation, while hah turns reqx.Problem into a boundary error that
// flows through WriteError like any other public HTTP error.
func adaptReqxProblem(err error) error {
	if err == nil {
		return nil
	}

	var problem *reqx.Problem
	if errors.As(err, &problem) && problem != nil {
		return NewHTTPError(problem.Status(), problem.Code(), problem.Message(), problem.Details()...)
	}

	return err
}
