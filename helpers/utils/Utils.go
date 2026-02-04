package utils

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
)

func GenerateULID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var validFilenameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func IsValidFilename(name string) bool {
	if name == "" {
		return false
	}
	if name == "." || name == ".." || filepath.Base(name) != name {
		return false
	}
	return validFilenameRe.MatchString(name)
}

func FileExists(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err == nil {
		return !info.IsDir()
	}
	return false
}
