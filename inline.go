package jsonr

import (
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

type inlineSegKind int

const (
	segLiteral inlineSegKind = iota
	segWildcard
	segMulti
)

type inlineSeg struct {
	kind inlineSegKind
	lit  string
	keys []string
}

// parseInlinePath parses one jsonr path for Inline / InlineDecode without expanding
// multi-field brackets into separate paths. Multi-field segments must be last.
func parseInlinePath(dot string) ([]inlineSeg, error) {
	dot = strings.TrimSpace(dot)
	if dot == "" {
		return nil, fmt.Errorf("empty jsonr path")
	}
	parts := splitPathSegments(dot)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty jsonr path")
	}
	segs := make([]inlineSeg, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("empty path segment in %q", dot)
		}
		switch {
		case p == "*":
			segs = append(segs, inlineSeg{kind: segWildcard})
		case isMultiFieldSegment(p):
			if i != len(parts)-1 {
				return nil, fmt.Errorf("jsonr path %q: multi-field segment must be last", dot)
			}
			keys, err := parseMultiFieldSegment(p)
			if err != nil {
				return nil, fmt.Errorf("jsonr path %q: %w", dot, err)
			}
			for _, k := range keys {
				if strings.ContainsRune(k, ':') {
					return nil, fmt.Errorf("jsonr path %q: index ranges are not supported in Inline paths", dot)
				}
			}
			segs = append(segs, inlineSeg{kind: segMulti, keys: keys})
		default:
			segs = append(segs, inlineSeg{kind: segLiteral, lit: p})
		}
	}
	return segs, nil
}

func inlineSegsHaveWildcard(segs []inlineSeg) bool {
	for _, s := range segs {
		if s.kind == segWildcard {
			return true
		}
	}
	return false
}

// inlineAllowedDestination rejects struct types other than time.Time and validates
// nested maps/slices/arrays recursively.
func inlineAllowedDestination(t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return inlineAllowedDestinationInner(t)
}

func inlineAllowedDestinationInner(t reflect.Type) error {
	if t == reflect.TypeFor[time.Time]() {
		return nil
	}
	//nolint:exhaustive // jsonr: only JSON-backed kinds and collections are allowed for Inline.
	switch t.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return nil
	case reflect.Struct:
		return fmt.Errorf("jsonr: Inline destination cannot be struct type %v (time.Time is allowed)", t)
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return fmt.Errorf("jsonr: Inline map destination must use string keys")
		}
		return inlineAllowedDestinationInner(t.Elem())
	case reflect.Slice, reflect.Array:
		return inlineAllowedDestinationInner(t.Elem())
	default:
		return fmt.Errorf("jsonr: unsupported Inline destination type %v", t)
	}
}

func inlinePathMatchesType(segs []inlineSeg, t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if inlineSegsHaveWildcard(segs) {
		if t.Kind() != reflect.Slice {
			return fmt.Errorf("jsonr: wildcard path requires slice destination type")
		}
		return nil
	}
	if len(segs) > 0 && segs[len(segs)-1].kind == segMulti {
		if t.Kind() != reflect.Map || t.Key().Kind() != reflect.String {
			return fmt.Errorf("jsonr: multi-field path requires map[string]T destination")
		}
	}
	return nil
}

func inlineDecodeRoot(dec *jsontext.Decoder, segs []inlineSeg, typ reflect.Type) (reflect.Value, error) {
	if len(segs) == 0 {
		return reflect.Value{}, fmt.Errorf("jsonr: empty path")
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return reflect.Value{}, fmt.Errorf("jsonr: read token: %w", err)
	}
	return inlineMandatoryAfterOpen(dec, tok, segs, typ)
}

func inlineDecodeLeaf(dec *jsontext.Decoder, typ reflect.Type) (reflect.Value, error) {
	u, err := getUnmarshaler(typ)
	if err != nil {
		return reflect.Value{}, err
	}
	rv := reflect.New(typ).Elem()
	if err := u(dec, rv); err != nil {
		return reflect.Value{}, err
	}
	return rv, nil
}

