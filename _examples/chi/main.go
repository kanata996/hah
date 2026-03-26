package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kanata996/hah"
	"github.com/kanata996/hah/errcode"
)

var (
	errUserConflict = errors.New("users: conflict")
	errUserNotFound = errors.New("users: not found")
)

const (
	codeUserConflict = "user_conflict"
	codeUserNotFound = "user_not_found"
)

type createUserRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type listUsersQuery struct {
	Page  *int   `query:"page"`
	Limit *int   `query:"limit"`
	Role  string `query:"role"`
}

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type userRepository struct {
	users map[string]user
}

type userService struct {
	repo *userRepository
}

func main() {
	log.Fatal(http.ListenAndServe(":8080", newRouter()))
}

func newRouter() http.Handler {
	service := &userService{repo: newUserRepository()}

	r := chi.NewRouter()
	r.Route("/users", func(r chi.Router) {
		// WithResponses 挂在 feature 边界，负责把内部错误语义映射成公开 HTTP 错误。
		r.Use(hah.WithResponses(hah.ErrorMappers(mapUserError)))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			var query listUsersQuery
			if err := hah.DecodeAndValidateQuery(r, &query, validateListUsersQuery); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			users, meta, err := service.List(query)
			if err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			if err := hah.RenderWithMeta(w, r, users, meta); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}
		})

		r.Get("/{userID}", func(w http.ResponseWriter, r *http.Request) {
			item, err := service.Get(chi.URLParam(r, "userID"))
			if err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			if err := hah.Render(w, r, item); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}
		})

		r.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var req createUserRequest
			if err := hah.DecodeAndValidateJSON(r, &req, validateCreateUserRequest); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			item, err := service.Create(req)
			if err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			hah.Status(r, http.StatusCreated)
			if err := hah.Render(w, r, item); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}
		})
	})

	return r
}

func mapUserError(err error) *hah.HTTPError {
	switch {
	case errors.Is(err, errUserNotFound):
		return hah.NotFound(codeUserNotFound, "user not found")
	case errors.Is(err, errUserConflict):
		return hah.Conflict(codeUserConflict, "user already exists")
	default:
		return nil
	}
}

func newUserRepository() *userRepository {
	return &userRepository{
		users: map[string]user{
			"u_1": {ID: "u_1", Name: "Alice", Role: "admin"},
			"u_2": {ID: "u_2", Name: "Bob", Role: "member"},
			"u_3": {ID: "u_3", Name: "Carol", Role: "admin"},
		},
	}
}

func (s *userService) List(query listUsersQuery) ([]user, map[string]any, error) {
	page := 1
	if query.Page != nil {
		page = *query.Page
	}

	limit := 20
	if query.Limit != nil {
		limit = *query.Limit
	}

	role := strings.TrimSpace(query.Role)
	items := s.repo.List(role, page, limit)

	meta := map[string]any{
		"page":  page,
		"limit": limit,
		"count": len(items),
	}
	if role != "" {
		meta["role"] = role
	}

	return items, meta, nil
}

func (s *userService) Get(userID string) (user, error) {
	return s.repo.Get(userID)
}

func (s *userService) Create(req createUserRequest) (user, error) {
	name := strings.TrimSpace(req.Name)
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "member"
	}

	return s.repo.Create(name, role)
}

func (r *userRepository) List(role string, page, limit int) []user {
	ids := make([]string, 0, len(r.users))
	for id := range r.users {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	filtered := make([]user, 0, len(ids))
	for _, id := range ids {
		item := r.users[id]
		if role == "" || item.Role == role {
			filtered = append(filtered, item)
		}
	}

	start := (page - 1) * limit
	if start >= len(filtered) {
		return []user{}
	}

	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	items := make([]user, end-start)
	copy(items, filtered[start:end])
	return items
}

func (r *userRepository) Get(userID string) (user, error) {
	item, ok := r.users[userID]
	if !ok {
		return user{}, errUserNotFound
	}
	return item, nil
}

func (r *userRepository) Create(name, role string) (user, error) {
	for _, item := range r.users {
		if strings.EqualFold(item.Name, name) {
			return user{}, errUserConflict
		}
	}

	item := user{
		ID:   fmt.Sprintf("u_%d", len(r.users)+1),
		Name: name,
		Role: role,
	}
	r.users[item.ID] = item
	return item, nil
}

func validateCreateUserRequest(value *createUserRequest) []hah.Violation {
	var violations []hah.Violation

	if strings.TrimSpace(value.Name) == "" {
		violations = append(violations, hah.Violation{
			Field:   "name",
			Code:    errcode.ViolationRequired,
			Message: "is required",
		})
	}

	role := strings.TrimSpace(value.Role)
	if role != "" && role != "member" && role != "admin" {
		violations = append(violations, hah.Violation{
			Field:   "role",
			Code:    errcode.ViolationOneOf,
			Message: "must be member or admin",
		})
	}

	return violations
}

func validateListUsersQuery(value *listUsersQuery) []hah.Violation {
	var violations []hah.Violation

	if value.Page != nil && *value.Page < 1 {
		violations = append(violations, hah.Violation{
			Field:   "page",
			Code:    errcode.ViolationMin,
			Message: "must be at least 1",
		})
	}

	if value.Limit != nil && (*value.Limit < 1 || *value.Limit > 100) {
		violations = append(violations, hah.Violation{
			Field:   "limit",
			Code:    errcode.ViolationRange,
			Message: "must be between 1 and 100",
		})
	}

	role := strings.TrimSpace(value.Role)
	if role != "" && role != "member" && role != "admin" {
		violations = append(violations, hah.Violation{
			Field:   "role",
			Code:    errcode.ViolationOneOf,
			Message: "must be member or admin",
		})
	}

	return violations
}
