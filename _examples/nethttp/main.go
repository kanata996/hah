package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

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
			"acct_002": {ID: "acct_002", OrgID: "org_456", Name: "Billing"},
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

func (s *accountStore) list(orgID, name string) []account {
	s.mu.Lock()
	defer s.mu.Unlock()

	filter := strings.ToLower(strings.TrimSpace(name))
	items := make([]account, 0, len(s.accounts))
	for _, acct := range s.accounts {
		if acct.OrgID != orgID {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(acct.Name), filter) {
			continue
		}
		items = append(items, acct)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
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

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	if writeErr := hah.WriteError(w, err); writeErr != nil {
		log.Printf("write error response failed: %v", writeErr)
	}
}

type listAccountsRequest struct {
	Name string `query:"name"`
}

func (r *listAccountsRequest) normalize() {
	r.Name = strings.TrimSpace(r.Name)
}

type createAccountRequest struct {
	Name string `json:"name"`
}

func (r *createAccountRequest) normalize() {
	r.Name = strings.TrimSpace(r.Name)
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

func newServer(store *accountStore) http.Handler {
	application := &app{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", application.healthz)
	mux.HandleFunc("GET /orgs/{org_id}/accounts", application.listAccounts)
	mux.HandleFunc("POST /orgs/{org_id}/accounts", application.createAccount)
	mux.HandleFunc("GET /orgs/{org_id}/accounts/{account_id}", application.getAccount)
	mux.HandleFunc("DELETE /orgs/{org_id}/accounts/{account_id}", application.deleteAccount)

	return mux
}

func (a *app) healthz(w http.ResponseWriter, r *http.Request) {
	if err := hah.OK(w, map[string]string{"status": "ok"}); err != nil {
		writeError(w, r, err)
	}
}

func (a *app) listAccounts(w http.ResponseWriter, r *http.Request) {
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
	req.normalize()

	items := a.store.list(orgID, req.Name)
	if err := hah.OK(w, map[string]any{
		"org_id": orgID,
		"count":  len(items),
		"items":  items,
	}); err != nil {
		writeError(w, r, err)
	}
}

func (a *app) createAccount(w http.ResponseWriter, r *http.Request) {
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

	if err := a.store.delete(orgID, accountID); err != nil {
		writeError(w, r, err)
		return
	}

	if err := hah.NoContent(w); err != nil {
		writeError(w, r, err)
	}
}

func main() {
	log.Fatal(http.ListenAndServe(":8080", newServer(newAccountStore())))
}
