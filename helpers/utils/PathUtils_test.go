package utils

import (
	"fmt"
	"testing"
)

func TestPathNormalizationWithExtensions(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test.dcm", "test.dcm"},
		{"test.FILE", "test.FILE"},
		{"test.file", "test.file"},
		{"patient..backup.dcm", "patient..backup.dcm"},
		{"test", "test"},
		{"./test.dcm", "test.dcm"},
		{"/test.dcm", "test.dcm"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeAndCleanPath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeAndCleanPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsDicomFileWithExtensions(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"test.dcm", true},
		{"test.FILE", true}, // Should work (uppercase)
		{"test.file", true}, // Should work (lowercase)
		{"test", true},      // No extension
		{"patient..backup.dcm", true},
		{"test.txt", false}, // Non-DICOM
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsDicomFile(tt.input)
			if result != tt.expected {
				t.Errorf("IsDicomFile(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidatePathSecurityWithDoubleDots(t *testing.T) {
	tests := []struct {
		input       string
		shouldError bool
	}{
		{"test.dcm", false},
		{"patient..backup.dcm", false}, // Should allow (.. in filename)
		{"../test.dcm", true},          // Should block (path traversal)
		{"folder/../test.dcm", true},   // Should block (path traversal)
		{"..", true},                   // Should block (exact match)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := ValidatePathSecurity(tt.input)
			hasError := (err != nil)
			if hasError != tt.shouldError {
				t.Errorf("ValidatePathSecurity(%q): got error=%v, want error=%v (err: %v)",
					tt.input, hasError, tt.shouldError, err)
			}
		})
	}
}

func TestFullPathFlow(t *testing.T) {
	// Simulate full flow: validate → normalize → check DICOM
	tests := []struct {
		input       string
		shouldPass  bool
		description string
	}{
		{"test.dcm", true, "Standard .dcm"},
		{"test.FILE", true, ".FILE uppercase"},
		{"test.file", true, ".file lowercase"},
		{"test", true, "No extension"},
		{"patient..backup.dcm", true, "Filename with .."},
		{"../test.dcm", false, "Path traversal"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			fmt.Printf("\n[TEST] %s: %q\n", tt.description, tt.input)

			// Step 1: Security validation
			err := ValidatePathSecurity(tt.input)
			if err != nil {
				if tt.shouldPass {
					t.Errorf("  ✗ ValidatePathSecurity FAILED: %v", err)
				} else {
					fmt.Printf("  ✓ ValidatePathSecurity blocked (expected): %v\n", err)
				}
				return
			}
			fmt.Printf("  ✓ ValidatePathSecurity PASSED\n")

			// Step 2: Normalization
			normalized := NormalizeAndCleanPath(tt.input)
			if normalized == "" {
				t.Errorf("  ✗ NormalizeAndCleanPath returned empty string")
				return
			}
			fmt.Printf("  ✓ NormalizeAndCleanPath: %q\n", normalized)

			// Step 3: DICOM check
			isDicom := IsDicomFile(normalized)
			if !isDicom {
				if tt.shouldPass {
					t.Errorf("  ✗ IsDicomFile FAILED (returned false)")
				} else {
					fmt.Printf("  ✓ IsDicomFile blocked (expected)\n")
				}
				return
			}
			fmt.Printf("  ✓ IsDicomFile PASSED\n")

			if !tt.shouldPass {
				t.Errorf("  ✗ Should have been blocked but passed all checks")
			}
		})
	}
}
