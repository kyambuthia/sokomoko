# Sokomoko documentation

These documents describe the code as it stands. If the code and a document
disagree, the code is right and the document needs fixing.

The public audience guide is served at `docs.asiliano.online`; this folder is
the deeper implementation and operations reference for contributors.

| # | Document | Read it for |
| --- | --- | --- |
| 1 | [Overview](01-overview.md) | What the product does, the three workspaces, roles |
| 2 | [Architecture](02-architecture.md) | Package layers, request pipeline, composition, process lifecycle |
| 3 | [HTTP surface](03-http-surface.md) | Every HTML route per host and the `/api/v1` JSON API |
| 4 | [Security](04-security.md) | Sessions, CSRF, roles, first-run setup, password reset, throttling, headers |
| 5 | [Data model](05-data-model.md) | Schema, migrations, money, inventory, checkout, order state machine |
| 6 | [Frontend](06-frontend.md) | Templates, layouts, assets, scripts |
| 7 | [Configuration](07-configuration.md) | Every environment variable and its validation rules |
| 8 | [Development](08-development.md) | Local setup, tests, CI, coding conventions |
| 9 | [Deployment](09-deployment.md) | Container image, production Compose, release, operations, recovery |
| 10 | [Known limitations](10-known-limitations.md) | Gaps and rough edges visible in the current code |
| – | [Payments](payments.md) | Providers, payment flow, unpaid-order expiry, refunds for late or duplicate charges |
| – | [Messaging](messaging.md) | Handles, requests, buyer/seller/courier conversations, blocking, reports, and access rules |
| – | [Roadmap](roadmap.md) | Planned and completed improvement work |
| – | [Design](design.md) | Visual rules, tokens, components, and the web component library |

- [End-to-end encrypted messaging](encrypted-messaging.md) — device trust, MLS, browser storage, migration and verification.

- [Services and local discovery proposal](services-and-local-discovery.md) — service listings, availability, bookings, map privacy and delivery sequence.
