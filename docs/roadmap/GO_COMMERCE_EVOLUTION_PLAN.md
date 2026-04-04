# Go Commerce Evolution Plan

Last updated: April 4, 2026

## Purpose

This document defines the next-stage commerce architecture for Sokomoko using Go, server-rendered HTML, SQLite today, and explicit service boundaries.

The objective is to grow the platform from a solid small-store application into a system with stronger commercial correctness:

- explicit domain boundaries
- durable checkout and payment state
- reservation-based inventory
- policy-driven pricing and promotions
- capability-based permissions
- reliable integration and event delivery

This plan preserves current delivery choices:

- Go remains the implementation language
- server-rendered HTML remains the primary UI
- one deployable binary remains acceptable for now
- one relational database remains acceptable for now

The change is in the commerce model, not in the presentation stack.

## Current Limits

Today the platform has a good base, but the commerce core is still narrow:

- one global catalog
- one mutable stock quantity on each product
- one checkout path that creates orders directly
- placeholder payment methods instead of durable payment state
- no channels, warehouses, reservations, or allocations
- no promotion engine or voucher model
- coarse `user|staff|admin` authorization
- no outbox-backed event delivery

The result is that new features will otherwise keep accumulating inside broad services and route handlers.

## Architecture Direction

The target architecture should make these decisions explicit:

1. Catalog, pricing, inventory, checkout, orders, payments, shipping, promotions, permissions, and integrations are separate domains.
2. Checkout is a stateful object, not just a POST over cart data.
3. Payment records are durable and provider-aware.
4. Inventory availability is derived from stock, reservations, and allocations, not a single mutable counter.
5. Business side effects are emitted through a durable outbox and dispatcher, not direct inline calls.
6. Admin and partner surfaces orchestrate shared domains rather than owning separate business rules.

## Target Domain Model

The next architecture should introduce these domain areas as explicit Go packages and schema concepts.

### 1. Catalog

Responsibilities:

- products
- categories
- media
- product status and publication state
- attributes and structured metadata
- channel-specific visibility

Go package target:

- `internal/service/catalog`

Technical direction:

- catalog should own product identity and publication state
- structured metadata should allow future filtering, merchandising, and reporting without scattering ad hoc columns
- channel visibility should determine whether an item is published, purchasable, and searchable in a given commercial surface

### 2. Pricing

Responsibilities:

- base prices
- channel prices
- currencies
- tax-inclusive or tax-exclusive display strategy
- compare-at prices
- discount inputs
- price resolution for storefront and checkout

Go package target:

- `internal/service/pricing`

Technical direction:

- pricing should stop living as constants and arithmetic inside checkout code
- the pricing service should accept a pricing context: item reference, channel, currency, quantity, customer class if needed, and active adjustment context
- the result should include base amount, applied adjustments, tax inputs, and resolved final amount
- checkout lines should persist a pricing snapshot so orders remain auditable when product prices later change
- order creation should never depend on recalculating a historical price from current product data

### 3. Inventory

Responsibilities:

- warehouses
- stock records per sellable item per warehouse
- reservations
- allocations
- releases
- stock adjustments
- inventory availability calculation

Go package target:

- `internal/service/inventory`

Technical direction:

- inventory should move away from `products.stock_quantity` as the source of truth
- stock represents on-hand quantity in a location
- reservations temporarily hold sellable units for a checkout
- allocations represent units committed to an order or fulfillment workflow
- releases return reserved quantity back to sellable inventory after expiration, cancellation, or payment failure
- stock movements should become an append-only ledger for receipts, corrections, reservations, releases, allocations, and returns
- storefront availability should be derived from inventory state, not from a manually edited total

### 4. Checkout

Responsibilities:

- checkout lifecycle
- address capture and validation
- line validation
- pricing snapshots
- shipping and tax quote orchestration
- payment initiation
- completion
- expiration
- idempotency

Go package target:

- `internal/service/checkout`

Technical direction:

- checkout should become its own persisted aggregate with an identifier, timestamps, status, currency, and channel
- checkout lines should carry quantity, item reference, pricing snapshot, and reservation linkage
- completion should run behind an idempotency key so retries cannot create duplicate orders or duplicate provider calls
- expiration should release reservations and invalidate stale checkouts
- validation should return structured errors such as invalid address, expired reservation, price mismatch, unavailable stock, unsupported payment method, or payment still pending
- a cart can remain a customer-facing concept, but checkout should be the transactionally important object

### 5. Orders

Responsibilities:

- order creation from checkout
- order state machine
- fulfillment state
- cancellation rules
- returns
- refunds
- customer order detail and history
- commercial snapshots

Go package target:

- `internal/service/order`

