package main

import (
	"errors"
	"testing"
)

func TestCapacityLimitsMissingDefaultsButReadFailureDoesNotDisablePolicy(t *testing.T) {
	limits, err := runtimeCapacityLimits(func(string) (string, error) { return "", nil })()
	if err != nil || limits.MaxConcurrent != 0 || limits.MinDiskFreeBytes != 0 {
		t.Fatal(limits, err)
	}
	if _, err := runtimeCapacityLimits(func(string) (string, error) { return "", errors.New("vault unavailable") })(); err == nil {
		t.Fatal("read failure silently disabled admission")
	}
	for _, value := range []string{"-1", "invalid"} {
		if _, err := runtimeCapacityLimits(func(string) (string, error) { return value, nil })(); err == nil {
			t.Fatal("invalid config ignored")
		}
	}
}
