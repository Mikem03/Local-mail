package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mikem03/Local-mail/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Postgres struct {
	db *sql.DB
}

var _ Store = (*Postgres)(nil)

func Open(cfg config.Config) (*Postgres, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	return &Postgres{db: db}, nil
}

func (p *Postgres) Close() error { return p.db.Close() }

func (p *Postgres) Ping(ctx context.Context) error {
	if err := p.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}

// ApplyMigrations applies ordered .sql files and records successful versions.
// Each file runs in its own transaction, so a failed migration can be retried.
func (p *Postgres) ApplyMigrations(ctx context.Context, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read migrations directory %q: %w", directory, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		migration, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := p.applyMigration(ctx, name, string(migration)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

func (p *Postgres) applyMigration(ctx context.Context, version, migration string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Serialize migration runners, including first-run table creation.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(72419021)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return err
	}

	var applied bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM schema_migrations WHERE version = $1
	)`, version).Scan(&applied); err != nil {
		return err
	}
	if applied {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, migration); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) InsertEmail(ctx context.Context, email Email) (bool, error) {
	result, err := p.db.ExecContext(ctx, `
		INSERT INTO emails (gmail_message_id, sender, subject, body_text, received_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (gmail_message_id) DO NOTHING`,
		email.GmailMessageID, email.Sender, email.Subject, email.BodyText, email.ReceivedAt)
	if err != nil {
		return false, fmt.Errorf("insert email %q: %w", email.GmailMessageID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check inserted email result: %w", err)
	}
	return count == 1, nil
}

func (p *Postgres) ClaimEmails(ctx context.Context, limit int, lease time.Duration) ([]Email, error) {
	if limit < 1 {
		return nil, fmt.Errorf("claim limit must be positive")
	}
	if lease.Milliseconds() < 1 {
		return nil, fmt.Errorf("lease duration must be positive")
	}

	rows, err := p.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM emails
			WHERE status = 'pending'
			   OR (status = 'processing' AND lease_expires_at <= NOW())
			ORDER BY received_at, id
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE emails AS e
		SET status = 'processing',
		    claimed_at = NOW(),
		    lease_expires_at = NOW() + ($2 * INTERVAL '1 millisecond'),
		    attempt_count = e.attempt_count + 1,
		    updated_at = NOW()
		FROM candidates
		WHERE e.id = candidates.id
		RETURNING e.id, e.gmail_message_id, e.sender, e.subject, e.body_text,
		          e.received_at, e.status, e.claimed_at, e.lease_expires_at,
		          e.attempt_count, e.last_error`,
		limit, lease.Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("claim pending emails: %w", err)
	}
	defer rows.Close()

	emails := make([]Email, 0)
	for rows.Next() {
		var email Email
		var subject, bodyText, lastError sql.NullString
		var claimedAt, leaseExpiresAt sql.NullTime
		if err := rows.Scan(&email.ID, &email.GmailMessageID, &email.Sender,
			&subject, &bodyText, &email.ReceivedAt, &email.Status,
			&claimedAt, &leaseExpiresAt, &email.AttemptCount, &lastError); err != nil {
			return nil, fmt.Errorf("scan claimed email: %w", err)
		}
		email.Subject, email.BodyText = subject.String, bodyText.String
		email.LastError = lastError.String
		if claimedAt.Valid {
			email.ClaimedAt = &claimedAt.Time
		}
		if leaseExpiresAt.Valid {
			email.LeaseExpiresAt = &leaseExpiresAt.Time
		}
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed emails: %w", err)
	}
	return emails, nil
}

func (p *Postgres) CompleteEmail(ctx context.Context, id int64, claimedAt time.Time, extraction Extraction) error {
	if len(extraction.RawResponse) > 0 && !json.Valid(extraction.RawResponse) {
		return fmt.Errorf("raw extraction response must be valid JSON")
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin email completion: %w", err)
	}
	defer tx.Rollback()

	var lockedID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM emails
		WHERE id = $1 AND status = 'processing' AND claimed_at = $2
		FOR UPDATE`, id, claimedAt).Scan(&lockedID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return fmt.Errorf("verify email lease: %w", err)
	}

	var rawResponse any
	if len(extraction.RawResponse) > 0 {
		rawResponse = string(extraction.RawResponse)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO job_extractions
			(email_id, job_title, job_url, company, platform, posted_at, deadline, raw_response)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		ON CONFLICT (email_id) DO UPDATE SET
			job_title = EXCLUDED.job_title,
			job_url = EXCLUDED.job_url,
			company = EXCLUDED.company,
			platform = EXCLUDED.platform,
			posted_at = EXCLUDED.posted_at,
			deadline = EXCLUDED.deadline,
			raw_response = EXCLUDED.raw_response,
			processed_at = NOW()`,
		id, extraction.JobTitle, extraction.JobURL, extraction.Company,
		extraction.Platform, extraction.PostedAt, extraction.Deadline, rawResponse)
	if err != nil {
		return fmt.Errorf("save job extraction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE emails SET status = 'processed', claimed_at = NULL,
			lease_expires_at = NULL, last_error = NULL, updated_at = NOW()
		WHERE id = $1`, id); err != nil {
		return fmt.Errorf("mark email processed: %w", err)
	}
	return tx.Commit()
}

func (p *Postgres) ReleaseEmail(ctx context.Context, id int64, claimedAt time.Time, cause error) error {
	if cause == nil {
		return fmt.Errorf("release cause must not be nil")
	}
	result, err := p.db.ExecContext(ctx, `
		UPDATE emails
		SET status = 'pending', claimed_at = NULL, lease_expires_at = NULL,
		    last_error = LEFT($3, 2000), updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND claimed_at = $2`,
		id, claimedAt, cause.Error())
	if err != nil {
		return fmt.Errorf("release email claim: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check released email result: %w", err)
	}
	if count == 0 {
		return ErrLeaseLost
	}
	return nil
}
