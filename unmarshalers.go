package jsonr

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"time"
)

var (
	unmarshalerCache  sync.Map // map[reflect.Type]unmarshaler
	structFieldsCache sync.Map // map[reflect.Type]*structFields
)

type unmarshaler func(*jsontext.Decoder, reflect.Value) error

//nolint:cyclop // jsonr: reflect.Kind dispatch builds the appropriate unmarshaler for each JSON-backed type.
func getUnmarshaler(t reflect.Type) (unmarshaler, error) {
	if v, ok := unmarshalerCache.Load(t); ok {
		u, ok := v.(unmarshaler)
		if !ok {
			return nil, fmt.Errorf("jsonr: internal: bad unmarshaler cache entry for %v", t)
		}
		return u, nil
	}
	var (
		fnc unmarshaler
		err error
	)

	//nolint:exhaustive // jsonr: only types that can be produced from JSON are supported.
	switch t.Kind() {
	case reflect.Bool:
		fnc = makeBoolUnmarshaler(t)
	case reflect.String:
		fnc = makeStringUnmarshaler(t)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fnc = makeIntUnmarshaler(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		fnc = makeUintUnmarshaler(t)
	case reflect.Float32, reflect.Float64:
		fnc = makeFloatUnmarshaler(t)
	case reflect.Map:
		fnc = makeMapUnmarshaler(t)
	case reflect.Pointer:
		var inner unmarshaler
		inner, err = getUnmarshaler(t.Elem())
		if err != nil {
			break
		}
		fnc = makePointerUnmarshaler(inner)
	case reflect.Slice:
		fnc, err = makeSliceUnmarshaler(t)
	case reflect.Array:
		fnc, err = makeArrayUnmarshaler(t)
	case reflect.Struct:
		if t == reflect.TypeFor[time.Time]() {
			fnc = makeTimeUnmarshaler(t)
		} else {
			fnc, err = makeStructValueUnmarshaler(t)
		}
	default:
		err = fmt.Errorf("jsonr: unsupported type: %v", t)
	}
	if fnc != nil {
		unmarshalerCache.Store(t, fnc)
	}
	return fnc, err
}

// makeStructValueUnmarshaler decodes one JSON object into a Go struct using that type's jsonr fields.
func makeStructValueUnmarshaler(t reflect.Type) (unmarshaler, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("jsonr: internal: expected struct, got %v", t)
	}
	sf, err := getCompiledStructFields(t)
	if err != nil {
		return nil, err
	}
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: only null and object are valid for a struct value.
		switch tok.Kind() {
		case 'n':
			vptr.SetZero()
			return nil
		case '{':
		default:
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected object or null, got %v", tok.Kind()),
			}
		}
		if err := decodeObject(dec, sf.structGraph.root, vptr, sf); err != nil {
			return err
		}
		closeTok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read closing token: %w", err)
		}
		if closeTok.Kind() != '}' {
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    closeTok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected object end '}', got %v", closeTok.Kind()),
			}
		}
		return nil
	}, nil
}

func makeBoolUnmarshaler(t reflect.Type) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}

		//nolint:exhaustive // jsonr: bool decoder handles null, true, false, and string forms.
		switch tok.Kind() {
		case 'n':
			vptr.SetBool(false)
		case 't':
			vptr.SetBool(true)
		case '"':
			var val bool
			if val, err = strconv.ParseBool(tok.String()); err == nil {
				vptr.SetBool(val)
			} else {
				err = &json.SemanticError{
					ByteOffset:  dec.InputOffset(),
					JSONPointer: dec.StackPointer(),
					JSONKind:    tok.Kind(),
					GoType:      t,
					Err:         fmt.Errorf("jsonr: failed to parse boolean from string: %q; error: %w", tok.String(), err),
				}
			}
		default:
			err = &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected boolean, got %v", tok.Kind()),
			}
		}
		return err
	}
}

func makeStringUnmarshaler(t reflect.Type) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		if tok.Kind() != '"' {
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected string, got %v", tok.Kind()),
			}
		}
		vptr.SetString(tok.String())
		return nil
	}
}

