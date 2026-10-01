package runapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/jsontype"
)

// runRequestMaxBytes bounds the body of a run submission. The body is decoded
// into memory before anything about it can be checked, so without a ceiling the
// caller decides how much memory one authenticated request costs. The number is
// generous on purpose: a real request carries a prompt, its context and a
// declared delivery contract, which is kilobytes, and a limit a legitimate call
// can hit would be a defect of its own.
const runRequestMaxBytes = 1 << 20

// writeRunRequestDecodeError answers a body the decoder could not read, and says
// which of the three failures it was.
//
// A document that is not json, a document that is json but does not match the
// request contract, and a body too large to read at all are different problems
// for the caller: the first is retried after the syntax is fixed, the second
// after the field is, and the third by sending less. Answering all of them with
// "invalid json" sends a caller whose document was perfectly valid to look for a
// broken body it does not have. The schema answer names the field and the type
// the contract expects; neither answer echoes the body or a value from it.
func writeRunRequestDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		// Nothing can be said about the shape of a document that was never read
		// to the end, so this is its own answer: the limit, and no claim about
		// the json.
		http.Error(w, fmt.Sprintf("Request Entity Too Large: the run request body is limited to %d bytes", tooLarge.Limit), http.StatusRequestEntityTooLarge)
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
	expected, got := jsontype.Expected(typeErr.Type), jsontype.Reported(typeErr.Value)
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
