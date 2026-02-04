package middleware

import (
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

type ValidationMiddleware struct {
	maxBodySize int64
}

func NewValidationMiddleware() *ValidationMiddleware {
	return &ValidationMiddleware{
		maxBodySize: 10 * 1024 * 1024,
	}
}

func (v *ValidationMiddleware) WithMaxBodySize(size int64) *ValidationMiddleware {
	v.maxBodySize = size
	return v
}

func (v *ValidationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := v.validateBodySize(r); err != nil {
			log.Printf("[Validation] Body size exceeded: %s %s", r.Method, r.URL.Path)
			http.Error(w, `{"error": "Request body too large. Maximum size: 10MB"}`, http.StatusRequestEntityTooLarge)
			return
		}

		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			if err := v.validateContentType(r); err != nil {
				log.Printf("[Validation] Invalid content-type: %s %s", r.Method, r.URL.Path)
				http.Error(w, `{"error": "Invalid Content-Type. Expected application/json"}`, http.StatusUnsupportedMediaType)
				return
			}
		}

		if err := v.validatePath(r.URL.Path); err != nil {
			log.Printf("[Validation] Path traversal attempt blocked: %s", r.URL.Path)
			http.Error(w, `{"error": "Invalid request path"}`, http.StatusBadRequest)
			return
		}

		v.sanitizeQueryParams(r)

		next.ServeHTTP(w, r)
	})
}

func (v *ValidationMiddleware) validateBodySize(r *http.Request) error {
	if r.ContentLength > v.maxBodySize {
		return http.ErrContentLength
	}

	r.Body = http.MaxBytesReader(nil, r.Body, v.maxBodySize)
	return nil
}

func (v *ValidationMiddleware) validateContentType(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		return nil
	}

	if strings.HasPrefix(r.URL.Path, "/api/") {
		if !strings.Contains(contentType, "application/json") {
			return http.ErrNotSupported
		}
	}

	return nil
}

func (v *ValidationMiddleware) validatePath(path string) error {
	pathTraversalPatterns := []string{
		"..",
		"./",
		"\\",
		"%2e%2e",
		"%2f",
		"%5c",
	}

	lowerPath := strings.ToLower(path)
	for _, pattern := range pathTraversalPatterns {
		if strings.Contains(lowerPath, pattern) {
			return http.ErrAbortHandler
		}
	}

	return nil
}

func (v *ValidationMiddleware) sanitizeQueryParams(r *http.Request) {
	query := r.URL.Query()
	sanitized := false

	for _, values := range query {
		for i, value := range values {
			escaped := html.EscapeString(value)
			if escaped != value {
				values[i] = escaped
				sanitized = true
			}
		}
	}

	if sanitized {
		r.URL.RawQuery = query.Encode()
	}
}

func SQLInjectionCheck(input string) bool {
	sqlPatterns := []string{
		`(?i)\b(union|select|insert|update|delete|drop|create|alter|exec|execute|script|javascript|eval)\b`,
		`(?i)(\-\-|\;|\/\*|\*\/|xp_|sp_)`,
		`(?i)(\'|\"|\\x27|\\x22)`,
	}

	for _, pattern := range sqlPatterns {
		matched, _ := regexp.MatchString(pattern, input)
		if matched {
			log.Printf("[Validation] Potential SQL injection detected: %s", input)
			return true
		}
	}

	return false
}

func XSSCheck(input string) bool {
	xssPatterns := []string{
		`(?i)<script`,
		`(?i)javascript:`,
		`(?i)onerror\s*=`,
		`(?i)onload\s*=`,
		`(?i)<iframe`,
		`(?i)<object`,
		`(?i)<embed`,
	}

	for _, pattern := range xssPatterns {
		matched, _ := regexp.MatchString(pattern, input)
		if matched {
			log.Printf("[Validation] Potential XSS detected: %s", input)
			return true
		}
	}

	return false
}

func SanitizeInput(input string) string {
	sanitized := strings.TrimSpace(input)

	sanitized = strings.ReplaceAll(sanitized, "\x00", "")

	sanitized = html.EscapeString(sanitized)

	const maxLength = 10000
	if len(sanitized) > maxLength {
		sanitized = sanitized[:maxLength]
	}

	return sanitized
}

func ValidateJSONBody(r *http.Request, maxSize int64) error {
	if r.Body == nil {
		return http.ErrMissingContentLength
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
	if err != nil {
		return err
	}
	defer r.Body.Close()

	bodyStr := string(body)
	if SQLInjectionCheck(bodyStr) || XSSCheck(bodyStr) {
		log.Printf("[Validation] Malicious payload detected in request body")
		return http.ErrAbortHandler
	}

	r.Body = io.NopCloser(strings.NewReader(bodyStr))
	r.ContentLength = int64(len(body))

	return nil
}
