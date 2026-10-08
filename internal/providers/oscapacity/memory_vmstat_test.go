package oscapacity

import "testing"

func TestDarwinVMStatUsesActualPageSizeAndKeepsZero(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want uint64
	}{
		{"Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 10.\nPages inactive: 500.", 163840},
		{"Mach Virtual Memory Statistics: (page size of 4096 bytes)\nPages free: 0.", 0},
	} {
		free, err := parseVMStat([]byte(tc.raw))
		if err != nil || free != tc.want {
			t.Fatal(free, err)
		}
	}
}

func TestDarwinVMStatRejectsMissingMalformedAndOverflow(t *testing.T) {
	for _, raw := range []string{"Pages free: 100.", "(page size of nope bytes)\nPages free: 100.", "(page size of 16384 bytes)\nPages free: nope.", "(page size of 65536 bytes)\nPages free: 18446744073709551615."} {
		if _, err := parseVMStat([]byte(raw)); err == nil {
			t.Fatal("unobserved memory fabricated", raw)
		}
	}
}
