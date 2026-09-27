package audit

import (
	"encoding/json"
	"time"
)

type Response struct {
	ID        string          `json:"id"`
	UserID    *string         `json:"user_id,omitempty"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  *string         `json:"entity_id,omitempty"`
	OldData   json.RawMessage `json:"old_data,omitempty"`
	NewData   json.RawMessage `json:"new_data,omitempty"`
	IPAddress string          `json:"ip_address,omitempty"`
	UserAgent string          `json:"user_agent,omitempty"`
	CreatedAt string          `json:"created_at"`
}

func toResponse(a *AuditLog) Response {
	return Response{
		ID:        a.ID,
		UserID:    a.UserID,
		Action:    a.Action,
		Entity:    a.Entity,
		EntityID:  a.EntityID,
		OldData:   a.OldData,
		NewData:   a.NewData,
		IPAddress: a.IPAddress,
		UserAgent: a.UserAgent,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
}