func makeTimeUnmarshaler(t reflect.Type) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: time decoder handles null, string, and integer unix-seconds forms.
		switch tok.Kind() {
		case 'n':
			vptr.Set(reflect.Zero(vptr.Type()))
			return nil
		case '"':
			parsed, derr := decodeTime(tok.String())
			if derr != nil {
				return &json.SemanticError{
					ByteOffset:  dec.InputOffset(),
					JSONPointer: dec.StackPointer(),
					JSONKind:    tok.Kind(),
					GoType:      t,
					Err:         derr,
				}
			}
			vptr.Set(reflect.ValueOf(parsed))
			return nil
		case '0':
			parsed := time.Unix(tok.Int(), 0)
			vptr.Set(reflect.ValueOf(parsed))
			return nil
		default:
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected time string, integer unix seconds, or null, got json kind %v", tok.Kind()),
			}
		}
	}
}

func makeIntUnmarshaler(t reflect.Type) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: int decoder handles null, number, and quoted integer strings.
		switch tok.Kind() {
		case 'n':
			vptr.SetInt(0)
		case '0':
			vptr.SetInt(tok.Int())
		case '"':
			var val int64
			if val, err = strconv.ParseInt(tok.String(), 10, 64); err == nil {
				vptr.SetInt(val)
			} else {
				err = &json.SemanticError{
					ByteOffset:  dec.InputOffset(),
					JSONPointer: dec.StackPointer(),
					JSONKind:    tok.Kind(),
					GoType:      t,
					Err:         fmt.Errorf("jsonr: failed to parse integer from string: %q; error: %w", tok.String(), err),
				}
			}
		default:
			err = &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected integer, got %v", tok.Kind()),
			}
		}
		return err
	}
}

func makeUintUnmarshaler(t reflect.Type) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: uint decoder handles null, number, and quoted integer strings.
		switch tok.Kind() {
		case 'n':
			vptr.SetUint(0)
		case '0':
			vptr.SetUint(tok.Uint())
		case '"':
			var val uint64
			if val, err = strconv.ParseUint(tok.String(), 10, 64); err == nil {
				vptr.SetUint(val)
			} else {
				err = &json.SemanticError{
					ByteOffset:  dec.InputOffset(),
					JSONPointer: dec.StackPointer(),
					JSONKind:    tok.Kind(),
					GoType:      t,
					Err:         fmt.Errorf("jsonr: failed to parse unsigned integer from string: %q; error: %w", tok.String(), err),
				}
			}
		default:
			err = &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected unsigned integer, got %v", tok.Kind()),
			}
		}
		return err
	}
}

func makeFloatUnmarshaler(t reflect.Type) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: float decoder handles null, number, and quoted float strings.
		switch tok.Kind() {
		case 'n':
			vptr.SetFloat(0)
		case '0':
			vptr.SetFloat(tok.Float())
		case '"':
			var val float64
			if val, err = strconv.ParseFloat(tok.String(), 64); err == nil {
				vptr.SetFloat(val)
			} else {
				err = &json.SemanticError{
					ByteOffset:  dec.InputOffset(),
					JSONPointer: dec.StackPointer(),
					JSONKind:    tok.Kind(),
					GoType:      t,
					Err:         fmt.Errorf("jsonr: failed to parse float from string: %q; error: %w", tok.String(), err),
				}
			}
		default:
			err = &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected float, got %v", tok.Kind()),
			}
		}
		return err
	}
}

// makeMapKeyUnmarshaler decodes one JSON value for object key mapKey into map[string]T (or a ptr wrapper).
func makeMapKeyUnmarshaler(mapKey string, elem reflect.Type) (unmarshaler, error) {
	elemUnmar, err := getUnmarshaler(elem)
	if err != nil {
		return nil, err
	}
	k := reflect.ValueOf(mapKey)
	return func(dec *jsontext.Decoder, mapVal reflect.Value) error {
		if mapVal.IsNil() {
			mapVal.Set(reflect.MakeMap(mapVal.Type()))
		}
		el := reflect.New(elem).Elem()
		if err := elemUnmar(dec, el); err != nil {
			return err
		}
		mapVal.SetMapIndex(k, el)
		return nil
	}, nil
}

