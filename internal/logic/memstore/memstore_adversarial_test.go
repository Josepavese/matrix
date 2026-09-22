package memstore

import (
	"fmt"
	"sync"
	"testing"
)

// TestStorageSetGetReturnsACopy pins the isolation contract: a caller that
// mutates what it passed in, or what it got back, must not be able to change the
// stored value from under another reader.
func TestStorageSetGetReturnsACopy(t *testing.T) {
	store := New()

	written := []byte("first")
	if err := store.Set("k", written); err != nil {
		t.Fatalf("set: %v", err)
	}
	written[0] = 'X'

	got, err := store.Get("k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != "first" {
		t.Fatalf("mutating the caller's slice changed the stored value: got %q", got)
	}

	got[0] = 'Y'
	again, err := store.Get("k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(again) != "first" {
		t.Fatalf("mutating a returned slice changed the stored value: got %q", again)
	}
}

func TestStorageMissingKeyIsNotAnError(t *testing.T) {
	store := New()
	value, err := store.Get("absent")
	if err != nil {
		t.Fatalf("a missing key must not be an error: %v", err)
	}
	if value != nil {
		t.Fatalf("a missing key must read as nil, got %q", value)
	}
}

func TestStorageDeleteRemovesOnlyTheNamedKey(t *testing.T) {
	store := New()
	for _, key := range []string{"a", "ab", "b"} {
		if err := store.Set(key, []byte(key)); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	if err := store.Delete("a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if value, _ := store.Get("a"); value != nil {
		t.Fatal("the deleted key must not be readable")
	}
	if value, _ := store.Get("ab"); string(value) != "ab" {
		t.Fatal("delete removed a key it was not asked to remove")
	}
	// Deleting an absent key is a no-op, not a failure.
	if err := store.Delete("absent"); err != nil {
		t.Fatalf("deleting an absent key must be a no-op: %v", err)
	}
}

func TestStorageListFiltersByPrefix(t *testing.T) {
	store := New()
	for _, key := range []string{"run:1", "run:2", "session:1"} {
		if err := store.Set(key, []byte(key)); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	keys, err := store.List("run:")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("prefix filter returned %d keys, want 2: %v", len(keys), keys)
	}
	for _, key := range keys {
		if key == "session:1" {
			t.Fatal("list returned a key that does not match the prefix")
		}
	}
	empty, err := store.List("nothing:")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("a non-matching prefix must return no keys, got %v", empty)
	}
}

// TestStorageConcurrentAccess exercises the lock under the race detector.
func TestStorageConcurrentAccess(t *testing.T) {
	store := New()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				key := fmt.Sprintf("key-%d", i%5)
				if err := store.Set(key, []byte(key)); err != nil {
					t.Errorf("set: %v", err)
					return
				}
				if _, err := store.Get(key); err != nil {
					t.Errorf("get: %v", err)
					return
				}
				if _, err := store.List("key-"); err != nil {
					t.Errorf("list: %v", err)
					return
				}
				if i%7 == 0 {
					if err := store.Delete(key); err != nil {
						t.Errorf("delete: %v", err)
						return
					}
				}
			}
		}(worker)
	}
	wg.Wait()
}
