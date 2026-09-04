# M0 Foundation

M0 is the executable safety and operational foundation before domain features.

## Included

- environment configuration with production validation;
- bounded request body size;
- request correlation via `X-Request-ID`;
- JSON error envelope with machine-readable codes;
- health and readiness probes;
- strict exact-origin CORS;
- baseline security headers;
- panic recovery with safe client response;
- structured JSON access logging without request payloads;
- bounded HTTP timeouts and graceful shutdown;
- non-root distroless container image;
- CI formatting, test, vet and build gates.

## Explicit boundary

M0 does not claim database, authentication, authorization, AI provider, WebMCP or domain readiness. The readiness endpoint currently represents process readiness only; dependency checks will be registered when PostgreSQL and provider adapters are introduced.

## Local run

```powershell
$env:WEBMCP_ENV = "development"
$env:WEBMCP_HTTP_ADDR = "127.0.0.1:8080"
go run ./cmd/server
```

Probe `GET /healthz` for liveness and `GET /readyz` for process readiness.
