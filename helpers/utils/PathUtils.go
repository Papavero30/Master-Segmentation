package utils

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxPathLength      = 500
	MaxComponentLength = 255
)

type PathValidationError struct {
	Path    string
	Reason  string
	Details string
}

func (e *PathValidationError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("invalid path '%s': %s (%s)", e.Path, e.Reason, e.Details)
	}
	return fmt.Sprintf("invalid path '%s': %s", e.Path, e.Reason)
}

func ValidatePathSecurity(rawPath string) error {
	trimmed := strings.TrimSpace(rawPath)

	if trimmed == "" {
		return &PathValidationError{
			Path:   rawPath,
			Reason: "empty path not allowed",
		}
	}

	if len(trimmed) > MaxPathLength {
		return &PathValidationError{
			Path:    rawPath,
			Reason:  "path too long",
			Details: fmt.Sprintf("maximum %d characters, got %d", MaxPathLength, len(trimmed)),
		}
	}

	decoded := trimmed
	for i := 0; i < 3; i++ {
		if unescaped, err := url.QueryUnescape(decoded); err == nil && unescaped != decoded {
			decoded = unescaped
		} else {
			break
		}
	}

	if strings.Contains(decoded, "\x00") {
		return &PathValidationError{
			Path:   rawPath,
			Reason: "null byte not allowed",
		}
	}

	normalized := strings.ReplaceAll(decoded, "\\", "/")

	if len(normalized) >= 2 && normalized[1] == ':' {
		return &PathValidationError{
			Path:    rawPath,
			Reason:  "absolute paths not allowed",
			Details: "Windows drive letter detected",
		}
	}

	if strings.HasPrefix(normalized, "/") {
		return &PathValidationError{
			Path:   rawPath,
			Reason: "absolute paths not allowed",
		}
	}

	parts := strings.Split(normalized, "/")

	for i, part := range parts {
		isLastPart := (i == len(parts)-1)

		if part == ".." {
			return &PathValidationError{
				Path:    rawPath,
				Reason:  "path traversal not allowed",
				Details: "'..' component detected",
			}
		}

		if !isLastPart && strings.Contains(part, "..") {
			return &PathValidationError{
				Path:    rawPath,
				Reason:  "path traversal not allowed",
				Details: fmt.Sprintf("'..' detected in directory component '%s'", part),
			}
		}

		if len(part) > MaxComponentLength {
			return &PathValidationError{
				Path:    rawPath,
				Reason:  "path component too long",
				Details: fmt.Sprintf("component '%s' exceeds %d characters", part, MaxComponentLength),
			}
		}
	}

	cleaned := path.Clean(normalized)
	if strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return &PathValidationError{
			Path:    rawPath,
			Reason:  "path traversal detected after normalization",
			Details: fmt.Sprintf("normalized to: %s", cleaned),
		}
	}

	return nil
}

func NormalizeAndCleanPath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")

	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}

	normalized = strings.TrimPrefix(normalized, "/")

	normalized = strings.ReplaceAll(normalized, "//", "/")

	cleaned := path.Clean(normalized)

	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimPrefix(cleaned, "./")

	if cleaned == "." {
		return ""
	}

	cleaned = strings.ReplaceAll(cleaned, "//", "/")
	cleaned = strings.Trim(cleaned, "/")

	return cleaned
}

var dicomExtensions = map[string]struct{}{
	".dcm":   {},
	".dic":   {},
	".dicm":  {},
	".dicom": {},
	".dc3":   {},
	".ima":   {},
	".file":  {},
}

func IsDicomFile(filename string) bool {
	trimmed := strings.TrimSpace(filename)
	if trimmed == "" {
		return false
	}

	base := filepath.Base(trimmed)
	ext := strings.ToLower(filepath.Ext(base))

	if ext == "" {
		return true
	}

	if _, ok := dicomExtensions[ext]; ok {
		return true
	}

	nonDicomExtensions := map[string]struct{}{
		".txt":  {},
		".pdf":  {},
		".doc":  {},
		".docx": {},
		".xls":  {},
		".xlsx": {},
		".zip":  {},
		".rar":  {},
		".exe":  {},
		".dll":  {},
		".so":   {},
		".sh":   {},
		".bat":  {},
		".ps1":  {},
		".jpg":  {},
		".jpeg": {},
		".png":  {},
		".gif":  {},
		".bmp":  {},
		".mp4":  {},
		".avi":  {},
		".mp3":  {},
		".wav":  {},
	}

	if _, ok := nonDicomExtensions[ext]; ok {
		return false
	}

	return true
}

func ValidateAndNormalizePath(rawPath string) (string, error) {
	if err := ValidatePathSecurity(rawPath); err != nil {
		return "", err
	}

	cleaned := NormalizeAndCleanPath(rawPath)
	if cleaned == "" {
		return "", &PathValidationError{
			Path:   rawPath,
			Reason: "path normalization resulted in empty string",
		}
	}

	if !IsDicomFile(cleaned) {
		return "", &PathValidationError{
			Path:    rawPath,
			Reason:  "not a DICOM file",
			Details: fmt.Sprintf("file extension not recognized as DICOM: %s", filepath.Ext(cleaned)),
		}
	}

	return cleaned, nil
}

func SanitizeManifestPath(sid, relPath string) (string, error) {
	return ValidateAndNormalizePath(relPath)
}
