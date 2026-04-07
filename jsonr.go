package jsonr

import (
	"encoding/json/jsontext"
	"fmt"
	"io"
	"reflect"
)

// Unmarshal streams JSON from r and fills only the exported fields of v that carry a jsonr tag.
// v must be a non-nil pointer to a struct, map, slice, or array. For a struct, the JSON document
// must be an object at the top level (or null, in which case nothing is written). For a map,
// the JSON must be an object; for a slice or array, a JSON array.
func Unmarshal(r io.Reader, v any) error {
	dec := jsontext.NewDecoder(r)
	return UnmarshalDecode(dec, v)
}

// UnmarshalDecode is like Unmarshal but takes a jsontext.Decoder instead of an io.Reader.
// This is useful for streaming multiple top-level values from a single JSON document.
func UnmarshalDecode(dec *jsontext.Decoder, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("jsonr: v must be a non-nil pointer")
	}
	rv = rv.Elem()

	//nolint:exhaustive // jsonr: only struct, map, slice, and array targets are supported.
	switch rv.Kind() {
	case reflect.Struct:
		sf, err := getCompiledStructFields(rv.Type())
		if err != nil {
			return err
		}
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read token: %w", err)
		}
		//nolint:exhaustive // jsonr: only null and object are valid at top level for structs.
		switch tok.Kind() {
		case 'n':
			return nil
		case '{':
		default:
			return fmt.Errorf("jsonr: expected JSON object or null, got json kind %q", tok.Kind())
		}
		if err := decodeObject(dec, sf.structGraph.root, rv, sf); err != nil {
			return err
		}
		closeTok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read closing token: %w", err)
		}
		if closeTok.Kind() != '}' {
			return fmt.Errorf("jsonr: expected '}' closing top-level object, got json kind %q", closeTok.Kind())
		}
		return nil
	case reflect.Map, reflect.Slice, reflect.Array:
		f, err := getUnmarshaler(rv.Type())
		if err != nil {
			return err
		}
		return f(dec, rv)
	default:
		return fmt.Errorf("jsonr: v must be pointer to struct, map, slice, or array")
	}
}

// Inline decodes one JSON value selected by path from the reader's current position.
// Path uses the same dot notation as jsonr struct tags, including wildcard segments ("*")
// and multi-field bracket segments that must appear last (e.g. "y.[a,b]"). Wildcard paths
// collect one value per array element or object entry; when the rest of the path is missing
// for an element, that element is skipped with no placeholder. Multi-field paths decode only
// the listed keys into a map[string]T.
//
// The destination type T must not be a struct other than [time.Time]. Maps must use string keys.
//
// The document root must be a JSON object or array. After a successful call, the reader is
// advanced past the decoded value. Remaining tokens in the document (sibling keys, trailing
// values) are left unconsumed.
func Inline[T any](r io.Reader, path string) (T, error) {
	return InlineDecode[T](jsontext.NewDecoder(r), path)
}

// InlineDecode decodes one JSON value selected by path from the decoder's current position.
// See [Inline] for path syntax and wildcard / multi-field behavior.
//
// The decoder must be positioned at the start of a JSON object or array. After success the
// decoder has consumed the selected value; remaining input is left for the caller.
func InlineDecode[T any](dec *jsontext.Decoder, path string) (T, error) {
	var zero T
	segs, err := parseInlinePath(path)
	if err != nil {
		return zero, err
	}
	typ := reflect.TypeFor[T]()
	if err := inlineAllowedDestination(typ); err != nil {
		return zero, err
	}
	if err := inlinePathMatchesType(segs, typ); err != nil {
		return zero, err
	}
	rv, err := inlineDecodeRoot(dec, segs, typ)
	if err != nil {
		return zero, err
	}
	out, ok := rv.Interface().(T)
	if !ok {
		return zero, fmt.Errorf("jsonr: internal: result type mismatch for %v", typ)
	}
	return out, nil
}
