package hah_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/kanata996/hah"
	"github.com/kanata996/hah/errcode"
)

func Example() {
	type listUsersQuery struct {
		Role string `query:"role"`
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query listUsersQuery
		if err := hah.DecodeQuery(r, &query); err != nil {
			_ = hah.RenderError(w, r, err)
			return
		}

		role := strings.TrimSpace(query.Role)
		if role == "" {
			role = "member"
		}

		if err := hah.RenderWithMeta(w, r, []map[string]any{
			{"id": "u_1", "role": role},
		}, map[string]any{"count": 1}); err != nil {
			_ = hah.RenderError(w, r, err)
			return
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/users?role=admin", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	fmt.Println(rr.Code)
	fmt.Println(strings.TrimSpace(rr.Body.String()))
	// Output:
	// 200
	// {"data":[{"id":"u_1","role":"admin"}],"meta":{"count":1}}
}

func ExampleRenderError_withErrorMappers() {
	errUserNotFound := errors.New("user not found")
	const codeUserNotFound = "user_not_found"
	mapUserError := func(err error) *hah.HTTPError {
		if errors.Is(err, errUserNotFound) {
			return hah.NotFound(codeUserNotFound, "user not found")
		}
		return nil
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = hah.RenderError(w, r, errUserNotFound, hah.ErrorMappers(mapUserError))
	})

	req := httptest.NewRequest(http.MethodGet, "/users/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	fmt.Println(rr.Code)
	fmt.Println(strings.TrimSpace(rr.Body.String()))
	// Output:
	// 404
	// {"error":{"code":"user_not_found","message":"user not found","details":[]}}
}

func ExampleWithResponses() {
	errUserNotFound := errors.New("user not found")
	const codeUserNotFound = "user_not_found"
	mapUserError := func(err error) *hah.HTTPError {
		if errors.Is(err, errUserNotFound) {
			return hah.NotFound(codeUserNotFound, "user not found")
		}
		return nil
	}

	handler := hah.WithResponses(hah.ErrorMappers(mapUserError))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = hah.RenderError(w, r, errUserNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	fmt.Println(rr.Code)
	fmt.Println(strings.TrimSpace(rr.Body.String()))
	// Output:
	// 404
	// {"error":{"code":"user_not_found","message":"user not found","details":[]}}
}

func ExampleRenderError() {
	req := httptest.NewRequest(http.MethodGet, "/reports/heavy", nil)
	rr := httptest.NewRecorder()

	_ = hah.RenderError(rr, req, hah.TooManyRequests(
		errcode.RateLimited,
		"rate limit exceeded",
	))

	fmt.Println(rr.Code)
	fmt.Println(strings.TrimSpace(rr.Body.String()))
	// Output:
	// 429
	// {"error":{"code":"rate_limited","message":"rate limit exceeded","details":[]}}
}

func ExampleDecodeAndValidateJSON() {
	type createUserRequest struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")

	var input createUserRequest
	err := hah.DecodeAndValidateJSON(req, &input, func(value *createUserRequest) []hah.Violation {
		if strings.TrimSpace(value.Name) == "" {
			return []hah.Violation{{
				Field:   "name",
				Code:    errcode.ViolationRequired,
				Message: "is required",
			}}
		}
		return nil
	})

	fmt.Println(err == nil)
	fmt.Println(input.Name)
	// Output:
	// true
	// alice
}
