package vm

//go:generate go run ./generate

import (
	"fmt"
	"math"
	"reflect"
)

type Call struct {
	Name string
	Size int
}

type Scope map[string]interface{}

type Fetcher interface {
	Fetch(interface{}) interface{}
}

func fetch(from, i interface{}, nilsafe bool) interface{} {
	if fetcher, ok := from.(Fetcher); ok {
		value := fetcher.Fetch(i)
		if value != nil {
			return value
		}
		if !nilsafe {
			panic(fmt.Sprintf("cannot fetch %v from %T", i, from))
		}
		return nil
	}

	v := reflect.ValueOf(from)
	kind := v.Kind()

	// Structures, maps, and slices can be accessed through a pointer or through
	// a value, when they are accessed through a pointer we don't want to copy
	// them to a value.
	follow := map[reflect.Kind]bool{
		reflect.Map:    true,
		reflect.Slice:  true,
		reflect.Struct: true,
	}

	if kind == reflect.Ptr && follow[reflect.Indirect(v).Kind()] {
		v = reflect.Indirect(v)
		kind = v.Kind()
	}

	switch kind {
	case reflect.Array, reflect.Slice, reflect.String:
		return normalize(v.Index(toInt(i)))

	case reflect.Map:
		value := v.MapIndex(reflect.ValueOf(i))

		if value.IsValid() {
			return normalize(value)
		}

		elem := reflect.TypeOf(from).Elem()

		return reflect.Zero(elem).Interface()

	case reflect.Struct:
		return normalize(v.FieldByName(reflect.ValueOf(i).String()))
	}

	if !nilsafe {
		panic(fmt.Sprintf("cannot fetch %v from %T", i, from))
	}

	return nil
}

// normalize dereferences pointers to basic types, returning the
// underlying value.
func normalize(v reflect.Value) interface{} {
	deref := map[reflect.Kind]bool{
		reflect.Bool:       true,
		reflect.Complex128: true,
		reflect.Complex64:  true,
		reflect.Float32:    true,
		reflect.Float64:    true,
		reflect.Int16:      true,
		reflect.Int32:      true,
		reflect.Int64:      true,
		reflect.Int8:       true,
		reflect.Int:        true,
		reflect.Map:        true,
		reflect.Slice:      true,
		reflect.String:     true,
		reflect.Uint16:     true,
		reflect.Uint32:     true,
		reflect.Uint64:     true,
		reflect.Uint8:      true,
		reflect.Uint:       true,
	}

	// Unwrap an interface to a pointer first, if necessary.
	if v.Kind() == reflect.Interface && v.Elem().Kind() == reflect.Pointer {
		v = v.Elem()
	}

	if v.Kind() == reflect.Pointer && deref[v.Elem().Kind()] {
		v = v.Elem()
	}

	return v.Interface()
}

func slice(array, from, to interface{}) interface{} {
	v := reflect.ValueOf(array)

	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.String:
		length := v.Len()
		a, b := toInt(from), toInt(to)

		if b > length {
			b = length
		}
		if a > b {
			a = b
		}

		value := v.Slice(a, b)
		if value.IsValid() && value.CanInterface() {
			return value.Interface()
		}

	case reflect.Ptr:
		value := v.Elem()
		if value.IsValid() && value.CanInterface() {
			return slice(value.Interface(), from, to)
		}

	}
	panic(fmt.Sprintf("cannot slice %v", from))
}

func FetchFn(from interface{}, name string) reflect.Value {
	v := reflect.ValueOf(from)

	// Methods can be defined on any type.
	if v.NumMethod() > 0 {
		method := v.MethodByName(name)
		if method.IsValid() {
			return method
		}
	}

	d := v
	if v.Kind() == reflect.Ptr {
		d = v.Elem()
	}

	switch d.Kind() {
	case reflect.Map:
		value := d.MapIndex(reflect.ValueOf(name))
		if value.IsValid() && value.CanInterface() {
			return value.Elem()
		}
	case reflect.Struct:
		// If struct has not method, maybe it has func field.
		// To access this field we need dereference value.
		value := d.FieldByName(name)
		if value.IsValid() {
			return value
		}
	}
	panic(fmt.Sprintf(`cannot get "%v" from %T`, name, from))
}

func FetchFnNil(from interface{}, name string) reflect.Value {
	if v := reflect.ValueOf(from); !v.IsValid() {
		return v
	}
	return FetchFn(from, name)
}

func in(needle interface{}, array interface{}) bool {
	if array == nil {
		return false
	}
	v := reflect.ValueOf(array)

	switch v.Kind() {

	case reflect.Array, reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			value := v.Index(i)
			if value.IsValid() && value.CanInterface() {
				if equal(value.Interface(), needle).(bool) {
					return true
				}
			}
		}
		return false

	case reflect.Map:
		n := reflect.ValueOf(needle)
		if !n.IsValid() {
			panic(fmt.Sprintf("cannot use %T as index to %T", needle, array))
		}
		value := v.MapIndex(n)
		if value.IsValid() {
			return true
		}
		return false

	case reflect.Struct:
		n := reflect.ValueOf(needle)
		if !n.IsValid() || n.Kind() != reflect.String {
			panic(fmt.Sprintf("cannot use %T as field name of %T", needle, array))
		}
		value := v.FieldByName(n.String())
		if value.IsValid() {
			return true
		}
		return false

	case reflect.Ptr:
		value := v.Elem()
		if value.IsValid() && value.CanInterface() {
			return in(needle, value.Interface())
		}
		return false
	}

	panic(fmt.Sprintf(`operator "in"" not defined on %T`, array))
}

func length(a interface{}) int {
	v := reflect.ValueOf(a)
	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
		return v.Len()
	default:
		panic(fmt.Sprintf("invalid argument for len (type %T)", a))
	}
}

func negate(i interface{}) interface{} {
	switch v := i.(type) {
	case float32:
		return -v
	case float64:
		return -v

	case int:
		return -v
	case int8:
		return -v
	case int16:
		return -v
	case int32:
		return -v
	case int64:
		return -v

	case uint:
		return -v
	case uint8:
		return -v
	case uint16:
		return -v
	case uint32:
		return -v
	case uint64:
		return -v

	default:
		panic(fmt.Sprintf("invalid operation: - %T", v))
	}
}

func exponent(a, b interface{}) float64 {
	return math.Pow(toFloat64(a), toFloat64(b))
}

func makeRange(min, max int) []int {
	size := max - min + 1
	if size <= 0 {
		return []int{}
	}
	rng := make([]int, size)
	for i := range rng {
		rng[i] = min + i
	}
	return rng
}

func toInt(a interface{}) int {
	switch x := a.(type) {
	case float32:
		return int(x)
	case float64:
		return int(x)

	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)

	case uint:
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		return int(x)

	default:
		panic(fmt.Sprintf("invalid operation: int(%T)", x))
	}
}

func toInt64(a interface{}) int64 {
	switch x := a.(type) {
	case float32:
		return int64(x)
	case float64:
		return int64(x)

	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x

	case uint:
		return int64(x)
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)

	default:
		panic(fmt.Sprintf("invalid operation: int64(%T)", x))
	}
}

func toFloat64(a interface{}) float64 {
	switch x := a.(type) {
	case float32:
		return float64(x)
	case float64:
		return x

	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)

	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)

	default:
		panic(fmt.Sprintf("invalid operation: float64(%T)", x))
	}
}

func isNil(v interface{}) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Slice:
		return r.IsNil()
	default:
		return false
	}
}
