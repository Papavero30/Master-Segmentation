package utils

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFilepathExtBehavior(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test.dcm", ".dcm"},
		{"test.FILE", ".FILE"}, // uppercase preserved
		{"test.file", ".file"}, // lowercase preserved
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ext := filepath.Ext(tt.input)
			if ext != tt.expected {
				t.Errorf("filepath.Ext(%q) = %q, want %q", tt.input, ext, tt.expected)
			}

			// Now test with strings.ToLower
			extLower := strings.ToLower(ext)
			t.Logf("  %q → Ext=%q → Lower=%q", tt.input, ext, extLower)
		})
	}
}

func TestDicomExtensionWhitelist(t *testing.T) {
	// Our whitelist from PathUtils.go
	whitelist := map[string]struct{}{
		".dcm":   {},
		".dic":   {},
		".dicm":  {},
		".dicom": {},
		".dc3":   {},
		".ima":   {},
		".file":  {}, // lowercase!
	}

	tests := []struct {
		filename string
		expected bool
	}{
		{"test.dcm", true},
		{"test.FILE", true}, // Should work after lowercase
		{"test.file", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			ext := filepath.Ext(tt.filename)
			extLower := strings.ToLower(ext)

			_, inWhitelist := whitelist[extLower]

			t.Logf("  File: %q", tt.filename)
			t.Logf("  Ext: %q", ext)
			t.Logf("  ExtLower: %q", extLower)
			t.Logf("  InWhitelist: %v", inWhitelist)

			if inWhitelist != tt.expected {
				t.Errorf("Expected inWhitelist=%v, got %v", tt.expected, inWhitelist)
			}
		})
	}
}