//nolint:gocognit // jsonr: map decode coordinates key/elem unmarshalers and object traversal.
func makeMapUnmarshaler(t reflect.Type) unmarshaler {
	var (
		once    sync.Once
		keyFncs unmarshaler
		valFncs unmarshaler
		initErr error
	)
	init := func() {
		keyFncs, initErr = getUnmarshaler(t.Key())
		if initErr != nil {
			return
		}
		valFncs, initErr = getUnmarshaler(t.Elem())
	}
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		k := tok.Kind()
		if k == 'n' {
			vptr.SetZero()
			return nil
		}
		if k != '{' {
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected object start '{', got %v", tok.Kind()),
			}
		}
		once.Do(init)
		if initErr != nil {
			return initErr
		}
		if vptr.IsNil() {
			vptr.Set(reflect.MakeMap(t))
		}
		key := reflect.New(t.Key()).Elem()
		val := reflect.New(t.Elem()).Elem()
		for dec.PeekKind() != '}' {
			if err = keyFncs(dec, key); err != nil {
				return err
			}
			if err = valFncs(dec, val); err != nil {
				return err
			}
			vptr.SetMapIndex(key, val)
			key.SetZero()
			val.SetZero()
		}
		_, err = dec.ReadToken()
		if err != nil {
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: failed to read object end '}': %w", err),
			}
		}
		return nil
	}
}

func makeSliceUnmarshaler(t reflect.Type) (unmarshaler, error) {
	elemUnmar, err := getUnmarshaler(t.Elem())
	if err != nil {
		return nil, err
	}
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: only null and array opens are valid for a slice.
		switch tok.Kind() {
		case 'n':
			vptr.SetZero()
			return nil
		case '[':
		default:
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected array or null, got %v", tok.Kind()),
			}
		}
		s := reflect.MakeSlice(t, 0, 0)
		for dec.PeekKind() != ']' {
			el := reflect.New(t.Elem()).Elem()
			if err := elemUnmar(dec, el); err != nil {
				return err
			}
			s = reflect.Append(s, el)
		}
		if _, err := dec.ReadToken(); err != nil {
			return fmt.Errorf("jsonr: read array end: %w", err)
		}
		vptr.Set(s)
		return nil
	}, nil
}

//nolint:gocognit // jsonr: fixed-size array fill with overflow skip is branchy but linear.
func makeArrayUnmarshaler(t reflect.Type) (unmarshaler, error) {
	if t.Kind() != reflect.Array {
		return nil, fmt.Errorf("jsonr: internal: expected array, got %v", t)
	}
	elemUnmar, err := getUnmarshaler(t.Elem())
	if err != nil {
		return nil, err
	}
	n := t.Len()
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: only null and array opens are valid for a fixed array.
		switch tok.Kind() {
		case 'n':
			vptr.SetZero()
			return nil
		case '[':
		default:
			return &json.SemanticError{
				ByteOffset:  dec.InputOffset(),
				JSONPointer: dec.StackPointer(),
				JSONKind:    tok.Kind(),
				GoType:      t,
				Err:         fmt.Errorf("jsonr: expected array or null, got %v", tok.Kind()),
			}
		}
		i := 0
		for dec.PeekKind() != ']' {
			if i < n {
				if err := elemUnmar(dec, vptr.Index(i)); err != nil {
					return err
				}
				i++
			} else {
				if err := dec.SkipValue(); err != nil {
					return fmt.Errorf("jsonr: skip extra array element: %w", err)
				}
			}
		}
		if _, err := dec.ReadToken(); err != nil {
			return fmt.Errorf("jsonr: read array end: %w", err)
		}
		return nil
	}, nil
}

// makeSliceAppendUnmarshaler reads a single JSON value and appends it to the slice field.
func makeSliceAppendUnmarshaler(t reflect.Type) (unmarshaler, error) {
	elemUnmar, err := getUnmarshaler(t.Elem())
	if err != nil {
		return nil, err
	}
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		el := reflect.New(t.Elem()).Elem()
		if err := elemUnmar(dec, el); err != nil {
			return err
		}
		if vptr.IsNil() {
			vptr.Set(reflect.MakeSlice(t, 0, 1))
		}
		vptr.Set(reflect.Append(vptr, el))
		return nil
	}, nil
}

