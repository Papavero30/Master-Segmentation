package utils

import (
	"encoding/json"
	"time"
)


type AuditEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"`
	UserID    string                 `json:"user_id,omitempty"`
	DeviceID  string                 `json:"device_id,omitempty"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Resource  string                 `json:"resource,omitempty"`
	Action    string                 `json:"action"`
	Result    string                 `json:"result"`
	Message   string                 `json:"message,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Severity  string                 `json:"severity"`
}


type AuditLogger struct {
	logger *Logger
}


func NewAuditLogger(logger *Logger) *AuditLogger {
	return &AuditLogger{logger: logger}
}


func (al *AuditLogger) LogEvent(event AuditEvent) {
	event.Timestamp = time.Now()


	eventJSON, err := json.Marshal(event)
	if err != nil {
		al.logger.Error("Failed to marshal audit event: %v", err)
		return
	}


	switch event.Severity {
	case "critical":
		al.logger.Error("[AUDIT] %s", string(eventJSON))
	case "high":
		al.logger.Warning("[AUDIT] %s", string(eventJSON))
	case "medium":
		al.logger.Info("[AUDIT] %s", string(eventJSON))
	default:
		al.logger.Debug("[AUDIT] %s", string(eventJSON))
	}
}


func (al *AuditLogger) LogLogin(deviceID, ipAddress, userAgent, result string) {
	al.LogEvent(AuditEvent{
		EventType: "authentication",
		DeviceID:  deviceID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Action:    "login",
		Result:    result,
		Severity:  "medium",
	})
}

func (al *AuditLogger) LogLogout(deviceID, ipAddress string) {
	al.LogEvent(AuditEvent{
		EventType: "authentication",
		DeviceID:  deviceID,
		IPAddress: ipAddress,
		Action:    "logout",
		Result:    "success",
		Severity:  "low",
	})
}

func (al *AuditLogger) LogFailedAuth(ipAddress, userAgent, reason string) {
	al.LogEvent(AuditEvent{
		EventType: "security",
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Action:    "authentication_failed",
		Result:    "blocked",
		Message:   reason,
		Severity:  "high",
	})
}


func (al *AuditLogger) LogDataAccess(deviceID, ipAddress, resource, action, result string) {
	al.LogEvent(AuditEvent{
		EventType: "data_access",
		DeviceID:  deviceID,
		IPAddress: ipAddress,
		Resource:  resource,
		Action:    action,
		Result:    result,
		Severity:  "medium",
	})
}


func (al *AuditLogger) LogSecurityEvent(eventType, ipAddress, action, message string, severity string) {
	al.LogEvent(AuditEvent{
		EventType: eventType,
		IPAddress: ipAddress,
		Action:    action,
		Result:    "detected",
		Message:   message,
		Severity:  severity,
	})
}


func (al *AuditLogger) LogRateLimit(ipAddress, userAgent string) {
	al.LogEvent(AuditEvent{
		EventType: "security",
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Action:    "rate_limit_exceeded",
		Result:    "blocked",
		Severity:  "medium",
	})
}
