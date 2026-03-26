package reqx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

func BenchmarkDecodeQueryWarm(b *testing.B) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/users?id=u_1&page=2&active=true&tag=a&tag=b&limit=50&created_at=2024-01-02T03:04:05Z&mode=full",
		nil,
	)

	var warm listUsersQuery
	if err := DecodeQuery(req, &warm); err != nil {
		b.Fatalf("warm DecodeQuery() error = %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(req.URL.RawQuery)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var got listUsersQuery
		if err := DecodeQuery(req, &got); err != nil {
			b.Fatalf("DecodeQuery() error = %v", err)
		}
	}
}

func BenchmarkDecodeQueryColdPlan(b *testing.B) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/users?id=u_1&page=2&active=true&tag=a&tag=b&limit=50&created_at=2024-01-02T03:04:05Z&mode=full",
		nil,
	)

	b.ReportAllocs()
	b.SetBytes(int64(len(req.URL.RawQuery)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		queryDecodePlanCache = sync.Map{}

		var got listUsersQuery
		if err := DecodeQuery(req, &got); err != nil {
			b.Fatalf("DecodeQuery() error = %v", err)
		}
	}
}

func BenchmarkBuildQueryDecodePlan(b *testing.B) {
	typ := reflect.TypeOf(listUsersQuery{})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		plan, err := buildQueryDecodePlan(typ)
		if err != nil {
			b.Fatalf("buildQueryDecodePlan() error = %v", err)
		}
		if plan == nil {
			b.Fatal("buildQueryDecodePlan() = nil")
		}
	}
}

func BenchmarkLoadQueryDecodePlanHit(b *testing.B) {
	typ := reflect.TypeOf(listUsersQuery{})
	queryDecodePlanCache = sync.Map{}

	plan, err := loadQueryDecodePlan(typ)
	if err != nil {
		b.Fatalf("warm loadQueryDecodePlan() error = %v", err)
	}
	if plan == nil {
		b.Fatal("warm loadQueryDecodePlan() = nil")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		plan, err := loadQueryDecodePlan(typ)
		if err != nil {
			b.Fatalf("loadQueryDecodePlan() error = %v", err)
		}
		if plan == nil {
			b.Fatal("loadQueryDecodePlan() = nil")
		}
	}
}