Technical direction:

- order creation should freeze names, addresses, prices, discounts, taxes, and shipping charges
- order state should be modeled as a constrained transition system rather than arbitrary string edits
- cancellation rules should know whether payment is pending, authorized, captured, refunded, or whether fulfillment has started
- returns and refunds should be modeled separately because they affect different parts of the system
- partner and admin order views should be projections over the same order domain, not separate order implementations

### 6. Payments

Responsibilities:

- payment records
- provider attempts
- authorization
- capture
- failure
- cancellation
- refunds
- provider transaction IDs
- webhook or callback processing
- reconciliation support

Go package target:

- `internal/service/payment`

Technical direction:

- payment method strings are not enough; the system needs durable payment state
- `payments` should represent the commercial payment attached to a checkout or order
- `payment_attempts` should record every call made to a provider, with timestamps, request references, provider responses, and external transaction IDs
- payment state should be explicit: `pending`, `authorized`, `partially_captured`, `captured`, `failed`, `cancelled`, `refund_pending`, `partially_refunded`, `refunded`
- webhook processing must be idempotent because providers retry events and reorder deliveries
- provider metadata should be preserved for debugging and operational reconciliation

### 7. Promotions

Responsibilities:

- catalogue discounts
- order discounts
- vouchers
- redemption rules
- free-shipping triggers
- campaign windows
- reward evaluation

Go package target:

- `internal/service/promotion`

Technical direction:

- promotions should separate definition from evaluation
- a promotion definition should describe scope, activation window, stacking rules, eligibility predicates, and reward type
- evaluation should produce explicit adjustments attached to prices or order totals
- vouchers should track code, limits, customer scope, and validity windows
- applied discounts must be persisted as concrete adjustments so the system can explain why a total was reduced
- the first implementation can support fixed and percentage discounts before more advanced rule combinations

### 8. Shipping

Responsibilities:

- shipping zones
- shipping methods
- rate rules
- delivery estimates
- pickup options
- shipment lifecycle

Go package target:

- `internal/service/shipping`

Technical direction:

- shipping zones should map geographies to available methods
- shipping methods should express channel restrictions, subtotal thresholds, weight or size limits, and service levels
- checkout should resolve shipping options through a deterministic quote function
- selected shipping should be snapshotted into the order
- shipment records should track package creation, dispatch, carrier references, and delivery completion
- pickup should be modeled as a shipping method family, not special-case route logic

### 9. Identity and Permissions

Responsibilities:

- staff permissions
- partner permissions
- role groups
- privileged-action policy checks
- optional MFA and verified email
- auditability

Go package targets:

- keep authentication mechanics in `internal/auth`
- add service-layer authorization policy logic in `internal/service/identity` or `internal/service/authz`

Technical direction:

- route guards should remain as a first filter, but service-layer rules must enforce authorization too
- role should remain a coarse identity bucket
- permissions should become capability-based
- sensitive actions such as stock adjustment, refund issuance, user deactivation, product publication, and order override should always emit audit entries
- audit details should be structured enough to support operator review and future export tooling

### 10. Integrations and Events

Responsibilities:

- outbox event persistence
- internal event dispatch
- webhook subscriptions
- webhook delivery retries
- integration adapters for payment, tax, search, analytics, and email

Go package targets:

- `internal/service/event`
- `internal/service/integration`

Technical direction:

- business writes should emit domain events into an outbox table inside the same database transaction
- a dispatcher loop should pull pending events and deliver them to internal handlers with retry and backoff
- external webhooks should be layered on top of the same durable event pipeline
- integrations should consume stable event payloads instead of directly reaching into unrelated tables or route handlers
- delivery attempts should be logged with status, retry count, and last error for operational debugging

## Recommended Package Shape

Sokomoko should move from the current broad services:

- catalog
- account
- commerce
- admin
- partner
- auth

toward a domain layout more like:

```text
internal/service/
├── account/
├── admin/
├── authz/
├── catalog/
├── checkout/
├── event/
├── integration/
├── inventory/
├── order/
├── payment/
├── pricing/
├── promotion/
├── shipping/
└── partner/
```

Notes:

- `admin` and `partner` should become orchestration layers over shared domains
- `commerce` should be retired gradually by splitting it into `pricing`, `checkout`, `payment`, `inventory`, and `order`
- `partner` should consume shared order and inventory logic rather than hosting a permanent shadow fulfillment model

## Schema Evolution

These are the most important new tables to add.

### Phase A: Channels and pricing

Add:

- `channels`
- `channel_currencies`
- `product_channel_listings`
- `product_channel_prices` or `variant_channel_prices`

Technical description:

