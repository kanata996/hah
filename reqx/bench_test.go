package reqx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var (
	benchmarkIntSink    int
	benchmarkBoolSink   bool
	benchmarkStringSink string
	benchmarkTimeSink   time.Time
)

type benchmarkListQuery struct {
	Page    int       `query:"page"`
	Limit   int       `query:"limit"`
	Enabled bool      `query:"enabled"`
	Cursor  string    `query:"cursor"`
	At      time.Time `query:"at"`
}

func BenchmarkQueryIntGet(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=7", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, err := Query(req, "page").Int().Get()
		if err != nil {
			b.Fatalf("Query().Int().Get() error = %v", err)
		}
		benchmarkIntSink = value
	}
}

func BenchmarkQueryMultipleKeys(b *testing.B) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/items?page=2&limit=20&enabled=true&cursor=next&at=2026-04-22T08:30:00Z",
		nil,
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := Query(req, "page").Int().Get()
		if err != nil {
			b.Fatalf("Query(page) error = %v", err)
		}
		limit, err := Query(req, "limit").Int().Get()
		if err != nil {
			b.Fatalf("Query(limit) error = %v", err)
		}
		enabled, err := Query(req, "enabled").Bool().Get()
		if err != nil {
			b.Fatalf("Query(enabled) error = %v", err)
		}
		cursor, err := Query(req, "cursor").String().Get()
		if err != nil {
			b.Fatalf("Query(cursor) error = %v", err)
		}
		at, err := Query(req, "at").Time().Get()
		if err != nil {
			b.Fatalf("Query(at) error = %v", err)
		}

		benchmarkIntSink = page + limit
		benchmarkBoolSink = enabled
		benchmarkStringSink = cursor
		benchmarkTimeSink = at
	}
}

func BenchmarkBindQueryMultipleKeys(b *testing.B) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/items?page=2&limit=20&enabled=true&cursor=next&at=2026-04-22T08:30:00Z",
		nil,
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var query benchmarkListQuery
		if err := BindQuery(req, &query); err != nil {
			b.Fatalf("BindQuery() error = %v", err)
		}

		benchmarkIntSink = query.Page + query.Limit
		benchmarkBoolSink = query.Enabled
		benchmarkStringSink = query.Cursor
		benchmarkTimeSink = query.At
	}
}
