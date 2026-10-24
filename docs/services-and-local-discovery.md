# Services and local discovery

Status: service listings, seller forms and public discovery are implemented with migration 10;
booking and map integration below remain a design proposal. The demo must continue to say that
listings, bookings and payments are illustrative; no real sale or appointment
is confirmed.

## Product model

Keep the existing partner/shop identity, team membership and unique user
handles. Add a **service listing** beside a product listing. Products reserve
stock; services reserve time, people or equipment. Routing a haircut through
inventory quantity would make availability, cancellation and fulfilment
incorrect.

A service belongs to a partner and offers a title, description, category,
images, duration, preparation/buffer time, currency, and a price mode: fixed,
from, or quote required. Store money in integer minor units. Distinguish a
salon business from its individual haircut offerings and practitioners. One
business can offer products and services from several locations.

Use these records:

| Record | Purpose |
| --- | --- |
| `service_listings` | Partner ownership, category, duration, price mode, publication state |
| `partner_locations` | Public business address, verified pin, timezone, opening hours and visibility |
| `service_location_offerings` | Which services are available at each branch, at what price |
| `service_resources` | Practitioner, chair or room and its capacity |
| `service_availability` / `service_exceptions` | Weekly hours, leave, holidays and overrides |
| `service_bookings` | Buyer, immutable offering/price snapshot, location/resource, start/end and state |
| `service_booking_events` | Audited changes and actor IDs, without free-form medical details |
| `service_areas` | Coverage for mobile providers; separate from the provider's private home address |

Start with one resource per booking and capacity one. Use UTC timestamps plus
an IANA timezone for business rules and presentation. Enforce non-overlapping
active reservations at the database level (a time-range exclusion constraint
for a resource), including unexpired temporary holds. Confirm in a transaction
with idempotency and ownership checks; never trust a browser's displayed slot.
A cleanup worker expires holds through explicit state changes. Test concurrent
bookings, cancellations, timezone boundaries and rescheduling before release.

Suggested states: requested -> confirmed -> in progress -> completed, with
explicit rejected, cancelled, expired and no-show branches. For a first release,
use request-to-book rather than promise automatic confirmation. Show the seller's
response deadline and what confirmation means. Deposits, refunds and payment
capture can follow once this lifecycle is reliable. Service completion should
not reuse parcel/courier delivery statuses.

## Discovery and maps

Give users an accessible list and an optional map, sharing category, area,
price and availability filters. Search by a typed neighbourhood first; offer
"Use my location" as an explicit action. Avoid continuous location tracking.
The list must remain useful without geolocation, JavaScript maps or WebGL.

MapLibre GL JS is a candidate renderer; it is not a tile service, geocoder or
routing provider. Pin and review its version when implementation starts, use
its documented CSP-compatible bundle, and self-host application assets. Reuse
the site's typography, colours, cards and focus styles rather than an unrelated
map UI. Preserve visible map/data attribution.

For geographic search, evaluate PostGIS on the existing PostgreSQL deployment:
store public business points as `geography(Point,4326)`, add a GiST index, and
use bounded `ST_DWithin` radius queries with stable pagination. Its geography
distance argument is metres and the predicate can use an index. This is a
schema/platform change: confirm the deployed database image and backup/restore
support before migrating. Do not scan every listing in the browser. Distances
are straight-line estimates unless a routing service computes travel distance.

Keep tile/geocoding/routing providers behind configuration and narrow adapters.
Choose hosted service or self-hosted infrastructure after assessing expected
traffic, Nairobi/local address coverage, cost, privacy and data licensing. The
public OSM tile servers have usage restrictions and no SLA; they are not a
free production capacity plan. Public Nominatim has an application-wide maximum
of one request/second and prohibits client autocomplete. Do not wire keystroke
search directly to it. Geocode seller addresses on explicit submission, cache
per provider terms, validate bounds, and let the seller correct a pin.

Third-party maps can observe IP addresses and viewed areas. Keep precise buyer
coordinates, private delivery addresses, access tokens and appointment notes
out of public URLs, analytics, logs and third-party geocoding queries. Publish
an exact pin only for a business that chooses public premises. Mobile/home-based
providers should expose an approximate service area, with private visit details
shared only with the booked parties under an explicit policy. A rounded home
coordinate can still identify someone; prefer a named neighbourhood or a broad
predefined area. Don't return the hidden exact location in public API payloads.

## Messaging, privacy and trust

A "Message provider" action can reuse the encrypted buyer/shop thread with an
immutable service context. A later booking-specific conversation must verify
booking ownership on every operation. Practitioners or shop staff are explicit
cryptographic recipients; show their devices when trusting the roster. Adding
a receptionist must not silently disclose an existing transcript.

