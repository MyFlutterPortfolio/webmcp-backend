# M14 — Neon Managed PostgreSQL Integration

## Operating decision

The Go API uses Neon’s direct read-write connection for the current deployment. The application already owns a bounded `pgxpool` and uses transaction-local RLS context plus session timeouts. Neon pooled endpoints use PgBouncer and must not be used for migrations; pooled runtime support requires a separate timeout/startup-parameter compatibility review.

## Migration workflow

1. Create or select the target Neon branch.
2. Keep the direct connection string only in the deployment secret store or the local process environment. Never put it in Vite variables, source control or chat.
3. Run `go run ./cmd/migrate` from the backend root with `WEBMCP_DATABASE_URL` set. The runner discovers only `NNNNNN_name.up.sql` files, applies them in order, takes a PostgreSQL advisory lock and records successful versions in `schema_migrations`.
4. Verify the schema and `relforcerowsecurity` before deploying the API.
5. Deploy the API with the same branch URL and run `/healthz` and `/readyz` checks.

The migration command is intentionally separate from the server process. A failed migration stops before deployment, while a previously applied migration is safely skipped only when its version/name matches the local file.

## Branch discipline

Use a staging child branch for migration validation and reserve the protected production branch for reviewed changes. Do not use the extension’s localhost proxy URL as a production `WEBMCP_DATABASE_URL`.
