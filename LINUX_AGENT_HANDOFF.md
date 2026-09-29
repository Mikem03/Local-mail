# Local Mail AI Pipeline — Linux Agent Handoff

## Goal

Build a privacy-first email intelligence pipeline. Gmail ingestion and workflow execution run on a Raspberry Pi 5 with 8 GB RAM. A separate Ubuntu/WSL machine runs local AI inference through Ollama.

Project root on the user's Windows machine


The planned directories exist, but implementation files have not yet been created.

## Architecture decisions

- **Raspberry Pi:** PostgreSQL 17 Alpine in Docker for shared email application data; a Go Gmail ingestion worker; a Go API serving pending messages to the Ubuntu processor over Tailscale.
- **Temporal:** Keep the existing Temporal development server and its SQLite-backed state for the prototype. Adding PostgreSQL to Compose does not make Temporal's `start-dev` server use PostgreSQL. Moving Temporal itself to PostgreSQL later requires a separately configured Temporal server deployment.
- **Ubuntu/WSL:** Python client retrieves pending email records and processes them with local Ollama, targeting `llama3.2` (3B).
- **Pi memory:** Raspberry Pi 5 has 8 GB RAM. This should be comfortable for the Pi-side services. The proposed PostgreSQL 100 MB container cap is a target to validate, not a guaranteed safe limit.

## PostgreSQL plan

Use a PostgreSQL 17 Alpine container with the following low-memory configuration as a starting point:

```ini
max_connections = 10
shared_buffers = 16MB
work_mem = 2MB
maintenance_work_mem = 4MB
effective_cache_size = 32MB
wal_buffers = 512kB
min_wal_size = 32MB
max_wal_size = 128MB
```

The expected workload is about five active application connections.

Implementation notes:

- Store PostgreSQL data in a dedicated directory such as `database/postgres/`. Do not mount the entire `database/` directory because it also contains Temporal state.
- The proposed hard limit of `100M` may cause PostgreSQL to be killed during startup or query memory spikes. Measure actual usage before treating it as a reliable limit. `work_mem` applies per query operation and can be used multiple times within a query.
- Replace `POSTGRES_PASSWORD=changeme` with a local secret or environment variable. Never commit real credentials.
- Restrict port 5432 to authorized Tailscale devices with firewall/ACL rules.
- PostgreSQL stores application email data in the prototype. Temporal's dev-server SQLite state remains separate.

## Planned project layout

```text
Local-mail/
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── worker/
│       └── main.go
├── config/
│   ├── credentials.json        # Local only; do not commit
│   └── token.json              # Local only; do not commit
├── database/
│   ├── postgres/               # PostgreSQL runtime data; do not commit
│   └── temporal_state.db       # Temporal dev state; do not commit
├── docker/
│   ├── compose.yaml
│   └── postgresql.conf
├── internal/
│   ├── auth/
│   │   └── gmail.go
│   ├── config/
│   │   └── config.go
│   ├── ingest/
│   │   ├── activities.go
│   │   └── workflows.go
│   ├── server/
│   │   └── handlers.go
│   └── store/
│       └── postgres.go
├── migrations/
│   └── 001_create_email_tables.sql
├── python/
│   ├── ask_ai.py
│   └── requirements.txt
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── LINUX_AGENT_HANDOFF.md
```

The layout above is planned. The Markdown handoff and empty directories exist; implementation files remain to be built. Add OAuth credentials, tokens, `.env`, database files, and PostgreSQL runtime data to `.gitignore`.

## Ingestion behavior to define and implement

- Use `golang.org/x/oauth2` and `google.golang.org/api/gmail/v1`.
- Implement a supported installed-app authorization-code flow. Do not assume a legacy copy/paste OAuth flow. Persist and protect refreshable tokens.
- Define the Gmail query precisely. The current draft proposes unread messages matching a LinkedIn sender **or** a job-related subject. Clarify the exact sender/query rules and whether mail marked read before ingestion should still be collected.
- Fetch message details and parse MIME content safely. Handle `text/plain` and HTML alternatives, character encodings, quoted replies, signatures, and size limits.
- Sanitize HTML, scripts, styles, and tracking content without removing useful job details.
- Use Gmail message IDs as idempotency keys and persist them in PostgreSQL.
- Temporal task queue: `email-ingestion-queue`; run ingestion every 10 minutes. Schedule creation must be idempotent across restarts.
- Keep Temporal activities retry-safe.

## Serving and processing behavior

- The draft endpoint is `GET /sync-wsl`; use a reliable acknowledgment protocol.
- Do not permanently mark a record processed just because the API returned it. If the Python script or Ollama fails, the record must remain retriable.
- Prefer a claim/lease plus separate success acknowledgment, with failure/retry state, or an equivalent reliable pattern.
- Restrict API/database access to authorized Tailscale devices. Do not expose an unauthenticated endpoint to the wider LAN.
- Decide whether AI results are only printed or also saved to PostgreSQL.
- Define a structured result format for job title, company, deadlines, and unknown values. Treat email content as untrusted data, not as instructions to the model.

## Python and Ollama

- Use `requests` and the Ollama Python client.
- Target model: `llama3.2` (3B); the listed model download is about 2.0 GB. Keep context modest (for example 4K–8K) for the user's NVIDIA GTX 970 with 4 GB VRAM.
- Process messages in small batches and handle malformed model output and inference failures.
- Acknowledge records only after successful processing and any required result persistence.

## Immediate agent task

Inspect the repository contents first, then implement the prototype in the planned layout. Confirm the project root before writing files. Preserve OAuth secrets and runtime database files through `.gitignore`. Keep Temporal dev persistence in SQLite for now and use PostgreSQL for email application data.
