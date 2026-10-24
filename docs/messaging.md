# Handles and messaging

**Current transport:** New messages use the [encrypted messaging client](encrypted-messaging.md).
The server-readable transport described below is earlier history and its original
implementation contract. New plaintext Start/Send requests are rejected.
Use the encrypted messaging document for device setup, ciphertext APIs,
current reporting, browser requirements and limitations.

Messaging reuses `users.username` as `@username`. Handles retain their existing
3–32 character ASCII format and case-insensitive uniqueness. Lookup accepts
an optional leading `@`. Handles cannot be changed through the application;
internal user IDs identify participants and authorize actions. Lookup returns
only the contact name and conversation context, never account email or credentials.

## Using messages

- Buyers open **Messages** in the storefront, **Message seller** on a product, **Message service provider** on a service,
  or **Message seller or courier** on an order.
- Partners use **Messages** in their workspace or **Message buyer** on an
  order. Seller conversations belong to the partner account. Active linked partner users may participate through approved devices; each
  has an individual read marker. Joining the shop does not automatically grant
  a device access to past encrypted history.
- Couriers open **Message buyer** on an assigned delivery. Couriers cannot
  receive or initiate unsolicited handle requests.
- An exact handle can start a request between two buyers, or between a buyer
  and a shop. The sender gets one introduction. Only the receiving side may
  accept or decline. Acceptance enables replies. Declined requests cannot be
  reopened by the sender.
- Product, service and order conversations start accepted because they have explicit
  marketplace context. A block applies to the entire relationship, including
  all its product, service and order contexts.

The inbox orders conversations by their most recent message. It has pages of
30 conversations and unread indicators. History has pages of 50 messages.
The current conversation polls every five seconds while the page is visible,
backs off on failures, and stops after access is revoked. Older history pages
do not poll. Ordinary forms work without JavaScript. Read markers change via
POST, automatically when the latest shown message is visible, or through the
explicit **Mark shown messages as read** button.

## Participant boundaries

The store checks active user role and membership on every list, history read,
poll, send and conversation action. An unrelated or deactivated user gets no
thread data. Platform admins and staff do not get access to private threads.

Seller access comes from `partner_users` and an active partner. A mixed order
has separate conversations per partner; no other shop's items, prices,
addresses or conversations are included in messaging responses. If a product
is removed or moves to another seller, its existing conversation becomes
read-only. Old messages retain their original participants.

A delivery conversation is tied to the **assignment event**, not just the
mutable `courier_deliveries` row. Reassignment revokes the former courier's
access and requires a new conversation for the replacement. Even reassigning
the original courier later creates a different thread. The buyer retains
read-only history of earlier assignments. Completed or cancelled delivery
conversations are read-only. No system can retract messages already read or
copied by a participant.

Reads and writes lock the order and then its delivery row in shared mode while
checking the assignment. Dispatch takes locks in the same order. This serializes
chat access against reassignment and cancellation. Mutation transactions also lock the participant pair
and conversation to serialize first-message creation, acceptance, blocking
and sending. A `(conversation_id, sender_user_id, client_nonce)` unique
constraint makes retries idempotent. Reusing a nonce for different text fails.

## Safety and privacy

Messages are plain text, limited to 4,000 Unicode code points / 16,000 UTF-8
bytes, with control characters rejected except newline and tab. HTML is
escaped by `html/template`; browser updates use `textContent`. There are no
uploads, rich HTML, remote previews, or email notifications.

All routes require sessions. Mutations use the existing CSRF protection;
private responses use `Cache-Control: no-store`. Logs omit message bodies
and lookup query strings. **Messages are stored as server-readable database
text, not end-to-end encrypted.** Database, volume and backup encryption are
deployment responsibilities. No automatic message expiry is configured in
this first release; ordinary messages and reports remain in the database
until an operator applies a retention policy. Do not promise deletion or
automatic expiry to users before such a policy is implemented.

Rate limits are PostgreSQL counters shared by application instances:

| Action | Per authenticated account |
| --- | --- |
| Contact lookup, including unsuccessful lookup | 20/minute |
| New conversation | 10/hour |
| Successful message sends | 30/minute and 500/day |
| Reports | 10/hour |

These use fixed windows. Duplicate successful sends do not consume another
message budget. Expired counters for an active account are pruned during
counter updates. Existing IP limits and authentication abuse limits still apply.

