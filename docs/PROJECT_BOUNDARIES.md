# WebMCP Platform Project Boundaries

Bu platform ikkita mustaqil production loyiha sifatida yuritiladi:

| Loyiha | Path | Asosiy vazifa |
|---|---|---|
| Frontend | `C:\Users\user\StudioProjects\webmcp-frontend` | React UI, SEO, UX va WebMCP Native Layer |
| Backend | `C:\Users\user\StudioProjects\webmcp-backend` | Go REST API, orchestration, AI, scenarios, approvals va PostgreSQL state |

## Contract authority

Backend business state, authorization, approval va execution uchun yakuniy authority. Frontend registry, cache yoki UI state server qarorini almashtirmaydi.

Frontend-backend aloqa HTTPS, versioned REST API, typed schema, correlation ID va idempotency semantics orqali qilinadi. LLM provider secretlari faqat backend runtime’da bo‘ladi.

## Current phase

Architecture-first. Product flow, roles, auth, WebMCP execution modeli, provider fallback, persistence va deployment qarorlari tasdiqlanmaguncha feature implementation qilinmaydi.

## Next collaboration step

Foydalanuvchi product arxitekturasini tushuntiradi. Shundan keyin contractlar, threat model, state machine va implementation milestone’lari yangilanadi; undan keyingina kod yozish boshlanadi.