E2EE chat does not make bookings end-to-end encrypted. The scheduling service
needs structured times/resource IDs and access-controlled booking records.
Keep sensitive free-form notes out of this metadata. Map and scheduling events
must not copy chat text into logs or notifications. Recipient-disclosed reports
retain the existing warning and narrow moderation boundary.

For dentists and other regulated providers, separate a public provider directory
from clinical care. Verify practitioner/facility credentials against the relevant
authoritative register and applicable jurisdiction before using a verified badge;
store the verification source, scope and date. Identity, licence and customer
review badges mean different things. Do not collect symptoms, diagnoses, clinical
records or emergency requests in the first booking form. Adding those requires
a separate jurisdiction-specific privacy, retention, consent and clinical workflow
review. An appointment request must not imply medical advice or emergency access.

Reviews should require a completed booking and protect both sides from spam.
Seller publication, location edits and booking updates need existing partner
membership checks plus audit events. Rate-limit discovery, contact attempts,
booking holds and cancellations. Give mobile providers tools to block/report
abuse without exposing their home address.

## Delivery order

1. Publish moderated service listings and public/approximate locations; add list
   discovery and contextual encrypted enquiries. All demo data stays labelled.
2. Add the optional map with a selected provider, attribution, privacy controls
   and bounded geographic search. Verify on keyboard, mobile and slow networks.
3. Add request-to-book, seller confirmation, cancellations and resource conflict
   prevention. Exercise concurrent requests and permission changes.
4. Only then consider instant booking, deposits/refunds, recurring appointments,
   multi-resource capacity and live travel estimates. Rider live tracking is a
   separate, consent-bound feature with its own retention policy.

## Primary documentation consulted

