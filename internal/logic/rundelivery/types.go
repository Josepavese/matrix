package rundelivery

import "time"

const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusDead    = "dead"
)

type Delivery struct {
	ID            string    `json:"id"`
	SinkID        string    `json:"sink_id"`
	RunID         string    `json:"run_id"`
	EventID       string    `json:"event_id"`
	EventKind     string    `json:"event_kind"`
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"last_error,omitempty"`
	NextAttemptAt time.Time `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
