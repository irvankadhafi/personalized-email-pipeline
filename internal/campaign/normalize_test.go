package campaign

import "testing"

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{" User@Example.COM ", "user@example.com", true},
		{"a+b@example.com", "a+b@example.com", true},
		{"first.last@example.com", "first.last@example.com", true},
		{"missing-at.example.com", "", false},
		{"a@@example.com", "", false},
		{"a@-example.com", "", false},
		{"a@example..com", "", false},
		{"a b@example.com", "", false},
		{"é@example.com", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizeEmail(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Errorf("NormalizeEmail(%q) = %q,%v; want %q,%v", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestUsableName(t *testing.T) {
	if got, ok := UsableName("  Ada  "); got != "Ada" || !ok {
		t.Fatalf("UsableName named = %q,%v", got, ok)
	}
	if got, ok := UsableName(" \t "); got != "" || ok {
		t.Fatalf("UsableName blank = %q,%v", got, ok)
	}
	if got, ok := UsableName("Ada\nInjected"); got != "Ada Injected" || !ok {
		t.Fatalf("UsableName controls = %q,%v", got, ok)
	}
}
