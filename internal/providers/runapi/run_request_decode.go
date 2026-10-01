package runapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

// writeRunRequestDecodeError answers a body the decoder could not read, and says
// which of the two failures it was.
//
// A document that is not json and a document that is json but does not match the
// request contract are different problems for the caller: the first is retried
// after the syntax is fixed, the second after the field is. Answering both with
// "invalid json" sends a caller whose document was perfectly valid to look for a
// broken body it does not have. The schema answer names the field and the type
// the contract expects; neither answer echoes the body or a value from it.
func writeRunRequestDecodeError(w http.ResponseWriter, err error) {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		http.Error(w, "Bad Request: invalid json: the request body is empty", http.StatusBadRequest)
	case errors.Is(err, io.ErrUnexpectedEOF):
		http.Error(w, "Bad Request: invalid json: the request body ends before the json does", http.StatusBadRequest)
	case errors.As(err, &syntaxErr):
		http.Error(w, fmt.Sprintf("Bad Request: invalid json: syntax error at byte offset %d", syntaxErr.Offset), http.StatusBadRequest)
	case errors.As(err, &typeErr):
		http.Error(w, "Bad Request: "+runRequestSchemaMismatch(typeErr), http.StatusBadRequest)
	default:
		http.Error(w, "Bad Request: invalid request body: the json does not match the request schema", http.StatusBadRequest)
	}
}

// runRequestSchemaMismatch names the field and the type the request contract
// expects, or answers generically when the mismatch was not found while filling
// the request itself.
func runRequestSchemaMismatch(typeErr *json.UnmarshalTypeError) string {
	expected, got := expectedJSONType(typeErr.Type), reportedJSONType(typeErr.Value)
	if !namesARequestField(typeErr) {
		return fmt.Sprintf("invalid request body: expected %s, got %s", expected, got)
	}
	return fmt.Sprintf("invalid request field %q: expected %s, got %s", typeErr.Field, expected, got)
}

// namesARequestField reports whether the decoder found the mismatch while filling
// the request, rather than inside a field's own unmarshaller. The decoder records
// that by naming the struct it was filling, which is the only evidence the error
// carries: a nested unmarshaller decodes its own document, so its path is
// relative to that document and naming it would point the caller at a field it
// never sent at this level.
func namesARequestField(typeErr *json.UnmarshalTypeError) bool {
	if strings.TrimSpace(typeErr.Field) == "" {
		return false
	}
	return typeErr.Struct == reflect.TypeOf(runRequest{}).Name()
}

// jsonTypeNames is the vocabulary the answers describe a field's type with, so a
// caller reads "object" instead of a Go type from the daemon's internals. It is a
// table rather than a switch because the vocabulary is a contract, and a contract
// reads better as a list than as a tree of cases.
var jsonTypeNames = map[reflect.Kind]string{
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

// expectedJSONType is how the answer names the type a request field is declared
// with. A type the vocabulary does not know is named generically rather than
// guessed.
func expectedJSONType(t reflect.Type) string {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return "value"
	}
	if name, found := jsonTypeNames[t.Kind()]; found {
		return name
	}
	return "value"
}

// reportedJSONType is the type the decoder found in the document, in the same
// vocabulary the expected type is named with.
func reportedJSONType(value string) string {
	if value == "bool" {
		return "boolean"
	}
	if strings.TrimSpace(value) == "" {
		return "value"
	}
	return value
}
