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
)

type Normalizer interface {
	Normalize()
}

type sourceKind string

const (
	sourceBody    sourceKind = "json"
	sourceQuery   sourceKind = "query"
	sourcePath    sourceKind = "param"
	sourceHeader  sourceKind = "header"
	sourceRequest sourceKind = "request"
)

var (
	validatorOnce sync.Once
	validators    map[sourceKind]*validator.Validate
)

func BindAndValidate[T any](r *http.Request, dst *T, opts ...BindOption) error {
	if err := Bind(r, dst, opts...); err != nil {
		return err
	}
	return validate(dst, sourceRequest)
}

func BindAndValidateBody[T any](r *http.Request, dst *T, opts ...BindBodyOption) error {
	if err := BindBody(r, dst, opts...); err != nil {
		return err
	}
	return validate(dst, sourceBody)
}

func BindAndValidateQuery[T any](r *http.Request, dst *T, opts ...BindQueryParamsOption) error {
	if err := BindQueryParams(r, dst, opts...); err != nil {
		return err
	}
	return validate(dst, sourceQuery)
}

func BindAndValidatePath[T any](r *http.Request, dst *T) error {
	if err := BindPathValues(r, dst); err != nil {
		return err
	}
	return validate(dst, sourcePath)
}

func BindAndValidateHeaders[T any](r *http.Request, dst *T, opts ...BindHeadersOption) error {
	if err := BindHeaders(r, dst, opts...); err != nil {
		return err
	}
	return validate(dst, sourceHeader)
}

func validate[T any](target *T, source sourceKind) error {
	if err := validateTarget(target); err != nil {
		return err
	}

	if normalizer, ok := any(target).(Normalizer); ok {
		normalizer.Normalize()
	}

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

func validateStruct(target any, source sourceKind) ([]Violation, error) {
	err := validatorFor(source).Struct(target)
	if err == nil {
		return nil, nil
	}

	var invalidValidationErr *validator.InvalidValidationError
	if errors.As(err, &invalidValidationErr) {
		return nil, err
	}

	// validator/v10's Struct contract returns only nil,
	// InvalidValidationError, or ValidationErrors.
	validationErrs := err.(validator.ValidationErrors)
	return violationsFromValidation(source, target, validationErrs), nil
}

func validatorFor(source sourceKind) *validator.Validate {
	validatorOnce.Do(func() {
		validators = map[sourceKind]*validator.Validate{
			sourceBody:    newValidator(sourceBody),
			sourceQuery:   newValidator(sourceQuery),
			sourcePath:    newValidator(sourcePath),
			sourceHeader:  newValidator(sourceHeader),
			sourceRequest: newValidator(sourceRequest),
		}
	})

	v, ok := validators[source]
	if !ok {
		panic(fmt.Sprintf("reqx: unsupported validation source %q", source))
	}
	return v
}

func newValidator(source sourceKind) *validator.Validate {
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

func violationsFromValidation(source sourceKind, target any, errs validator.ValidationErrors) []Violation {
	if len(errs) == 0 {
		return nil
	}

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
			in:    validationInput(source, target, validationErr),
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

func validationFieldPath(source sourceKind, err validator.FieldError) string {
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
	case sourceBody:
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

func validationInput(source sourceKind, target any, err validator.FieldError) string {
	if source != sourceRequest {
		return violationInForSource(source)
	}

	fields, ok := resolveValidationFieldPath(target, err.StructNamespace())
	if !ok {
		return ViolationInRequest
	}

	for i := len(fields) - 1; i >= 0; i-- {
		for _, tagName := range sourceTagPriority(sourceRequest) {
			if name := tagValue(fields[i], tagName); name != "" {
				return violationInForTag(tagName)
			}
		}
	}
	return ViolationInRequest
}

func violationInForSource(source sourceKind) string {
	if input, ok := violationInputsBySource[source]; ok {
		return input
	}
	return ViolationInRequest
}

func violationInForTag(tagName string) string {
	if input, ok := violationInputsByTag[tagName]; ok {
		return input
	}
	return ViolationInRequest
}

func resolveValidationFieldPath(target any, structNamespace string) ([]reflect.StructField, bool) {
	t := reflect.TypeOf(target)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, false
	}

	path := parseStructNamespace(structNamespace)
	if len(path) == 0 {
		return nil, false
	}

	current := t
	fields := make([]reflect.StructField, 0, len(path))
	for _, name := range path[:len(path)-1] {
		field, ok := current.FieldByName(name)
		if !ok {
			return nil, false
		}
		fields = append(fields, field)

		next := field.Type
		for next.Kind() == reflect.Pointer || next.Kind() == reflect.Slice || next.Kind() == reflect.Array {
			next = next.Elem()
		}
		if next.Kind() != reflect.Struct {
			return nil, false
		}
		current = next
	}

	field, ok := current.FieldByName(path[len(path)-1])
	if !ok {
		return nil, false
	}
	fields = append(fields, field)
	return fields, true
}

func parseStructNamespace(namespace string) []string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil
	}

	parts := strings.Split(namespace, ".")
	if len(parts) <= 1 {
		return nil
	}

	path := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		if bracket := strings.Index(part, "["); bracket >= 0 {
			part = part[:bracket]
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		path = append(path, part)
	}
	return path
}

func fieldAlias(field reflect.StructField, source sourceKind) string {
	for _, tagName := range sourceTagPriority(source) {
		if name := tagValue(field, tagName); name != "" {
			if tagName == "header" {
				return textproto.CanonicalMIMEHeaderKey(name)
			}
			return name
		}
	}
	return field.Name
}

func sourceTagPriority(source sourceKind) []string {
	if priority, ok := sourceTagPriorities[source]; ok {
		return priority
	}
	panic(fmt.Sprintf("reqx: unsupported tag source %q", source))
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

var (
	sourceTagPriorities = map[sourceKind][]string{
		sourceBody:    {"json", "query", "param", "header"},
		sourceQuery:   {"query", "json", "param", "header"},
		sourcePath:    {"param", "json", "query", "header"},
		sourceHeader:  {"header", "json", "query", "param"},
		sourceRequest: {"param", "query", "json", "header"},
	}
	violationInputsBySource = map[sourceKind]string{
		sourceBody:    ViolationInBody,
		sourceQuery:   ViolationInQuery,
		sourcePath:    ViolationInPath,
		sourceHeader:  ViolationInHeader,
		sourceRequest: ViolationInRequest,
	}
	violationInputsByTag = map[string]string{
		"json":   ViolationInBody,
		"query":  ViolationInQuery,
		"param":  ViolationInPath,
		"header": ViolationInHeader,
	}
)

func errorsf(format string, args ...any) error {
	return fmt.Errorf("reqx: "+format, args...)
}
