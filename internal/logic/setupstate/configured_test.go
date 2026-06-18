package setupstate

import "testing"

func TestConfigured(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "json bool true", data: []byte("true"), want: true},
		{name: "json bool false", data: []byte("false"), want: false},
		{name: "vault string true", data: []byte(`"true"`), want: true},
		{name: "vault string false", data: []byte(`"false"`), want: false},
		{name: "empty", data: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Configured(tt.data); got != tt.want {
				t.Fatalf("Configured(%q) = %v, want %v", string(tt.data), got, tt.want)
			}
		})
	}
}
