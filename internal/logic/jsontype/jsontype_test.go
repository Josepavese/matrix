package jsontype

import (
	"reflect"
	"testing"
	"time"
)

// TestExpectedNamesTheTypeACallerReads pins the vocabulary as data: these words
// are what an API answer says, so a change here is a change to a contract.
func TestExpectedNamesTheTypeACallerReads(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{"string", reflect.TypeOf(""), "string"},
		{"bool", reflect.TypeOf(false), "boolean"},
		{"int", reflect.TypeOf(0), "number"},
		{"float", reflect.TypeOf(0.0), "number"},
		{"slice", reflect.TypeOf([]string{}), "array"},
		{"map", reflect.TypeOf(map[string]string{}), "object"},
		{"struct", reflect.TypeOf(struct{ A int }{}), "object"},
		{"pointer to string", reflect.TypeOf(new(string)), "string"},
		{"time is the struct that marshals to a string", reflect.TypeOf(time.Time{}), "string"},
		{"an unknown kind is named generically", reflect.TypeOf(make(chan int)), "value"},
		{"a nil type is named generically", nil, "value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Expected(tc.typ); got != tc.want {
				t.Fatalf("Expected(%v) = %q, want %q", tc.typ, got, tc.want)
			}
		})
	}
}

// TestReportedSpeaksTheSameVocabularyAsExpected keeps the two sides of one
// mismatch comparable: the decoder reports "bool" and an empty value, and neither
// may reach the caller in those words.
func TestReportedSpeaksTheSameVocabularyAsExpected(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"bool", "boolean"},
		{"string", "string"},
		{"number", "number"},
		{"", "value"},
		{"  ", "value"},
	} {
		if got := Reported(tc.value); got != tc.want {
			t.Fatalf("Reported(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