- `channels` define a commercial surface such as a region, market, brand, or sales program
- `product_channel_listings` control whether an item is visible and purchasable in a channel
- channel price rows should hold published price, compare-at price, currency, and optional scheduling windows
- if variants are introduced soon, attach price to variants immediately rather than creating a second migration path later

### Phase B: Inventory

Add:

- `warehouses`
- `warehouse_channels`
- `stocks`
- `stock_movements`
- `stock_reservations`
- `stock_allocations`

Technical description:

- `warehouses` represent physical or logical fulfillment locations
- `stocks` hold on-hand quantity for a sellable item in a warehouse
- `stock_reservations` hold quantity temporarily for a checkout and expire automatically
- `stock_allocations` bind quantity to an order or fulfillment step
- `stock_movements` form the auditable ledger explaining every quantity change

### Phase C: Checkout and payments

Add:

- `checkouts`
- `checkout_lines`
- `checkout_addresses`
- `payment_methods`
- `payments`
- `payment_attempts`
- `payment_events`
- `idempotency_keys`

Technical description:

- `checkouts` should include status, customer reference, channel, currency, expiry, selected shipping method, selected payment method, and computed totals
- `checkout_lines` should include item reference, quantity, pricing snapshot, discount snapshot, and reservation linkage
- `payments` should reflect the aggregate financial state for a checkout or order
- `payment_attempts` should store each provider interaction separately so operational debugging is possible
- `idempotency_keys` should bind a completion request to one result so client retries are safe

### Phase D: Promotions

Add:

- `promotions`
- `promotion_rules`
- `vouchers`
- `voucher_redemptions`
- `order_discounts`
- `catalog_discounts`

Technical description:

- `promotions` are top-level campaigns
- `promotion_rules` describe eligibility and reward formulas
- `vouchers` carry redeemable codes and redemption policy
- discount adjustment tables preserve the actual commercial effect applied at purchase time

### Phase E: Shipping and permissions

Add:

- `shipping_zones`
- `shipping_methods`
- `shipping_method_channels`
- `permissions`
- `role_permissions`
- `user_permissions` if direct grants become necessary

Technical description:

- shipping tables should support both configuration and operational shipment derivation
- permission tables should normalize capability checks so admin UI, partner UI, background jobs, and future APIs all authorize actions consistently

### Optional future tables

- `gift_cards`
- `returns`
- `refunds`
- `webhook_subscriptions`
- `event_outbox`
- `external_references`

## Interfaces To Introduce In Go

The key architectural step is not only new tables. It is cleaner dependency boundaries.

### Payment provider

```go
type Provider interface {
    Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error)
    Capture(ctx context.Context, req CaptureRequest) (CaptureResult, error)
    Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
    ParseWebhook(ctx context.Context, payload []byte, headers http.Header) (WebhookEvent, error)
}
```

### Tax provider

```go
type Provider interface {
    Quote(ctx context.Context, req QuoteRequest) (QuoteResult, error)
}
```

### Event publisher

```go
type Publisher interface {
    Publish(ctx context.Context, event Event) error
}
```

### Search indexer

```go
type Indexer interface {
    UpsertProduct(ctx context.Context, product ProductDocument) error
    DeleteProduct(ctx context.Context, productID int64) error
}
```

Design rules:

- interfaces should live in the consuming domain package
- provider-specific implementations should live in small adapter packages
- route handlers should not depend on provider payload shapes directly
- application composition should wire concrete implementations at the edge

## Event Model

Core domain changes should emit durable business events.

Minimum event set:

- `catalog.product.created`
- `catalog.product.updated`
- `inventory.stock.adjusted`
- `inventory.stock.reserved`
- `inventory.stock.released`
- `checkout.created`
- `checkout.completed`
- `order.created`
- `order.cancelled`
- `payment.authorized`
- `payment.captured`
- `payment.failed`
- `payment.refunded`
- `fulfillment.created`
- `fulfillment.dispatched`
- `customer.created`

Implementation recommendation:

1. Store events in `event_outbox` inside the same transaction as the business write.
2. Run a background dispatcher in Go.
3. Deliver to internal handlers first.
4. Add external webhooks after the outbox path is stable.

Do not start with direct "call webhook after DB write" logic. That will create immediate reliability problems and make retries unsafe.

## Permission Model Direction

Current model:

- `user`
- `staff`
- `admin`

Target model:

- role remains a coarse identity bucket
- permissions become capability-based

Examples:

- `manage_products`
- `manage_orders`
- `manage_users`
- `manage_promotions`
- `manage_channels`
- `manage_inventory`
- `manage_payments`
- `manage_fulfillment`
- `view_reports`
- `manage_webhooks`

Recommended transition:

