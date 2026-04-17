package errx

// 这些编译期断言用于锁住 errx 的稳定公开 API 表面。
type httpErrorPublicSurface interface {
	Error() string
	Unwrap() error
	Status() int
	Code() string
	Title() string
	Detail() string
	Errors() []Violation
	WithViolations([]Violation) *HTTPError
}

var _ httpErrorPublicSurface = (*HTTPError)(nil)

var (
	_ = Violation{}

	_ ViolationCode = CodeInvalid
	_ ViolationCode = CodeRequired
	_ ViolationCode = CodeUnknown
	_ ViolationCode = CodeType
	_ ViolationCode = CodeMultiple

	_ ViolationIn = InBody
	_ ViolationIn = InQuery
	_ ViolationIn = InPath
	_ ViolationIn = InHeader

	_ func(int, string, string) *HTTPError        = NewHTTPError
	_ func(int, string, string, error) *HTTPError = NewHTTPErrorWithCause
	_ func(string, string) *HTTPError             = BadRequest
	_ func(string, string) *HTTPError             = Unauthorized
	_ func(string, string) *HTTPError             = Forbidden
	_ func(string, string) *HTTPError             = NotFound
	_ func(string, string) *HTTPError             = MethodNotAllowed
	_ func(string, string) *HTTPError             = Conflict
	_ func(string, string) *HTTPError             = UnprocessableEntity
	_ func(string, string) *HTTPError             = TooManyRequests
)