Users can report a specific received message, choosing a reason. The form
explains that the selected text will be shared with moderators. Admins review
only those messages at `/message-reports`; the rest of the conversation is
not exposed. Resolving a report creates an audit entry without message text.
Reports do not automatically punish or suspend an account.

Demo mode explains that shared demo credentials also share conversation
history. Use sample information. Checkout, payment and email restrictions
remain the same as the rest of the demo.

## HTTP contracts

HTML routes are available on the storefront (`user`), partner (`partner`),
and courier (`courier`) hosts:

| Method | Path | Purpose |
| --- | --- | --- |
| GET/HEAD | `/messages?before=<last_message_id>` | Inbox |
| GET/HEAD | `/messages/new` | Exact handle lookup or contextual contact chooser |
| POST | `/messages/start` | Resolve target and send introduction atomically |
| GET/HEAD | `/messages/<id>?before=<message_id>` | Thread/history |
| POST | `/messages/<id>/send` | Reply |
| POST | `/messages/<id>/action` | Accept, decline, block, unblock, read, report |
| GET/HEAD, POST | `/message-reports` on admin host | Review reported messages; POST resolves a report |

JSON uses the standard `data`/`error` envelope and session plus CSRF:

| Method | Path | Body or query |
| --- | --- | --- |
| GET | `/api/v1/messages` | Optional `before` inbox cursor |
| POST | `/api/v1/messages/contacts` | A target object |
| POST | `/api/v1/messages` | `target`, `body`, `nonce` |
| GET | `/api/v1/messages/<id>` | Optional `before` or `after` message ID, never both |
| POST | `/api/v1/messages/<id>/send` | `body`, `nonce`; returns message ID and a fresh nonce |
| POST | `/api/v1/messages/<id>/action` | `action`, optional `message_id`, optional `reason` |

A target has exactly one of `handle`, `product_id`, `service_id`, `order_id` or
`delivery_order_id`. An `order_id` without `partner_id` lists eligible
contacts; select a seller with both `order_id` and `partner_id` to send.
Courier contact uses `delivery_order_id`. Server resolution supplies every
participant ID. A nonce is 16 cryptographically random bytes encoded as 32
lowercase hex characters and must be reused when retrying an uncertain send.

## Service enquiries

Migration `0011_service_conversations.sql` attaches a service context to a seller
conversation. `GET /messages/new?service_id=<id>` resolves the published service
to its active shop. The encrypted `start` payload is
`{"target":{"service_id":123}}`; use the existing signed device request protocol.
Clients cannot override the shop or combine service context with another target.

Only buyers can initiate this context; linked shop members can read and reply
using their approved devices. Different services, product enquiries, order
conversations and general shop requests remain separate threads. Relationship
blocks apply across all of them. A service being archived or made a draft closes
sending; authorized participants retain access to their existing conversation.
Re-publishing enables the existing conversation again. Shop suspension and
membership removal continue to revoke seller access.

The service title/ID and shop relationship are server-visible context metadata.
Message content continues through MLS; no plaintext introductory message is
created. A conversation is an enquiry, never an appointment confirmation. The
existing device trust, history and recovery limitations still apply.

## Implementation and verification

Migration `0008_messaging.sql` adds `chat_conversations`, `chat_messages`,
`chat_reads`, `chat_blocks`, `chat_reports`, and `chat_rate_limits`.
`internal/service/messaging` validates input; `internal/db/messaging*.go`
owns transactions and relationship checks. HTML and JSON handlers share the
same service. No new external dependency or messaging server is required.

Focused tests cover unauthorized reads/writes, shop membership changes,
cross-context blocks, mixed orders, assignment replacement, cancelled
deliveries, duplicate/concurrent submissions, pagination, request acceptance,
CSRF, escaped message rendering, and restricted report review. Run them using
an isolated PostgreSQL 17 database:

```sh
SOKOMOKO_REQUIRE_DB_TESTS=1 go test ./internal/db ./internal/service/messaging ./cmd/sokomoko -run 'TestChat|TestNormalizeMessaging|TestMessagingHTTP'
```

The design follows the existing Go 1.26.10 `html/template` escaping rules,
[PostgreSQL 17 row and advisory locking](https://www.postgresql.org/docs/17/explicit-locking.html),
and [OWASP's requirement to check authorization on every request](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html).
