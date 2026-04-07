package jsonr

import (
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"strconv"
)

//nolint:gocognit,cyclop // jsonr: object vs array vs scalar dispatch is inherently branchy.
func decodeValue(dec *jsontext.Decoder, child *node, v reflect.Value, sf *structFields) error {
	if child == nil {
		if err := dec.SkipValue(); err != nil {
			return fmt.Errorf("jsonr: skip value: %w", err)
		}
		return nil
	}
	if child.unmar != nil {
		fi := sf.fields[&child.unmar]
		if fi < 0 {
			return fmt.Errorf("jsonr: internal: missing field index for path")
		}
		f := v.Field(fi)
		if !f.CanSet() {
			return fmt.Errorf("jsonr: field %s is not settable (unexported?)", sf.t.Field(fi).Name)
		}
		return child.unmar(dec, f)
	}
	//nolint:exhaustive // jsonr: only objects, arrays, and scalars (via SkipValue) are traversed here.
	switch dec.PeekKind() {
	case '{':
		if _, err := dec.ReadToken(); err != nil {
			return fmt.Errorf("jsonr: read '{': %w", err)
		}
		if err := decodeObject(dec, child, v, sf); err != nil {
			return err
		}
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read object end: %w", err)
		}
		if tok.Kind() != '}' {
			return fmt.Errorf("jsonr: expected object end '}', got %v", tok.Kind())
		}
	case '[':
		if _, err := dec.ReadToken(); err != nil {
			return fmt.Errorf("jsonr: read '[': %w", err)
		}
		if err := decodeArray(dec, child, v, sf); err != nil {
			return err
		}
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read array end: %w", err)
		}
		if tok.Kind() != ']' {
			return fmt.Errorf("jsonr: expected array end ']', got %v", tok.Kind())
		}
	default:
		if err := dec.SkipValue(); err != nil {
			return fmt.Errorf("jsonr: skip value: %w", err)
		}
	}
	return nil
}

func decodeObject(dec *jsontext.Decoder, n *node, v reflect.Value, sf *structFields) error {
	for dec.PeekKind() != '}' {
		keyTok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("jsonr: read object key token: %w", err)
		}
		if keyTok.Kind() != '"' {
			return fmt.Errorf("jsonr: expected object name, got %v", keyTok.Kind())
		}
		key := keyTok.String()
		child := n.child(key)
		if err := decodeValue(dec, child, v, sf); err != nil {
			return err
		}
	}
	return nil
}

func decodeArray(dec *jsontext.Decoder, n *node, v reflect.Value, sf *structFields) error {
	idx := 0
	for dec.PeekKind() != ']' {
		seg := strconv.Itoa(idx)
		child := n.child(seg)
		if err := decodeValue(dec, child, v, sf); err != nil {
			return err
		}
		idx++
	}
	return nil
}
