package reqx

import (
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/kanata996/hah/bind"
)

// 本文件负责 reqx 包的组合校验主流程、validator 初始化，以及字段级 violation 生成辅助。
//
// 这里承载的能力包括：
//   - 对外公开的组合入口：BindAndValidate、Validate
//   - 绑定后的固定阶段顺序：Normalize -> ValidateRequest -> validator/v10
//   - 各输入来源的 validator 实例、字段别名和 tag 优先级策略
//   - validator.ValidationErrors 到 Violation 的稳定转换
//   - 组合层目标类型与参数的公共前置校验

type Normalizer interface {
	Normalize()
}

// Source 标识校验阶段应采用哪一类输入来源语义。
//
// 它决定：
//   - validator 字段别名优先读取哪些 struct tag
//   - violation 的 `in` 字段使用哪个公开来源值
type Source string

const (
	SourceBody    Source = "json"
	SourceQuery   Source = "query"
	SourcePath    Source = "param"
	SourceHeader  Source = "header"
	SourceRequest Source = "request"
)

var (
	validatorOnce sync.Once
	validators    map[Source]*validator.Validate
)

func BindAndValidate(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if target == nil {
		return errorsf("destination must not be nil")
	}
	if err := validateTarget(target); err != nil {
		return err
	}
	if err := bind.Bind(r, target); err != nil {
		return err
	}
	return Validate(r, target, SourceRequest)
}

// Validate 在绑定完成后执行 Normalize、请求级规则和字段校验。
//
// source 用于确定 tag 别名优先级和 violation 的输入来源语义；它不负责执行绑定。
// 需要显式来源绑定时，可与 bind 包组合使用，例如：
//
//	if err := bind.BindHeaders(r, &dst); err != nil { ... }
//	if err := reqx.Validate(r, &dst, reqx.SourceHeader); err != nil { ... }
func Validate(r *http.Request, target any, source Source) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if err := validateSource(source); err != nil {
		return err
	}
	return postBindValidate(r, target, source)
}

func postBindValidate(r *http.Request, target any, source Source) error {
	if err := validateTarget(target); err != nil {
		return err
	}

	normalizeTarget(target)

	if err := applyRequestValidation(r, target); err != nil {
		return err
	}

	return validateFields(target, source)
}

func validateSource(source Source) error {
	if _, ok := sourceSpecFor(source); ok {
		return nil
	}
	return errorsf("unsupported validation source %q", source)
}

func normalizeTarget(target any) {
	if normalizer, ok := target.(Normalizer); ok {
		normalizer.Normalize()
	}
}

func validateFields(target any, source Source) error {
	violations, err := validateStruct(target, source)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		return nil
	}

	return invalidFieldsError(violations)
}

func validateTarget(target any) error {
	if target == nil {
		return errorsf("target must not be nil")
	}

	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return errorsf("target must be a non-nil pointer to struct")
	}

	return nil
}

func validateStruct(target any, source Source) ([]Violation, error) {
	return validationResultFromError(source, validatorFor(source).Struct(target))
}

func validationResultFromError(source Source, err error) ([]Violation, error) {
	if err == nil {
		return nil, nil
	}

	var invalidValidationErr *validator.InvalidValidationError
	if errors.As(err, &invalidValidationErr) {
		return nil, err
	}

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		return violationsFromValidation(source, validationErrs), nil
	}

	return nil, err
}

func validatorFor(source Source) *validator.Validate {
	validatorOnce.Do(func() {
		validators = make(map[Source]*validator.Validate, len(sourceSpecs))
		for source := range sourceSpecs {
			validators[source] = newValidator(source)
		}
	})

	v, ok := validators[source]
	if !ok {
		panic(fmt.Sprintf("reqx: unsupported validation source %q", source))
	}
	return v
}

func newValidator(source Source) *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		return fieldAlias(field, source)
	})
	mustRegisterValidation(v, "nospace", validateNoSpace)
	return v
}

func mustRegisterValidation(v *validator.Validate, tag string, fn validator.Func) {
	if err := v.RegisterValidation(tag, fn); err != nil {
		panic(fmt.Sprintf("reqx: register validator %q: %v", tag, err))
	}
}