1. Keep current roles.
2. Add service-layer permission checks behind the scenes.
3. Map `admin` to all permissions initially.
4. Give `staff` a limited permission subset.
5. Introduce partner-specific permission groups after the shared permission plumbing exists.

## What We Should Not Do Yet

To keep this plan practical, do not do these first:

- rewrite the app into microservices
- adopt GraphQL as a prerequisite
- split storefront and admin into separate repositories
- move away from server-rendered HTML before the domain model is ready
- build multi-seller settlement before channels, inventory, and payments are solid

## Delivery Phases

### Phase 1: Security and checkout correctness

Deliver:

- CSRF tokens
- auth throttling
- startup validation
- real payment integration
- payment records
- checkout idempotency

More specifically:

- add `payments` and `payment_attempts`
- ensure checkout completion is transactionally bound to payment intent or payment attempt creation
- add deduplicated completion using idempotency keys
- validate totals from persisted pricing snapshots instead of recalculating from request form values

Reason:

This directly reduces operational risk in the current app.

### Phase 2: Domain split of commerce

Deliver:

- split `commerce` into `pricing`, `checkout`, `payment`, `inventory`, and `order`
- move constants and arithmetic into pricing
- introduce `checkouts` and `payments`

More specifically:

- move shipping-fee logic and tax estimation out of the current broad commerce service
- give checkout its own repository and service contracts
- make order creation a downstream result of successful checkout completion

Reason:

Without this split, every new commerce feature will keep piling into one service.

### Phase 3: Inventory architecture

Deliver:

- warehouses
- stocks
- reservations
- allocations
- release workflow
- partner fulfillment built on shared inventory and order rules

More specifically:

- reserve inventory when checkout becomes payment-ready
- release reservations on expiry or payment failure
- allocate inventory on successful order creation
- let partner operations consume allocation-aware views instead of raw product stock values

Reason:

Inventory correctness is a prerequisite for scale and for better fulfillment operations.

### Phase 4: Channels and pricing

Deliver:

- channels
- channel pricing
- channel availability
- channel-aware shipping and promotions

More specifically:

- allow per-channel publication and purchaseability
- allow distinct prices and currencies per channel
- prepare for regional, wholesale, or partner-specific catalog views without cloning products

Reason:

Channels are the cleanest way to support multiple commercial surfaces without duplicating catalog state.

### Phase 5: Promotions and shipping

Deliver:

- vouchers
- catalogue and order discounts
- shipping zones and rates
- ETA logic

More specifically:

- start with simple fixed and percentage discounts
- then add subtotal thresholds, voucher constraints, and free-shipping rewards
- shipping resolution should become a deterministic service that returns quoted method choices for a checkout context

Reason:

These features depend on stronger pricing and checkout foundations.

### Phase 6: Permissions, events, and integrations

Deliver:

- permissions
- event outbox
- webhook subscriptions
- search, tax, and analytics adapters

More specifically:

- introduce capability-based checks for sensitive admin and partner actions
- push business events through the outbox first, then external webhooks
- connect secondary concerns such as search indexing and analytics to domain events instead of route-level side effects

Reason:

This is where the platform becomes extensible rather than merely feature-complete.

## Immediate Go Work Items

These are the next concrete implementation tasks to start with.

1. Create `internal/service/payment` and move payment method handling out of `commerce`.
2. Add `payments`, `payment_attempts`, and `idempotency_keys` tables.
3. Create `internal/service/checkout` and a `checkouts` table so checkout is not equivalent to "cart plus POST".
4. Add `warehouses`, `stocks`, and `stock_reservations`.
5. Introduce a minimal `event_outbox` table and dispatcher loop.
6. Replace route-level role assumptions with permission checks behind a compatibility layer.

## Suggested First Schema Milestone

If we want the smallest high-value first database change set, it should be:

- `payments`
- `payment_attempts`
- `idempotency_keys`
- `checkouts`
- `checkout_lines`
- `warehouses`
- `stocks`
- `stock_reservations`
- `event_outbox`

That gives us:

- real checkout lifecycle
- safer payment work
- inventory foundations
- eventing foundations

without forcing channels or full promotions immediately.

## Success Criteria

Sokomoko should be considered structurally ready for the next stage when these statements become true:

- checkout, order, payment, pricing, and inventory are separate Go domains
- payments have durable state and provider references
- inventory is reservation-based, not only decrement-based
- discounts are policy-driven rather than hard-coded
- permissions are capability-based, not only role-based
- integrations are driven by adapters and durable events
- channel support exists for at least pricing and availability

That is the target architecture.

The implementation should remain recognizably Sokomoko: a Go system with clean domain boundaries, simple deployment, and the option to expose more APIs later without reworking the core model again.
