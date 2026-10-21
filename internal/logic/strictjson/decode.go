// Package strictjson decodes one canonical JSON document without ambiguous keys.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

// MaxDepth bounds validation work independently of the transport's byte limit.
const MaxDepth = 64

var (
	ErrDuplicate = errors.New("duplicate JSON field")
	ErrSchema    = errors.New("unknown or noncanonical JSON field")
	ErrTrailing  = errors.New("additional JSON document")
	ErrDepth     = errors.New("JSON nesting exceeds limit")
)

// Decode preserves standard syntax/type errors, never includes input values in
// its own errors, and permits arbitrary keys only in declared maps/interfaces.
func Decode(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			return ErrTrailing
		}
		return err
	}
	checker := json.NewDecoder(bytes.NewReader(raw))
	checker.UseNumber()
	if err := value(checker, reflect.TypeOf(target), 0); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func value(decoder *json.Decoder, typ reflect.Type, depth int) error {
	if depth > MaxDepth {
		return ErrDepth
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch token {
	case json.Delim('{'):
		return object(decoder, typ, depth)
	case json.Delim('['):
		return array(decoder, typ, depth)
	}
	return err
}

func object(decoder *json.Decoder, typ reflect.Type, depth int) error {
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return ErrSchema
		}
		if seen[key] {
			return ErrDuplicate
		}
		seen[key] = true
		child, valid := fieldType(typ, key)
		if !valid {
			return ErrSchema
		}
		if err := value(decoder, child, depth+1); err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}

func fieldType(typ reflect.Type, key string) (reflect.Type, bool) {
	if typ == nil {
		return nil, true
	}
	if typ.Kind() == reflect.Map {
		return typ.Elem(), true
	}
	if typ.Kind() != reflect.Struct {
		return nil, true
	}
	for _, field := range reflect.VisibleFields(typ) {
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		if name == key {
			return field.Type, true
		}
	}
	return nil, false
}

func array(decoder *json.Decoder, typ reflect.Type, depth int) error {
	var elem reflect.Type
	if typ != nil && (typ.Kind() == reflect.Array || typ.Kind() == reflect.Slice) {
		elem = typ.Elem()
	}
	for decoder.More() {
		if err := value(decoder, elem, depth+1); err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}
