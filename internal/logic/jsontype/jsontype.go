// Package jsontype names Go types in the JSON vocabulary an API answer uses.
//
// Two places in Matrix describe a type to a caller: the refusal of a request field
// whose shape is wrong ("expected object, got string") and the field list of a
// typed record, which says what each field is before the caller gets the parse
// error. They must use the same words, or a caller reads "object" in one answer
// and something else in the next.
//
// The vocabulary is a table rather than a tree of cases because it is a contract,
// and a contract reads better as a list than as a tree.
package jsontype

import (
	"reflect"
	"strings"
	"time"
)

// names maps the kinds a JSON document can carry to the word the contract uses.
var names = map[reflect.Kind]string{
	reflect.String:  "string",
	reflect.Bool:    "boolean",
	reflect.Struct:  "object",
	reflect.Map:     "object",
	reflect.Slice:   "array",
	reflect.Array:   "array",
	reflect.Int:     "number",
	reflect.Int8:    "number",
	reflect.Int16:   "number",
	reflect.Int32:   "number",
	reflect.Int64:   "number",
	reflect.Uint:    "number",
	reflect.Uint8:   "number",
	reflect.Uint16:  "number",
	reflect.Uint32:  "number",
	reflect.Uint64:  "number",
	reflect.Float32: "number",
	reflect.Float64: "number",
}

// Expected names the type a field is declared with. The Go type is unwrapped from
// its pointers first, and a type the vocabulary does not know is named generically
// rather than guessed.
//
// time.Time is the one struct that marshals to a string, so it is checked before
// the table: saying "object" for a field that arrives as a string would be wrong in
// the only place it matters.
func Expected(t reflect.Type) string {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return "value"
	}
	if t == reflect.TypeOf(time.Time{}) {
		return "string"
	}
	if name, found := names[t.Kind()]; found {
		return name
	}
	return "value"
}

// Reported names the type the decoder found in the document, in the same
// vocabulary Expected uses, so one answer can name both sides of a mismatch.
func Reported(value string) string {
	if value == "bool" {
		return "boolean"
	}
	if strings.TrimSpace(value) == "" {
		return "value"
	}
	return value
}
