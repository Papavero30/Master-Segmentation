package config

import "testing"

func TestGetEnvIntOrDefault(t *testing.T) {
	t.Setenv("TEST_INT_VALID", "128")
	t.Setenv("TEST_INT_INVALID", "abc")
	t.Setenv("TEST_INT_EMPTY", "")

	if got := getEnvIntOrDefault("TEST_INT_VALID", 256); got != 128 {
		t.Fatalf("expected 128, got %d", got)
	}

	if got := getEnvIntOrDefault("TEST_INT_INVALID", 256); got != 256 {
		t.Fatalf("expected default 256 for invalid value, got %d", got)
	}

	if got := getEnvIntOrDefault("TEST_INT_EMPTY", 256); got != 256 {
		t.Fatalf("expected default 256 for empty value, got %d", got)
	}

	if got := getEnvIntOrDefault("TEST_INT_MISSING", 256); got != 256 {
		t.Fatalf("expected default 256 for missing value, got %d", got)
	}
}