func validateNoSpace(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() != reflect.String {
		return false
	}
	return !strings.ContainsRune(field.String(), ' ')
}

func violationsFromValidation(source Source, errs validator.ValidationErrors) []Violation {
	if len(errs) == 0 {
		return nil
	}
	spec := mustSourceSpec(source)

	seen := make(map[string]struct{}, len(errs))
	type entry struct {
		field string
		in    string
		code  string
	}
	entries := make([]entry, 0, len(errs))

	for _, validationErr := range errs {
		field := validationFieldPath(source, validationErr)
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		entries = append(entries, entry{
			field: field,
			in:    spec.violationIn,
			code:  validationCode(validationErr.Tag()),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].field < entries[j].field
	})

	violations := make([]Violation, 0, len(entries))
	for _, entry := range entries {
		violations = append(violations, newViolation(entry.field, entry.in, entry.code, violationDetailForCode(entry.code)))
	}
	return violations
}

func validationFieldPath(source Source, err validator.FieldError) string {
	namespace := strings.TrimSpace(err.Namespace())
	if namespace != "" {
		if dot := strings.Index(namespace, "."); dot >= 0 {
			namespace = namespace[dot+1:]
		}
		namespace = strings.TrimSpace(namespace)
		if namespace != "" {
			return namespace
		}
	}

	field := strings.TrimSpace(err.Field())
	if field != "" {
		return field
	}

	switch source {
	case SourceBody:
		return "body"
	default:
		return "request"
	}
}

func validationCode(tag string) string {
	switch tag {
	case "required":
		return ViolationCodeRequired
	default:
		return ViolationCodeInvalid
	}
}

func fieldAlias(field reflect.StructField, source Source) string {
	if source == SourceRequest {
		if alias, ok := requestFieldAlias(field); ok {
			return alias
		}
	}

	for _, tagName := range mustSourceSpec(source).tagPriority {
		if name := tagValue(field, tagName); name != "" {
			if tagName == "header" {
				return textproto.CanonicalMIMEHeaderKey(name)
			}
			return name
		}
	}
	return field.Name
}

func requestFieldAlias(field reflect.StructField) (string, bool) {
	var alias string
	for _, tagName := range mustSourceSpec(SourceRequest).tagPriority {
		name := tagValue(field, tagName)
		if name == "" {
			continue
		}
		if tagName == "header" {
			name = textproto.CanonicalMIMEHeaderKey(name)
		}
		if alias == "" {
			alias = name
			continue
		}
		if alias != name {
			// Mixed-source validation needs one stable public field name. When a field
			// declares different names across sources, prefer the struct field name
			// instead of misreporting one source-specific alias.
			return field.Name, true
		}
	}
	if alias == "" {
		return "", false
	}
	return alias, true
}

func tagValue(field reflect.StructField, tagName string) string {
	value := strings.TrimSpace(field.Tag.Get(tagName))
	if value == "" {
		return ""
	}
	value, _, _ = strings.Cut(value, ",")
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return ""
	}
	return value
}

type sourceSpec struct {
	tagPriority []string
	violationIn string
}

func sourceSpecFor(source Source) (sourceSpec, bool) {
	spec, ok := sourceSpecs[source]
	return spec, ok
}

func mustSourceSpec(source Source) sourceSpec {
	spec, ok := sourceSpecFor(source)
	if !ok {
		panic(fmt.Sprintf("reqx: unsupported validation source %q", source))
	}
	return spec
}

var (
	sourceSpecs = map[Source]sourceSpec{
		SourceBody: {
			tagPriority: []string{"json", "query", "param", "header"},
			violationIn: ViolationInBody,
		},
		SourceQuery: {
			tagPriority: []string{"query", "json", "param", "header"},
			violationIn: ViolationInQuery,
		},
		SourcePath: {
			tagPriority: []string{"param", "json", "query", "header"},
			violationIn: ViolationInPath,
		},
		SourceHeader: {
			tagPriority: []string{"header", "json", "query", "param"},
			violationIn: ViolationInHeader,
		},
		SourceRequest: {
			tagPriority: []string{"param", "query", "json", "header"},
			violationIn: ViolationInRequest,
		},
	}
)

func errorsf(format string, args ...any) error {
	return fmt.Errorf("reqx: "+format, args...)
}
