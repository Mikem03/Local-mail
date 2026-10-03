CREATE TABLE IF NOT EXISTS emails (
	id SERIAL PRIMARY KEY,
	gmail_message_id VARCHAR(255) UNIQUE NOT NULL,
	sender VARCHAR(255) NOT NULL,
	subject TEXT,
	body_text TEXT,
	received_at TIMESTAMPTZ NOT NULL,
	status VARCHAR(50) NOT NULL DEFAULT 'pending', --pending, processing, processed
	claimed_at TIMESTAMPTZ,
	lease_expires_at TIMESTAMPTZ,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	last_error TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Also upgrade databases where this migration was applied manually before
-- application-managed migrations were introduced.
ALTER TABLE emails ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE emails ADD COLUMN IF NOT EXISTS last_error TEXT;

CREATE INDEX IF NOT EXISTS idx_emails_status_lease ON emails(status, lease_expires_at);

CREATE TABLE IF NOT EXISTS job_extractions (
	id SERIAL PRIMARY KEY, 
	email_id INT UNIQUE REFERENCES emails(id) ON DELETE CASCADE,
	job_title VARCHAR(255),
	job_url TEXT,
	company VARCHAR(255),
	platform VARCHAR(255),
	posted_at TIMESTAMPTZ,
	deadline TIMESTAMPTZ,
	raw_response JSONB,
	processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
