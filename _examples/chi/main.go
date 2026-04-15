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
	"unicode/utf8"

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
	Limit int `query:"limit"`
}

func (r *listAccountsRequest) normalize() {
	if r.Limit == 0 {
		r.Limit = 20
	}
}

type createAccountRequest struct {
	Name string `json:"name"`
}

func (r *createAccountRequest) normalize() {
	r.Name = strings.TrimSpace(r.Name)
}

func validateListAccountsRequest(req *listAccountsRequest) error {
	req.normalize()
	if req.Limit < 1 || req.Limit > 100 {
		return hah.InvalidRequest(hah.Violation{
			Field:  "limit",
			In:     hah.ViolationInQuery,
			Detail: "must be between 1 and 100",
		})
	}
	return nil
}

func validateCreateAccountRequest(r *http.Request, req *createAccountRequest) error {
	if err := hah.RequireBody(r); err != nil {
		return err
	}

	req.normalize()
	switch nameLen := utf8.RuneCountInString(req.Name); {
	case req.Name == "":
		return hah.InvalidRequest(hah.Violation{
			Field: "name",
			In:    hah.ViolationInBody,
			Code:  hah.ViolationCodeRequired,
		})
	case nameLen < 3:
		return hah.InvalidRequest(hah.Violation{
			Field:  "name",
			In:     hah.ViolationInBody,
			Detail: "must be at least 3 characters",
		})
	case nameLen > 64:
		return hah.InvalidRequest(hah.Violation{
			Field:  "name",
			In:     hah.ViolationInBody,
			Detail: "must be at most 64 characters",
		})
	default:
		return nil
	}
}

func validateDeleteActor(actor string) error {
	if strings.TrimSpace(actor) == "" {
		return hah.InvalidRequest(hah.Violation{
			Field: "X-Actor",
			In:    hah.ViolationInHeader,
			Code:  hah.ViolationCodeRequired,
		})
	}
	return nil
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

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	if writeErr := hah.WriteError(w, err); writeErr != nil {
		slog.ErrorContext(r.Context(), "write error response failed", "err", writeErr)
	}
}

func (a *app) listAccounts(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	orgID, err := hah.Path(r, "org_id").String().Required().Get()
	if err != nil {
		writeError(w, r, err)
		return
	}

	var req listAccountsRequest
	if err := hah.BindQuery(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateListAccountsRequest(&req); err != nil {
		writeError(w, r, err)
		return
	}

	payload := map[string]any{
		"org_id": orgID,
		"count":  len(a.store.list(orgID, 0)),
		"items":  a.store.list(orgID, req.Limit),
	}
	if requestID := middleware.GetReqID(r.Context()); requestID != "" {
		payload["request_id"] = requestID
	}
	if traceID := traceid.FromContext(r.Context()); traceID != "" {
		payload["trace_id"] = traceID
	}

	if err := hah.OK(w, payload); err != nil {
		writeError(w, r, err)
	}
}

func (a *app) createAccount(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	orgID, err := hah.Path(r, "org_id").String().Required().Get()
	if err != nil {
		writeError(w, r, err)
		return
	}

	var req createAccountRequest
	if err := hah.BindBody(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateCreateAccountRequest(r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	acct, err := a.store.create(orgID, req.Name)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := hah.Created(w, acct); err != nil {
		writeError(w, r, err)
	}
}

func (a *app) getAccount(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	orgID, err := hah.Path(r, "org_id").String().Required().Get()
	if err != nil {
		writeError(w, r, err)
		return
	}

	accountID, err := hah.Path(r, "account_id").String().Required().Get()
	if err != nil {
		writeError(w, r, err)
		return
	}

	acct, err := a.store.get(orgID, accountID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := hah.OK(w, acct); err != nil {
		writeError(w, r, err)
	}
}

func (a *app) deleteAccount(w http.ResponseWriter, r *http.Request) {
	bridgeChiPathValues(r)

	orgID, err := hah.Path(r, "org_id").String().Required().Get()
	if err != nil {
		writeError(w, r, err)
		return
	}

	accountID, err := hah.Path(r, "account_id").String().Required().Get()
	if err != nil {
		writeError(w, r, err)
		return
	}

	actor := strings.TrimSpace(r.Header.Get("X-Actor"))
	if err := validateDeleteActor(actor); err != nil {
		writeError(w, r, err)
		return
	}

	if err := a.store.delete(orgID, accountID); err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("X-Deleted-By", actor)
	if err := hah.NoContent(w); err != nil {
		writeError(w, r, err)
	}
}

func main() {
	accessLogger := newExampleLogger(os.Stdout)
	slog.SetDefault(accessLogger)
	log.Fatal(http.ListenAndServe(":8080", newRouterWithLogger(newAccountStore(), accessLogger)))
}
