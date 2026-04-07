package reqx

import "reflect"

func cloneBindingTarget[T any](target *T) *T {
	cloned := new(T)
	*cloned = *target
	deepCloneValue(reflect.ValueOf(cloned).Elem(), reflect.ValueOf(target).Elem())
	return cloned
}

func deepCloneValue(dst, src reflect.Value) {
	if !src.IsValid() {
		return
	}

	dst.Set(src)

	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			return
		}

		cloned := reflect.New(src.Type().Elem())
		deepCloneValue(cloned.Elem(), src.Elem())
		dst.Set(cloned)
	case reflect.Interface:
		if src.IsNil() {
			return
		}

		cloned := reflect.New(src.Elem().Type()).Elem()
		deepCloneValue(cloned, src.Elem())
		dst.Set(cloned)
	case reflect.Struct:
		for i := range src.NumField() {
			fieldDst := dst.Field(i)
			if !fieldDst.CanSet() {
				continue
			}
			deepCloneValue(fieldDst, src.Field(i))
		}
	case reflect.Slice:
		if src.IsNil() {
			return
		}

		cloned := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := range src.Len() {
			deepCloneValue(cloned.Index(i), src.Index(i))
		}
		dst.Set(cloned)
	case reflect.Array:
		for i := range src.Len() {
			deepCloneValue(dst.Index(i), src.Index(i))
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}

		cloned := reflect.MakeMapWithSize(src.Type(), src.Len())
		iter := src.MapRange()
		for iter.Next() {
			key := iter.Key()
			clonedKey := reflect.New(key.Type()).Elem()
			deepCloneValue(clonedKey, key)

			value := iter.Value()
			clonedValue := reflect.New(value.Type()).Elem()
			deepCloneValue(clonedValue, value)

			cloned.SetMapIndex(clonedKey, clonedValue)
		}
		dst.Set(cloned)
	}
}
