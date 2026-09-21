package runconfig

import (
	"reflect"
	"testing"
)

// ----------------------------------------------------------------------------
// Adversarial configuration suite
// ----------------------------------------------------------------------------

// TestAdditionalDirectoriesNormaliseBeforeDedup keeps one directory from
// reaching the agent three times under three spellings.
func TestAdditionalDirectoriesNormaliseBeforeDedup(t *testing.T) {
	got, err := NormalizeAdditionalDirectories([]string{
		"/srv/data",
		"/srv/data/",
		"/srv/data/./",
		"/srv/data/../data",
		"/srv//data",
		"/srv/other",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"/srv/data", "/srv/other"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// TestAdditionalDirectoriesRejectRelativePaths keeps a relative entry from
// silently resolving against the daemon's working directory.
func TestAdditionalDirectoriesRejectRelativePaths(t *testing.T) {
	for _, value := range []string{"relative/path", "./here", "../up", "~"} {
		if _, err := NormalizeAdditionalDirectories([]string{value}); err == nil {
			t.Fatalf("%q must be rejected", value)
		}
	}
}

// TestAdditionalDirectoriesIgnoreBlankEntries keeps padding from becoming a
// directory named "".
func TestAdditionalDirectoriesIgnoreBlankEntries(t *testing.T) {
	got, err := NormalizeAdditionalDirectories([]string{"  ", "\t", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("blank entries must be dropped, got %v", got)
	}
}

// TestAdditionalDirectoriesEmptyInputStaysNil keeps the empty contract stable.
func TestAdditionalDirectoriesEmptyInputStaysNil(t *testing.T) {
	got, err := NormalizeAdditionalDirectories(nil)
	if err != nil || got != nil {
		t.Fatalf("nil input must stay nil: %v %v", got, err)
	}
}

// TestAdditionalDirectoriesPreservesOrder keeps the operator's ordering, which
// some agents use for search precedence.
func TestAdditionalDirectoriesPreservesOrder(t *testing.T) {
	got, err := NormalizeAdditionalDirectories([]string{"/z", "/a", "/m"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/z", "/a", "/m"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order changed: %v", got)
	}
}
