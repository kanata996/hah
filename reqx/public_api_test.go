package reqx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func assertPublicType[T any](_, _ T) {}

func TestPublicBuilderFamilies(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?x=1", nil)

	assertPublicType(Path(req, "id").String(), (*StringParam)(nil))
	assertPublicType(Query(req, "name").String(), (*StringParam)(nil))

	assertPublicType(Path(req, "id").Int(), (*OrderedParam[int])(nil))
	assertPublicType(Path(req, "id").Int64(), (*OrderedParam[int64])(nil))
	assertPublicType(Path(req, "id").Uint(), (*OrderedParam[uint])(nil))
	assertPublicType(Path(req, "id").Uint64(), (*OrderedParam[uint64])(nil))
	assertPublicType(Path(req, "id").UUID(), (*ValueParam[uuid.UUID])(nil))

	assertPublicType(Query(req, "page").Int(), (*OrderedParam[int])(nil))
	assertPublicType(Query(req, "page").Int64(), (*OrderedParam[int64])(nil))
	assertPublicType(Query(req, "page").Uint(), (*OrderedParam[uint])(nil))
	assertPublicType(Query(req, "page").Uint64(), (*OrderedParam[uint64])(nil))
	assertPublicType(Query(req, "enabled").Bool(), (*ValueParam[bool])(nil))
	assertPublicType(Query(req, "score").Float64(), (*OrderedParam[float64])(nil))
	assertPublicType(Query(req, "wait").Duration(), (*OrderedParam[time.Duration])(nil))
	assertPublicType(Query(req, "token").UUID(), (*ValueParam[uuid.UUID])(nil))
	assertPublicType(Query(req, "when").Time(), (*TimeParam)(nil))
	assertPublicType(Query(req, "when").UnixTime(), (*TimeParam)(nil))
	assertPublicType(Query(req, "when").UnixMilliTime(), (*TimeParam)(nil))
	assertPublicType(Query(req, "tag").Values(), (*MultiParam[string])(nil))
}