func decodeInlineMultiFieldObject(dec *jsontext.Decoder, keys []string, typ reflect.Type) (reflect.Value, error) {
	elemU, err := getUnmarshaler(typ.Elem())
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.MakeMap(typ)
	for dec.PeekKind() != '}' {
		keyTok, err := dec.ReadToken()
		if err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: read object key: %w", err)
		}
		if keyTok.Kind() != '"' {
			return reflect.Value{}, fmt.Errorf("jsonr: expected object name, got json kind %q", keyTok.Kind())
		}
		key := keyTok.String()
		if !slices.Contains(keys, key) {
			if err := dec.SkipValue(); err != nil {
				return reflect.Value{}, fmt.Errorf("jsonr: skip object value: %w", err)
			}
			continue
		}
		el := reflect.New(typ.Elem()).Elem()
		if err := elemU(dec, el); err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: decode map value for key %q: %w", key, err)
		}
		out.SetMapIndex(reflect.ValueOf(key), el)
	}
	if _, err := dec.ReadToken(); err != nil {
		return reflect.Value{}, fmt.Errorf("jsonr: read object end: %w", err)
	}
	return out, nil
}

//nolint:gocognit,cyclop // jsonr: array vs object wildcard iteration is branchy but linear over the input.
func inlineWildcardCollectMandatory(dec *jsontext.Decoder, open jsontext.Token, rest []inlineSeg, sliceType reflect.Type) (reflect.Value, error) {
	elemT := sliceType.Elem()
	out := reflect.MakeSlice(sliceType, 0, 0)
	//nolint:exhaustive // jsonr: wildcard applies to object or array containers only.
	switch open.Kind() {
	case '[':
		for dec.PeekKind() != ']' {
			before := dec.InputOffset()
			ok, v, err := inlineTryDecodeFromValueStart(dec, rest, elemT)
			if err != nil {
				return reflect.Value{}, err
			}
			if ok {
				out = reflect.Append(out, v)
			} else if dec.InputOffset() == before {
				// try did not consume a value; drop one element from the array.
				if err := dec.SkipValue(); err != nil {
					return reflect.Value{}, fmt.Errorf("jsonr: skip array element: %w", err)
				}
			}
		}
		if _, err := dec.ReadToken(); err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: read array end: %w", err)
		}
	case '{':
		for dec.PeekKind() != '}' {
			keyTok, err := dec.ReadToken()
			if err != nil {
				return reflect.Value{}, fmt.Errorf("jsonr: read object key: %w", err)
			}
			if keyTok.Kind() != '"' {
				return reflect.Value{}, fmt.Errorf("jsonr: expected object name, got json kind %q", keyTok.Kind())
			}
			before := dec.InputOffset()
			ok, v, err := inlineTryDecodeFromValueStart(dec, rest, elemT)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("jsonr: decode wildcard element: %w", err)
			}
			if ok {
				out = reflect.Append(out, v)
			} else if dec.InputOffset() == before {
				if err := dec.SkipValue(); err != nil {
					return reflect.Value{}, fmt.Errorf("jsonr: skip object value: %w", err)
				}
			}
		}
		if _, err := dec.ReadToken(); err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: read object end: %w", err)
		}
	default:
		return reflect.Value{}, fmt.Errorf("jsonr: expected object or array for wildcard segment, got json kind %q", open.Kind())
	}
	return out, nil
}

