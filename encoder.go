package form

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"
)

type encoder struct {
	e      *Encoder
	errs   EncodeErrors
	values url.Values
}

func (e *encoder) setError(namespace string, err error) {
	if e.errs == nil {
		e.errs = make(EncodeErrors)
	}

	e.errs[namespace] = err
}

func (e *encoder) setVal(namespace string, idx int, vals ...string) {

	arr, ok := e.values[namespace]
	if ok {
		arr = append(arr, vals...)
	} else {
		arr = vals
	}

	e.values[namespace] = arr
}

func (e *encoder) traverseStruct(v reflect.Value, namespace string, idx int) {

	typ := v.Type()
	var nn string // new namespace
	first := len(namespace) == 0

	// is anonymous struct, cannot parse or cache as
	// it has no name to index by and potentially a
	// dynamic value
	if len(typ.Name()) == 0 {

		numFields := v.NumField()
		var fld reflect.StructField
		var key string

		for i := 0; i < numFields; i++ {

			fld = typ.Field(i)

			if fld.PkgPath != blank && !fld.Anonymous {
				continue
			}

			if key = fld.Tag.Get(e.e.tagName); key == ignore {
				continue
			}

			if len(key) == 0 {
				key = fld.Name
			}

			if first {
				nn = key
			} else {
				nn = namespace + namespaceSeparator + key
			}

			e.setFieldByType(v.Field(i), nn, idx)

		}
	} else {
		s, ok := e.e.structCache.Get(typ)
		if !ok {
			s = e.e.parseStruct(v)
		}

		for _, f := range s.fields {

			if first {
				nn = f.name
			} else {
				nn = namespace + namespaceSeparator + f.name
			}

			e.setFieldByType(v.Field(f.idx), nn, idx)
		}
	}

	return
}

func (e *encoder) setFieldByType(current reflect.Value, namespace string, idx int) {

	if idx > -1 && current.Kind() == reflect.Ptr {
		namespace += "[" + strconv.Itoa(idx) + "]"
		idx = -2
	}

	v, kind := ExtractType(current)

	if e.e.customTypeFuncs != nil {

		if cf, ok := e.e.customTypeFuncs[v.Type()]; ok {

			arr, err := cf(v.Interface())
			if err != nil {
				e.setError(namespace, err)
				return
			}

			if idx > -1 {
				namespace += "[" + strconv.Itoa(idx) + "]"
			}

			e.setVal(namespace, idx, arr...)
			return
		}
	}

	switch kind {
	case reflect.Ptr, reflect.Interface, reflect.Invalid:
		return

	case reflect.String:

		e.setVal(namespace, idx, v.String())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:

		e.setVal(namespace, idx, strconv.FormatUint(v.Uint(), 10))

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:

		e.setVal(namespace, idx, strconv.FormatInt(v.Int(), 10))

	case reflect.Float32:

		e.setVal(namespace, idx, strconv.FormatFloat(v.Float(), 'f', -1, 32))

	case reflect.Float64:

		e.setVal(namespace, idx, strconv.FormatFloat(v.Float(), 'f', -1, 64))

	case reflect.Bool:

		e.setVal(namespace, idx, strconv.FormatBool(v.Bool()))

	case reflect.Slice, reflect.Array:

		if idx == -1 {

			for i := 0; i < v.Len(); i++ {
				e.setFieldByType(v.Index(i), namespace, i)
			}

			return
		}

		if idx > -1 {
			namespace += "[" + strconv.Itoa(idx) + "]"
		}

		for i := 0; i < v.Len(); i++ {
			e.setFieldByType(v.Index(i), namespace+"["+strconv.Itoa(i)+"]", -2)
		}

	case reflect.Map:

		if idx > -1 {
			namespace += "[" + strconv.Itoa(idx) + "]"
		}

		var valid bool
		var s string

		for _, key := range v.MapKeys() {
			if s, valid = e.getMapKey(key, namespace); !valid {
				continue
			}

			e.setFieldByType(current.MapIndex(key), namespace+"["+s+"]", -2)
		}

	case reflect.Struct:

		// if we get here then no custom time function declared so use RFC3339 by default
		if v.Type() == timeType {

			if idx > -1 {
				namespace += "[" + strconv.Itoa(idx) + "]"
			}

			e.setVal(namespace, idx, v.Interface().(time.Time).Format(time.RFC3339))
			return
		}

		if idx == -1 {
			e.traverseStruct(v, namespace, idx)
			return
		}

		if idx > -1 {
			namespace += "[" + strconv.Itoa(idx) + "]"
		}

		e.traverseStruct(v, namespace, -2)
	}

	return
}

func (e *encoder) getMapKey(key reflect.Value, namespace string) (string, bool) {

	v, kind := ExtractType(key)

	if e.e.customTypeFuncs != nil {

		if cf, ok := e.e.customTypeFuncs[v.Type()]; ok {
			arr, err := cf(v.Interface())
			if err != nil {
				e.setError(namespace, err)
				return "", false
			}

			return arr[0], true
		}
	}

	switch kind {
	case reflect.Interface, reflect.Ptr:
		return "", false

	case reflect.String:
		return v.String(), true

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), true

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), true

	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32), true

	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), true

	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), true

	default:
		e.setError(namespace, fmt.Errorf("Unsupported Map Key '%v' Namespace '%s'", v.String(), namespace))
		return "", false
	}
}