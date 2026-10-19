package validate

import "testing"

func TestEmail(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{"alice@example.com", true},
		{"alice.smith+pets@mail.example.com", true},
		{"", false},
		{"alice.example.com", false},
		{"alice@", false},
		{"@example.com", false},
		{"Alice <alice@example.com>", false},
		{" alice@example.com", false},
	}

	for _, tt := range tests {
		if got := Email(tt.email); got != tt.want {
			t.Errorf("Email(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}