//nolint:gocognit,cyclop // jsonr: literal vs wildcard vs multi-field dispatch at one depth.
func inlineMandatoryAfterOpen(dec *jsontext.Decoder, inner jsontext.Token, segs []inlineSeg, typ reflect.Type) (reflect.Value, error) {
	if len(segs) == 0 {
		return reflect.Value{}, fmt.Errorf("jsonr: internal: empty segment list")
	}

	if len(segs) == 1 && segs[0].kind == segMulti {
		if inner.Kind() != '{' {
			return reflect.Value{}, fmt.Errorf("jsonr: expected object for multi-field segment, got json kind %q", inner.Kind())
		}
		return decodeInlineMultiFieldObject(dec, segs[0].keys, typ)
	}

	if segs[0].kind == segWildcard {
		if inner.Kind() != '{' && inner.Kind() != '[' {
			return reflect.Value{}, fmt.Errorf("jsonr: expected object or array for wildcard segment, got json kind %q", inner.Kind())
		}
		return inlineWildcardCollectMandatory(dec, inner, segs[1:], typ)
	}

	if segs[0].kind != segLiteral {
		return reflect.Value{}, fmt.Errorf("jsonr: internal: unexpected segment kind")
	}
	lit := segs[0].lit
	//nolint:exhaustive // jsonr: path navigation only descends into objects and arrays.
	switch inner.Kind() {
	case '{':
		if err := seekKeyInObject(dec, lit); err != nil {
			return reflect.Value{}, err
		}
		if len(segs) == 1 {
			return inlineDecodeLeaf(dec, typ)
		}
		inner2, err := dec.ReadToken()
		if err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: read value token: %w", err)
		}
		return inlineMandatoryAfterOpen(dec, inner2, segs[1:], typ)
	case '[':
		idx, err := strconv.Atoi(lit)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: path segment %q is not a valid array index", lit)
		}
		if err := seekIndexInArray(dec, idx); err != nil {
			return reflect.Value{}, err
		}
		if len(segs) == 1 {
			return inlineDecodeLeaf(dec, typ)
		}
		inner2, err := dec.ReadToken()
		if err != nil {
			return reflect.Value{}, fmt.Errorf("jsonr: read array element token: %w", err)
		}
		return inlineMandatoryAfterOpen(dec, inner2, segs[1:], typ)
	default:
		return reflect.Value{}, fmt.Errorf("jsonr: cannot traverse JSON kind %q (need object or array for path segment %q)",
			inner.Kind(), lit)
	}
}

