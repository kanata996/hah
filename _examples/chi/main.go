package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httplog "github.com/go-chi/httplog/v3"
	"github.com/go-chi/traceid"
	"github.com/kanata996/hah"
	"github.com/kanata996/hah/errx"
)

type account struct {
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
	Name  string `json:"name"`
}

type accountStore struct {
	mu        sync.Mutex
	nextID    int
	accounts  map[string]account
	nameIndex map[string]string
}

func newAccountStore() *accountStore {
	store := &accountStore{
		nextID: 3,
		accounts: map[string]account{
			"acct_001": {ID: "acct_001", OrgID: "org_123", Name: "Primary"},
			"acct_002": {ID: "acct_002", OrgID: "org_123", Name: "Billing"},
		},
		nameIndex: map[string]string{},
	}
	for _, acct := range store.accounts {
		store.nameIndex[accountNameKey(acct.OrgID, acct.Name)] = acct.ID
	}
	return store
}

func accountNameKey(orgID, name string) string {
	return orgID + ":" + strings.ToLower(strings.TrimSpace(name))
}

func (s *accountStore) list(orgID string, limit int) []account {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]account, 0, len(s.accounts))
	for _, acct := range s.accounts {
		if acct.OrgID == orgID {
			items = append(items, acct)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (s *accountStore) create(orgID, name string) (account, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return account{}, errx.UnprocessableEntity("account_name_required", "account name must not be blank")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nameKey := accountNameKey(orgID, trimmed)
	if _, exists := s.nameIndex[nameKey]; exists {
		return account{}, errx.Conflict("account_name_conflict", "account name already exists")
	}

	acct := account{
		ID:    fmt.Sprintf("acct_%03d", s.nextID),
		OrgID: orgID,
		Name:  trimmed,
	}
	s.nextID++
	s.accounts[acct.ID] = acct
	s.nameIndex[nameKey] = acct.ID
	return acct, nil
}

func (s *accountStore) get(orgID, accountID string) (account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	acct, ok := s.accounts[accountID]
	if !ok || acct.OrgID != orgID {
		return account{}, errx.NotFound("account_not_found", "account not found")
	}
	return acct, nil
}

func (s *accountStore) delete(orgID, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acct, ok := s.accounts[accountID]
	if !ok || acct.OrgID != orgID {
		return errx.NotFound("account_not_found", "account not found")
	}

	delete(s.accounts, accountID)
	delete(s.nameIndex, accountNameKey(acct.OrgID, acct.Name))
	return nil
}

type app struct {
	store *accountStore
}

type listAccountsRequest struct {
	OrgID string `param:"org_id" validate:"required"`
	Limit int    `query:"limit" validate:"min=1,max=100"`
}

func (r *listAccountsRequest) Normalize() {
	if r.Limit == 0 {
		r.Limit = 20
	}
}

type createAccountRequest struct {
	OrgID string `param:"org_id" validate:"required"`
	Name  string `json:"name" validate:"required,min=3,max=64"`
}

func (r *createAccountRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
}

type accountPathRequest struct {
	OrgID     string `param:"org_id" validate:"required"`
	AccountID string `param:"account_id" validate:"required"`
}

type deleteAccountHeaders struct {
	Actor string `header:"x-actor" validate:"required"`
}

func (r *deleteAccountHeaders) Normalize() {
	r.Actor = strings.TrimSpace(r.Actor)
}

func newRouter(store *accountStore) http.Handler {
	return newRouterWithLogger(store, newExampleLogger(io.Discard))
}

func newRouterWithLogger(store *accountStore, accessLogger *slog.Logger) http.Handler {
	application := &app{store: store}
	if accessLogger == nil {
		accessLogger = newExampleLogger(io.Discard)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(traceid.Middleware)
	router.Use(httplog.RequestLogger(accessLogger, &httplog.Options{
		Level:             slog.LevelInfo,
		Schema:            httplog.SchemaECS,
		RecoverPanics:     true,
		Skip:              func(req *http.Request, _ int) bool { return req.URL != nil && req.URL.Path == "/healthz" },
		LogRequestHeaders: []string{"Content-Type", middleware.RequestIDHeader, traceid.Header},
		LogResponseHeaders: []string{
			"Content-Type",
			middleware.RequestIDHeader,
			traceid.Header,
		},
	}))
	router.Use(attachRequestObservability)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Use(middleware.Heartbeat("/healthz"))

	router.Route("/orgs/{org_id}/accounts", func(r chi.Router) {
		r.Get("/", application.listAccounts)
		r.Post("/", application.createAccount)
		r.Get("/{account_id}", application.getAccount)
		r.Delete("/{account_id}", application.deleteAccount)
	})

	return router
}

func newExampleLogger(out io.Writer) *slog.Logger {
	if out == nil {
		out = io.Discard
	}
	traceid.LogKey = "trace.id"
	return slog.New(traceid.LogHandler(slog.NewJSONHandler(out, &slog.HandlerOptions{
		ReplaceAttr: httplog.SchemaECS.ReplaceAttr,
	}))).With(
		slog.String("service.name", "hah-chi-example"),
	)
}

func attachRequestObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if requestID := middleware.GetReqID(ctx); requestID != "" {
			httplog.SetAttrs(ctx, slog.String("request.id", requestID))
			w.Header().Set(middleware.RequestIDHeader, requestID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bridgeChiPathValues(r *http.Request) {
	if r == nil {
		return
	}

	routeCtx := chi.RouteContext(r.Context())
	if routeCtx == nil {
		return
	}

	if pattern := strings.TrimSpace(routeCtx.RoutePattern()); pattern != "" {
		r.Pattern = pattern
	}
	for i, key := range routeCtx.URLParams.Keys {
		if strings.TrimSpace(key) == "" || i >= len(routeCtx.URLParams.Values) {
			continue
		}
		r.SetPathValue(key, routeCtx.URLParams.Values[i])
	}
}

func (a *app) listAccounts(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	var req listAccountsRequest
	if err := hah.BindAndValidate(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	payload := map[string]any{
		"org_id": req.OrgID,
		"count":  len(a.store.list(req.OrgID, 0)),
		"items":  a.store.list(req.OrgID, req.Limit),
	}
	if requestID := middleware.GetReqID(r.Context()); requestID != "" {
		payload["request_id"] = requestID
	}
	if traceID := traceid.FromContext(r.Context()); traceID != "" {
		payload["trace_id"] = traceID
	}

	if err := hah.OK(w, r, payload); err != nil {
		_ = hah.WriteError(w, r, err)
	}
}

func (a *app) createAccount(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	var req createAccountRequest
	if err := hah.BindAndValidate(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	acct, err := a.store.create(req.OrgID, req.Name)
	if err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	if err := hah.Created(w, r, acct); err != nil {
		_ = hah.WriteError(w, r, err)
	}
}

func (a *app) getAccount(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	var req accountPathRequest
	if err := hah.BindAndValidatePath(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	acct, err := a.store.get(req.OrgID, req.AccountID)
	if err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	if err := hah.OK(w, r, acct); err != nil {
		_ = hah.WriteError(w, r, err)
	}
}

func (a *app) deleteAccount(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	var pathReq accountPathRequest
	if err := hah.BindAndValidatePath(r, &pathReq); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	var headers deleteAccountHeaders
	if err := hah.BindAndValidateHeaders(r, &headers); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	if err := a.store.delete(pathReq.OrgID, pathReq.AccountID); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	w.Header().Set("X-Deleted-By", headers.Actor)
	if err := hah.NoContent(w, r); err != nil {
		_ = hah.WriteError(w, r, err)
	}
}

func main() {
	accessLogger := newExampleLogger(os.Stdout)
	slog.SetDefault(accessLogger)
	log.Fatal(http.ListenAndServe(":8080", newRouterWithLogger(newAccountStore(), accessLogger)))
}