func makePointerUnmarshaler(inner unmarshaler) unmarshaler {
	return func(dec *jsontext.Decoder, vptr reflect.Value) error {
		if dec.PeekKind() == 'n' {
			if _, err := dec.ReadToken(); err != nil {
				return fmt.Errorf("jsonr: read token: %w", err)
			}
			vptr.SetZero()
			return nil
		}
		if vptr.IsNil() {
			vptr.Set(reflect.New(vptr.Type().Elem()))
		}
		return inner(dec, vptr.Elem())
	}
}

func buildFieldUnmarshaler(t reflect.Type, sliceAppend bool) (unmarshaler, error) {
	//nolint:exhaustive // jsonr: field paths only recurse through pointers and slices; leaf types use getUnmarshaler.
	switch t.Kind() {
	case reflect.Pointer:
		inner, err := buildFieldUnmarshaler(t.Elem(), sliceAppend)
		if err != nil {
			return nil, err
		}
		return makePointerUnmarshaler(inner), nil
	case reflect.Slice:
		if sliceAppend {
			return makeSliceAppendUnmarshaler(t)
		}
		return makeSliceUnmarshaler(t)
	default:
		return getUnmarshaler(t)
	}
}

func pathsHaveWildcard(paths [][]string) bool {
	for _, path := range paths {
		if slices.Contains(path, "*") {
			return true
		}
	}
	return false
}

func isSliceOrPtrToSlice(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Slice
}

type structFields struct {
	t           reflect.Type
	structGraph *graph
	fields      map[*unmarshaler]int
}

func getCompiledStructFields(t reflect.Type) (*structFields, error) {
	if v, ok := structFieldsCache.Load(t); ok {
		sf, ok := v.(*structFields)
		if !ok {
			return nil, fmt.Errorf("jsonr: internal: bad struct fields cache entry for %v", t)
		}
		return sf, nil
	}
	sf, err := mapStructFields(t)
	if err != nil {
		return nil, err
	}
	structFieldsCache.Store(t, sf)
	return sf, nil
}

//nolint:gocognit,cyclop // jsonr: one pass over struct fields to register all jsonr paths into the trie.
func mapStructFields(t reflect.Type) (*structFields, error) {
	sf := &structFields{
		t:           t,
		structGraph: newGraph(),
		fields:      make(map[*unmarshaler]int),
	}
	for fi := range t.NumField() {
		field := t.Field(fi)
		tag := field.Tag.Get("jsonr")
		if tag == "" {
			continue
		}
		paths, err := dotNotationToPath(tag)
		if err != nil {
			return nil, fmt.Errorf("jsonr: field %s: %w", field.Name, err)
		}
		wild := pathsHaveWildcard(paths)
		if wild && !isSliceOrPtrToSlice(field.Type) {
			return nil, fmt.Errorf("jsonr: field %s: wildcard path %q requires a slice field type", field.Name, tag)
		}
		base := field.Type
		for base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		if len(paths) > 1 && base.Kind() != reflect.Map {
			return nil, fmt.Errorf("jsonr: field %s: multi-field path %q requires a map field type", field.Name, tag)
		}
		for _, path := range paths {
			var funmar unmarshaler
			if base.Kind() == reflect.Map && len(path) >= 2 {
				if base.Key().Kind() != reflect.String {
					return nil, fmt.Errorf("jsonr: field %s: map path with explicit keys requires map with string key type", field.Name)
				}
				jsonKey := path[len(path)-1]
				inner, err := makeMapKeyUnmarshaler(jsonKey, base.Elem())
				if err != nil {
					return nil, fmt.Errorf("jsonr: field %s: %w", field.Name, err)
				}
				funmar = inner
				for ft := field.Type; ft != base; ft = ft.Elem() {
					funmar = makePointerUnmarshaler(funmar)
				}
			} else {
				funmar, err = buildFieldUnmarshaler(field.Type, wild)
				if err != nil {
					return nil, fmt.Errorf("jsonr: field %s: %w", field.Name, err)
				}
			}
			sf.fields[sf.structGraph.register(path, funmar)] = fi
		}
	}
	return sf, nil
}
