package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrLeaseLost = errors.New("email lease is no longer owned by this processor")

// Email is an email record persisted by the ingestion worker.
type Email struct {
	ID             int64
	GmailMessageID string
	Sender         string
	Subject        string
	BodyText       string
	ReceivedAt     time.Time
	Status         string
	ClaimedAt      *time.Time
	LeaseExpiresAt *time.Time
	AttemptCount   int
	LastError      string
}

// Extraction is the structured result produced for a job-related email.
type Extraction struct {
	JobTitle    *string
	JobURL      *string
	Company     *string
	Platform    *string
	PostedAt    *time.Time
	Deadline    *time.Time
	RawResponse json.RawMessage
}

// Store contains the persistence operations used by ingestion and processing.
type Store interface {
	Close() error
	Ping(context.Context) error
	ApplyMigrations(context.Context, string) error
	InsertEmail(context.Context, Email) (bool, error)
	ClaimEmails(context.Context, int, time.Duration) ([]Email, error)
	CompleteEmail(context.Context, int64, time.Time, Extraction) error
	ReleaseEmail(context.Context, int64, time.Time, error) error
}
