package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runtimevault"
)

// vaultReadStore is the read-only slice of the vault the getters need. The
// getters never write, so the seam stays this narrow.
type vaultReadStore interface {
	middleware.Storage
	Close() error
}

// openVaultReadStore is the seam the tests replace: opening the real vault needs
// a bolt file behind a broker, and what the getters do with a record does not.
var openVaultReadStore = func(vaultPath string) (vaultReadStore, error) {
	return runtimevault.OpenReadOnly(vaultPath)
}

// The getters that walk a typed record are bounded: a run record is an operator
// artifact whose content fields can be megabytes, and a terminal that prints one
// without a cap is a terminal the operator has to kill.
const (
	defaultVaultGetMaxBytes  = 32 * 1024
	vaultSummaryTooLargeCode = "ERR_VAULT_SUMMARY_TOO_LARGE"
	vaultFieldTooLargeCode   = "ERR_VAULT_FIELD_TOO_LARGE"
)

// vaultRecordSchema is the typed record a vault key holds, when Matrix wrote the
// record and knows its shape. The shape comes from the record's own Go type, so
// there is exactly one contract: the one the writer marshals.
type vaultRecordSchema struct {
	Type   reflect.Type
	Fields []vaultRecordField
}

type vaultRecordField struct {
	Name string
	Type string
}

var vaultRecordTypes = []struct {
	prefix string
	value  interface{}
}{
	{prefix: "runtrace.run.", value: runtrace.Run{}},
}

// vaultSchemaFor returns the schema of the record a key holds, if Matrix writes
// that kind of record.
func vaultSchemaFor(key string) (vaultRecordSchema, bool) {
	for _, record := range vaultRecordTypes {
		if !strings.HasPrefix(key, record.prefix) {
			continue
		}
		recordType := reflect.TypeOf(record.value)
		return vaultRecordSchema{Type: recordType, Fields: vaultRecordFields(recordType)}, true
	}
	return vaultRecordSchema{}, false
}

