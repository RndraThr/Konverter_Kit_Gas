package audit

import (
	"errors"
	"time"
)

var ErrInvalidEvent = errors.New("invalid audit event")

type Event struct {
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]any
	IPAddress    string
	UserAgent    string
}

type Entry struct {
	ID           string         `json:"id"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	ActorName    string         `json:"actor_name,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type Filter struct {
	Page         int
	PageSize     int
	Action       string
	ResourceType string
	ActorUserID  string
}

type Page struct {
	Items    []Entry `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Total    int64   `json:"total"`
}
