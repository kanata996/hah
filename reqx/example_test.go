package reqx_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/kanata996/hah/reqx"
)

func ExampleDecodeQuery() {
	type listUsersQuery struct {
		Page int    `query:"page"`
		Role string `query:"role"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=2&role=admin", nil)

	var query listUsersQuery
	err := reqx.DecodeQuery(req, &query)

	fmt.Println(err == nil)
	fmt.Println(query.Page)
	fmt.Println(query.Role)
	// Output:
	// true
	// 2
	// admin
}