func vaultRecordFields(recordType reflect.Type) []vaultRecordField {
	fields := make([]vaultRecordField, 0, recordType.NumField())
	for i := 0; i < recordType.NumField(); i++ {
		field := recordType.Field(i)
		if !field.IsExported() || field.Anonymous {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields = append(fields, vaultRecordField{Name: name, Type: vaultFieldType(field.Type)})
	}
	return fields
}

// vaultJSONTypes names the JSON type of a field with the same vocabulary the
// daemon's refusal uses, so a caller who read "expected object" reads "object" in
// the field list too. It is a table because the vocabulary is a contract, and a
// contract reads better as a list than as a tree of cases.
var vaultJSONTypes = map[reflect.Kind]string{
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

// vaultFieldType names the JSON type of a field, because that is what a caller
// writing a query needs to know. time.Time is checked before the table: it is the
// one struct that marshals to a string, and saying "object" for it would be wrong
// in the only place it matters.
func vaultFieldType(fieldType reflect.Type) string {
	if fieldType == reflect.TypeOf(time.Time{}) {
		return "string"
	}
	for fieldType != nil && fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	if fieldType == nil {
		return "value"
	}
	if name, found := vaultJSONTypes[fieldType.Kind()]; found {
		return name
	}
	return "value"
}

// vaultRecordSchemaText is what a caller sees before the parse error: the fields
// the record really has, with the type of each one, so the next call is a field
// access instead of a guess.
func vaultRecordSchemaText(key string) string {
	schema, ok := vaultSchemaFor(key)
	if !ok {
		return fmt.Sprintf("%s is not a typed record Matrix writes; use `matrix vault get %s` for a string value.\n", key, key)
	}
	var lines strings.Builder
	fmt.Fprintf(&lines, "%s holds a typed record (object), not a string.\n", key)
	fmt.Fprintf(&lines, "Fields (%d):\n", len(schema.Fields))
	for _, field := range schema.Fields {
		fmt.Fprintf(&lines, "  %-28s %s\n", field.Name, field.Type)
	}
	fmt.Fprintf(&lines, "Read one field:  matrix vault get %s --field <name>\n", key)
	fmt.Fprintf(&lines, "Run terminal:    matrix vault result <id>   (diagnostic)\n")
	fmt.Fprintf(&lines, "                 matrix vault summary <id>  (terminal summary only)\n")
	return lines.String()
}

// vaultRecordFieldValue reads one field out of a record. A string field prints
// its text, the way a string key does; every other field prints its json, so a
// caller can pipe it into another tool unchanged.
func vaultRecordFieldValue(blob []byte, key, field string) (string, error) {
	var record map[string]json.RawMessage
	if err := json.Unmarshal(blob, &record); err != nil {
		return "", fmt.Errorf("ERR_VAULT_PARSE %s does not hold a json record: %w", key, err)
	}
	raw, found := record[field]
	if !found {
		return "", fmt.Errorf("ERR_VAULT_FIELD_UNKNOWN %s has no field %q; run `matrix vault get %s --help` for the fields it has", key, field, key)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	return string(raw), nil
}

// vaultBoundedValue refuses a value over the cap instead of truncating it: a
// truncated record is a wrong answer that looks right, and the dedicated code
// tells the caller the cap was the reason, not the record.
func vaultBoundedValue(code, what string, value string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		return "", fmt.Errorf("ERR_VAULT_CAP_INVALID --max-bytes must be positive, got %d", maxBytes)
	}
	if len(value) > maxBytes {
		return "", fmt.Errorf("%s %s is %d bytes, over the %d byte cap; raise it with --max-bytes if that is intended", code, what, len(value), maxBytes)
	}
	return value, nil
}

// vaultRunOutcome reads the terminal phase of a run record from the record's own
// fields, in the shape the trace projection publishes as Outcome. The summary
// content stays out: this is the diagnostic answer, and the summary has its own
// getter.
func vaultRunOutcome(blob []byte) (runtrace.Outcome, error) {
	var run runtrace.Run
	if err := json.Unmarshal(blob, &run); err != nil {
		return runtrace.Outcome{}, fmt.Errorf("ERR_VAULT_PARSE the record is not a run record: %w", err)
	}
	return runtrace.Outcome{
		Status:     run.Status,
		StopReason: run.StopReason,
		SummaryRef: run.OutputRef,
		Error:      run.Error,
	}, nil
}

// vaultRunSummary is the terminal summary itself, and only that. A record whose
// terminal phase has not been written yet is refused rather than answered with
// the empty string, which a caller cannot tell from an empty summary.
func vaultRunSummary(blob []byte) (string, error) {
	var run runtrace.Run
	if err := json.Unmarshal(blob, &run); err != nil {
		return "", fmt.Errorf("ERR_VAULT_PARSE the record is not a run record: %w", err)
	}
	if run.Output == "" && run.OutputRef == "" {
		return "", fmt.Errorf("ERR_VAULT_SUMMARY_UNAVAILABLE the run has no terminal summary yet (status %q)", run.Status)
	}
	return run.Output, nil
}

// vaultRunKey names the record of a run, whether the caller passed the run id or
// the whole vault key.
func vaultRunKey(idOrKey string) string {
	if strings.HasPrefix(idOrKey, "runtrace.") {
		return idOrKey
	}
	return "runtrace.run." + idOrKey
}

// readVaultRecord reads one record from the read-only vault. Absence is a normal
// answer for a key nobody wrote, and each failure keeps the wording the getter
// has always used, so a caller matching on it sees no change.
func readVaultRecord(key string) ([]byte, error) {
	provider, err := openVaultReadStore(DefaultVaultPath)
	if err != nil {
		return nil, fmt.Errorf("the read-only vault could not be opened: %w", err)
	}
	defer func() { _ = provider.Close() }()

	blob, err := provider.Get(key)
	if err != nil {
		return nil, fmt.Errorf("the vault refused the read: %w", err)
	}
	if blob == nil {
		return nil, errVaultNotFound
	}
	return blob, nil
}

// errVaultNotFound is what a getter answers for a key the vault does not hold.
// The operator-facing "Not found" belongs to the command that prints it, not to
// the error: an error string that starts a sentence is a linter finding and a
// sentence in the wrong layer.
var errVaultNotFound = errors.New("not found")

// exitVaultReadError prints a failed read the way the vault commands always
// have: absence keeps its own message, every other failure keeps the wording the
// command was given.
func exitVaultReadError(err error) {
	if errors.Is(err, errVaultNotFound) {
		exitf("Not found")
	}
	exitf("Failed to get value: %v", err)
}