- [PostGIS ST_DWithin](https://postgis.net/docs/ST_DWithin.html)
- [MapLibre GL JS and CSP guidance](https://maplibre.org/maplibre-gl-js/docs/)
- [OpenStreetMap tile usage policy](https://operations.osmfoundation.org/policies/tiles/)
- [Public Nominatim usage policy](https://operations.osmfoundation.org/policies/nominatim/)
- [PostgreSQL 17 range constraints](https://www.postgresql.org/docs/17/rangetypes.html#RANGETYPES-CONSTRAINT)

These sources support the geographic/platform constraints; the proposed product
model and rollout are architectural recommendations for this repository.

## Implemented foundation

Service listings have a shop owner, stable slug, title, category, duration,
fixed/from/quote pricing in KES or USD, broad public area, premises/mobile/remote
mode and draft/published/archived state. They do not reserve stock, accept
appointments, store precise coordinates or claim practitioner verification.
Public queries include only published listings of active shops. Seller writes
lock current membership and role, use optimistic versions to reject stale edits,
and record an audit event in the same transaction. Search treats user text
literally and bounds pagination. Owner IDs and slugs cannot be reassigned by an
edit. Sensitive visit details and clinical notes do not belong in listings.

### HTTP contract

Public discovery uses `GET /api/v1/catalog/services` with `q`, `category`,
`area`, `delivery_mode`, `page` and `limit`; details use
`GET /api/v1/catalog/services/{slug}`. Only published listings belonging to
active shops are visible. Search terms are literal text, including `%` and `_`.

Seller lists support an optional `state=draft|published|archived` query parameter
in both the workspace `/services` page and `GET /api/v1/partner/services`. Omit
`state` or leave it empty to show all states. Unknown values return HTTP 400.
The state predicate is applied with shop ownership in SQL before counting and
pagination; public discovery continues to expose only published services.

Authenticated sellers use `GET|POST /api/v1/partner/services` and
`GET|PATCH /api/v1/partner/services/{id}`. Writes require the session CSRF
header. Ownership is derived from the authenticated seller's active shop
membership; clients cannot choose an owner or slug. A PATCH supplies all
editable fields and the current `version`, rather than a partial field merge.
Stale updates return 409, and another shop's records return 404.

Example JSON for creating a draft (send with the authenticated partner session,
`Content-Type: application/json` and `X-CSRF-Token`):

```json
{
  "title": "Neighbourhood haircut",
  "description": "A sample haircut service for the demo directory.",
  "category": "hair",
  "duration_minutes": 30,
  "price_mode": "fixed",
  "price": "500.00",
  "currency": "KES",
  "public_area": "Westlands, Nairobi",
  "delivery_mode": "premises",
  "state": "draft",
  "version": 0
}
```

For updates, send the same editable fields with the `version` returned by the
latest read. The server generates `id`, `partner_id`, `slug`, timestamps and the
next version. Unknown JSON fields are rejected. Categories are `hair`, `beauty`,
`dental`, `wellness`, `home` and `other`; delivery modes are `premises`, `mobile`
and `remote`. Duration is 5–480 minutes; currencies are `KES` or `USD`.

Service prices in the directory, detail page, private preview and seller list
are formatted from minor units using the listing’s saved currency, independently
of the marketplace currency setting. For example, `50000` cents in `KES` displays
as `KES 500.00`; quote-only listings show a quote prompt instead of an amount.

Price input is a decimal string in `price`; returned `price_cents` is an integer
in minor currency units. Quote-only listings require zero price. No appointment,
payment, location tracking or medical-data collection is performed by these
endpoints. The storefront supports `/services` and `/services/{slug}`. Sellers manage offerings
on their workspace host at `/services`, `/services/new` and
`/services/{id}/edit`. Forms preserve entered text on validation errors and stale
updates, and require explicit review of the current version after a conflict.
The pages reuse the storefront and workspace brand, font, navigation and form
styles. Demo mode clearly marks offerings as samples with no appointments or
payments.


### Demo content

The development-only `seed` command adds three sample services (haircut, mobile
home maintenance and remote consultation) owned by the existing Urban Goods demo
shop. Re-running it inserts missing samples by stable slug and preserves seller
edits, archived state and version numbers. Migration and ordinary production
startup never seed these listings. Existing installations need an explicit seed
run to see the samples; creating listings through the seller workspace works
without them.

Implementation references: [PostgreSQL 17 INSERT and ON CONFLICT](https://www.postgresql.org/docs/17/sql-insert.html),
[PostgreSQL 17 constraints](https://www.postgresql.org/docs/17/ddl-constraints.html).


### Encrypted enquiries

Service detail pages link to the encrypted compose flow using `service_id`.
Migration 11 stores this context separately from product and order conversations.
Only published services from active, staffed shops can be contacted; archiving a
listing closes sending but preserves authorized history. Shop membership, device
approval and relationship blocks use the existing messaging rules. These are
enquiries only and do not confirm bookings. See [messaging](messaging.md#service-enquiries).

Migration references: [PostgreSQL 17 ALTER TABLE](https://www.postgresql.org/docs/17/sql-altertable.html)
and [partial indexes](https://www.postgresql.org/docs/17/indexes-partial.html).


### Correcting invalid listings

Service create/update validation returns HTTP 400 with stable field names in
`error.fields`, alongside `error.code: "invalid_service"` and a general message.
For example, an out-of-range duration includes
`"fields":{"duration_minutes":"Duration must be a whole number from 5 to 480 minutes."}`.
Multiple invalid fields are reported together; field messages contain corrective
instructions rather than submitted content. Missing or invalid version values
require reloading the form. A valid but stale version still returns 409.

Service descriptions accept multiple lines. Browser form CRLF line endings are
normalized to LF before validation and storage, so the 3000-character limit counts
each line break once. Descriptions remain escaped plain text on public pages,
with their line breaks preserved. Other control characters remain invalid.

Seller forms present a linked summary, identify invalid controls with
`aria-invalid` and connect them to inline text with `aria-describedby`. Error
pages are identified in the document title. Submitted values, including malformed
duration text, are retained for correction. No listing write occurs when field
validation fails. This follows the [W3C form notification guidance](https://www.w3.org/WAI/tutorials/forms/notifications/).


### Filtering where a service happens

Public HTML and JSON discovery accept `delivery_mode=premises`, `mobile` or
`remote`; omit it to include all modes. This combines with category, literal text
and public-area filters. Results and total counts use the same predicates, and
pagination keeps the chosen filters. Unknown mode values return 400. The mode
is also shown on result cards so users can distinguish a business visit, a
provider who travels to them and a remote offering. This filter does not request
or infer the visitor's location.

### Private seller previews

The partner workspace links to `/services/{id}/preview` from the listing table
and edit form. This renders the saved listing with the public detail design,
its current draft/published/archived state and a private-preview notice. Unsaved
edits are not included. Previewing does not publish a listing or open an enquiry.
The route requires current membership in the owning active shop, sends
`Cache-Control: no-store`, and leaves public draft/archived URLs unavailable.

### Browse one business

Public service details link to `/services?partner_id=<id>` to browse other
published offerings from that business. HTML and JSON discovery accept this
optional positive 32-bit integer and combine it with the existing filters.
The HTML search form and pagination retain it; clearing filters returns to all
businesses. Unknown or suspended businesses return an empty result, and private
listing states stay excluded. Invalid, zero, negative or overflowing IDs return
400 rather than silently broadening the search.
