# WebMCP Backend

Go asosidagi alohida production backend loyihasi.

## Hozirgi holat

Backend foundation, tenant-scoped PostgreSQL workflow, deterministic scenario/proposal/approval flow, hardened REST boundaries, provider-neutral OIDC JWT/JWKS validation, bounded guest judging and WebMCP-facing contracts are implemented. The current judge MVP deliberately keeps business calculations deterministic; an autonomous LLM router/provider is a documented extension boundary, not a runtime dependency. Live PostgreSQL/RLS concurrency verification and deployment smoke tests remain environment-level release checks.

## Vazifa

Backend REST API, typed WebMCP use-case boundary, scenario execution, approval policy va PostgreSQL business state uchun yagona server authority bo‘ladi. Browser agenti capability surface’ni WebMCP orqali boshqaradi; backend agentdan kelgan barcha qiymatlarni qayta tekshiradi.

## Chegaralar

- Client faqat HTTPS API orqali ulanadi.
- LLM provider secretlari faqat backend’da saqlanadi.
- Business state PostgreSQL’da persist qilinadi.
- Tool, scenario va approval qarorlari audit qilinadigan workflow sifatida ko‘riladi.

Arxitektura: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

Neon integration and the migration runner: [docs/M14_NEON_MANAGED_POSTGRES.md](docs/M14_NEON_MANAGED_POSTGRES.md)