// inlineTryDecodeFromValueStart decodes one JSON value starting at dec's next token.
// It returns ok=false when the path is absent (wildcard miss) without a type error.
//
//nolint:gocognit,cyclop // jsonr: try-path mirrors optional trie traversal with explicit skip/consume rules.
func inlineTryDecodeFromValueStart(dec *jsontext.Decoder, segs []inlineSeg, typ reflect.Type) (ok bool, v reflect.Value, err error) {
	if len(segs) == 0 {
		rv := reflect.New(typ).Elem()
		u, uerr := getUnmarshaler(typ)
		if uerr != nil {
			return false, reflect.Value{}, uerr
		}
		if err := u(dec, rv); err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: decode leaf: %w", err)
		}
		return true, rv, nil
	}

	if len(segs) == 1 && segs[0].kind == segMulti {
		if dec.PeekKind() != '{' {
			return false, reflect.Value{}, nil
		}
		if _, err := dec.ReadToken(); err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: read token: %w", err)
		}
		val, err := decodeInlineMultiFieldObject(dec, segs[0].keys, typ)
		if err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: multi-field object: %w", err)
		}
		return true, val, nil
	}

	if segs[0].kind == segWildcard {
		//nolint:exhaustive // jsonr: wildcard expects object or array; other kinds return ok=false without consuming.
		switch dec.PeekKind() {
		case '{', '[':
			tok, err := dec.ReadToken()
			if err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: read token: %w", err)
			}
			val, err := inlineWildcardCollectMandatory(dec, tok, segs[1:], typ)
			if err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: nested wildcard: %w", err)
			}
			return true, val, nil
		default:
			return false, reflect.Value{}, nil
		}
	}

	if segs[0].kind != segLiteral {
		return false, reflect.Value{}, fmt.Errorf("jsonr: internal: unexpected segment")
	}
	lit := segs[0].lit

	//nolint:exhaustive // jsonr: try-descend only through object or array.
	switch dec.PeekKind() {
	case '{':
		if _, err := dec.ReadToken(); err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: read token: %w", err)
		}
		for dec.PeekKind() != '}' {
			keyTok, err := dec.ReadToken()
			if err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: read object key: %w", err)
			}
			if keyTok.Kind() != '"' {
				return false, reflect.Value{}, fmt.Errorf("jsonr: expected object name, got json kind %q", keyTok.Kind())
			}
			key := keyTok.String()
			if key != lit {
				if err := dec.SkipValue(); err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: skip object value: %w", err)
				}
				continue
			}
			okChild, val, err := inlineTryDecodeFromValueStart(dec, segs[1:], typ)
			if err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: decode object field: %w", err)
			}
			if !okChild {
				for dec.PeekKind() != '}' {
					_, err := dec.ReadToken()
					if err != nil {
						return false, reflect.Value{}, fmt.Errorf("jsonr: read object key: %w", err)
					}
					if err := dec.SkipValue(); err != nil {
						return false, reflect.Value{}, fmt.Errorf("jsonr: skip object value: %w", err)
					}
				}
				if _, err := dec.ReadToken(); err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: read object end: %w", err)
				}
				return false, reflect.Value{}, nil
			}
			for dec.PeekKind() != '}' {
				_, err := dec.ReadToken()
				if err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: read object key: %w", err)
				}
				if err := dec.SkipValue(); err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: skip object value: %w", err)
				}
			}
			if _, err := dec.ReadToken(); err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: read object end: %w", err)
			}
			return true, val, nil
		}
		if _, err := dec.ReadToken(); err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: read object end: %w", err)
		}
		return false, reflect.Value{}, nil
	case '[':
		if _, err := dec.ReadToken(); err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: read token: %w", err)
		}
		idx, aerr := strconv.Atoi(lit)
		if aerr != nil {
			for dec.PeekKind() != ']' {
				if err := dec.SkipValue(); err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: skip array element: %w", err)
				}
			}
			if _, err := dec.ReadToken(); err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: read array end: %w", err)
			}
			return false, reflect.Value{}, nil
		}
		i := 0
		for dec.PeekKind() != ']' {
			if i == idx {
				okChild, val, err := inlineTryDecodeFromValueStart(dec, segs[1:], typ)
				if err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: decode array element: %w", err)
				}
				if !okChild {
					if err := dec.SkipValue(); err != nil {
						return false, reflect.Value{}, fmt.Errorf("jsonr: skip array element: %w", err)
					}
				}
				for dec.PeekKind() != ']' {
					if err := dec.SkipValue(); err != nil {
						return false, reflect.Value{}, fmt.Errorf("jsonr: skip array element: %w", err)
					}
				}
				if _, err := dec.ReadToken(); err != nil {
					return false, reflect.Value{}, fmt.Errorf("jsonr: read array end: %w", err)
				}
				return okChild, val, nil
			}
			if err := dec.SkipValue(); err != nil {
				return false, reflect.Value{}, fmt.Errorf("jsonr: skip array element: %w", err)
			}
			i++
		}
		if _, err := dec.ReadToken(); err != nil {
			return false, reflect.Value{}, fmt.Errorf("jsonr: read array end: %w", err)
		}
		return false, reflect.Value{}, nil
	default:
		return false, reflect.Value{}, nil
	}
}

func seekKeyInObject(dec *jsontext.Decoder, want string) error {
	for dec.PeekKind() != '}' {
		keyTok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read object key: %w", err)
		}
		if keyTok.Kind() != '"' {
			return fmt.Errorf("jsonr: expected object name, got json kind %q", keyTok.Kind())
		}
		key := keyTok.String()
		if key == want {
			return nil
		}
		if err := dec.SkipValue(); err != nil {
			return fmt.Errorf("jsonr: skip object value: %w", err)
		}
	}
	return fmt.Errorf("jsonr: key %q not found in object", want)
}

func seekIndexInArray(dec *jsontext.Decoder, want int) error {
	if want < 0 {
		return fmt.Errorf("jsonr: negative array index %d", want)
	}
	i := 0
	for dec.PeekKind() != ']' {
		if i == want {
			return nil
		}
		if err := dec.SkipValue(); err != nil {
			return fmt.Errorf("jsonr: skip array element: %w", err)
		}
		i++
	}
	return fmt.Errorf("jsonr: array index %d out of range (len %d)", want, i)
}
