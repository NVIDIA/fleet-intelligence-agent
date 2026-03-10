// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package v1

type NotificationType string

const (
	NotificationTypeShutdown NotificationType = "shutdown"
	NotificationTypeStartup  NotificationType = "startup"
)

type NotificationRequest struct {
	ID   string           `json:"id"`
	Type NotificationType `json:"type"`
}

type NotificationResponse struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}
