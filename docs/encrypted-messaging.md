# End-to-end encrypted messaging

## Status and security boundary

New messaging uses a browser client backed by **OpenMLS 0.9.2**, implementing
RFC 9420, with the RustCrypto provider 0.6.0. Go authenticates and authorizes
participants, handles device enrollment, and delivers opaque MLS messages.
It does not generate or receive device private keys, MLS group secrets,
protocol-state snapshots, ordinary message plaintext, or local history.

This is an implementation awaiting independent security review, not an
externally audited product. Do not claim it protects an unlocked compromised
browser, malicious same-origin JavaScript, or a server that delivers a modified
client. Fingerprint verification is manual; there is no independently witnessed
key-transparency directory. This implementation uses the classical MLS suite
`MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519`, not a post-quantum suite.

The server still sees account and shop relationships, device public keys,
conversation context, timing, message sizes, and read markers. This is content
encryption, not anonymous routing. Existing order, product, service, delivery and account
data are outside this encryption boundary.

## Devices and trust

Each browser origin and signed-in account has its own device identity. The
browser creates an Ed25519 signing key and MLS key packages locally. The first
device enrolls atomically under a user lock. Every later device is pending
until an existing approved device signs approval of its exact ID/public key.
The approval UI requires entering its full SHA-256 public-key fingerprint,
which must be compared on the new browser through a trusted channel.

Password reset grants account access, not decryption-key access. Losing all
approved devices does not automatically bootstrap a new trusted identity.
There is no administrative decryption key or password-based key escrow.
**There is no recovery backup or historical device transfer in this release.**
New devices receive future sessions only. Clearing browser storage loses local
keys/history. Keep an approved device available before clearing storage or
moving browsers. Different subdomains have separate browser storage and are
separate devices even on the same computer.

Revocation is signed by an approved device. Directory mutations take an
exclusive advisory lock; encrypted operations share it. Per-device requests
are serialized separately, so concurrent requests cannot race a revocation or
reuse a nonce for different effects. Marketplace authorization is checked
again before returning cached replies. Courier reassignment therefore revokes
cached responses as well as new reads and writes.

## Conversation membership

An encrypted roster consists of the buyer's approved devices and the other
buyer's, current courier's, or active shop team's approved devices. The first
version supports at most 32 devices per conversation. Shop teammates who
have not enrolled a device do not receive cryptographic history automatically.
Their enrolled devices become visible in the roster once approved.

Every client explicitly trusts the roster hash and can inspect each public-key
fingerprint. The MLS group ID binds the conversation ID, session ID and roster
hash. Both initial group creation and Welcome processing verify **every MLS
credential and signing key** against this roster. Decrypted sender credentials
must also match the event's stated sender device. TLS/session authentication
alone is not treated as peer-key verification.

When membership changes, sending under the previous roster stops. A device
already in the preceding session must create a fresh group for the new roster.
Other clients review the changed roster and join using the MLS Welcome. A
newly approved device cannot unilaterally replace an established session.
No old group secret or old transcript is automatically shared with it.

In-band add/remove proposals are rejected by the wrapper. Fresh groups are the
explicit application membership transition in this first version. Within a
stable roster, accepted conversations perform an MLS self-update before each
application message. The one introductory message for a pending request is
sent without a preceding update; the receiving side must accept before replies
or further messages are permitted.

One-time packages are claimed transactionally, bound to the creating device
and conversation, and consumed by one published session. Unclaimed packages older than 30 days are excluded
from availability and claims; clients replenish the pool when online, well
before the library's default 84-day lifetime. Session publication
checks that exactly one package is present for every other roster device.
The protocol library validates package signatures and the client compares the
resulting credentials with the approved roster. The server does not claim to
validate opaque MLS contents cryptographically.

Marketplace checks remain in force. A courier session belongs to one immutable
assignment event. A replacement courier gets a different conversation and no
old messages. Removed shop members lose API access; membership changes rotate
future encryption recipients. Revocation cannot retract previously copied keys
or plaintext. Existing clients must be online to advance an affected roster;
sending waits if none is available.

## Local state, ordering and failures

The browser uses Web Locks to serialize operations across tabs for the same
origin/account. An IndexedDB record contains MLS state, the exact signed
outbox entry, local outgoing text and decrypted history. The whole record is
AES-GCM encrypted with a locally stored non-extractable CryptoKey. The wrapping
key and ciphertext both reside on the device: this is not protection against
malicious code running as the same origin or an unlocked compromised browser.

Updated MLS state and the encrypted outbox are saved atomically **before** a
network submission. Lost responses reuse the exact device-signed request and
ciphertext. Server request caching makes those retries idempotent. An explicit
server rejection restores the pre-operation state where needed; network or
server uncertainty preserves the outbox for retry. No plaintext is submitted
as an error fallback.

The server serializes events per conversation and requires an exact preceding
event ID for each session. Concurrent sends based on stale state are rejected,
then the client refreshes before the user retries. Successful reads save the
advanced receive state and local plaintext cache before advancing the cursor.
Own outgoing messages use the local copy rather than attempting to decrypt a
consumed sending generation. MLS rejects tampering and replay.

Local transcript retention is separate from ratchet-secret deletion: an
attacker who obtains saved local transcripts can read that history regardless
of the transport protocol's forward secrecy. IndexedDB and filesystem backups
also cannot promise physical secure deletion. Neither ordinary ciphertext nor
moderator disclosures currently have an automatic retention-expiry policy.

## HTTP and persistence

`POST /api/v1/encrypted-chat` requires an authenticated session and CSRF token.
The request contains `op`, `device_id`, `nonce`, `payload` (an exact JSON string),
and `signature` (base64 Ed25519). The signed text is:

```
sokomoko-e2ee-v1\n<op>\n<user_id>\n<device_id>\n<nonce>\n<base64 SHA-256 of exact payload bytes>
```

Operations: `register`, `devices`, `approve`, `revoke`, `packages`, `start`,
`roster`, `claim`, `session`, `events`, `send`, `read`, `report`. Payloads contain
public metadata or ciphertext. Private state is never part of this API.
GET is not accepted. Bodies are bounded; responses are private/no-store.
Device signatures bind the account, operation, nonce and exact payload.

Migration `0009_encrypted_messaging.sql` adds device, package, encrypted-session,
encrypted-event, read-marker, request-idempotency and disclosure-report tables.
The encrypted event stream shares the existing global message ID sequence so
inbox ordering remains consistent. History reads return batches of 50 events,
visible only to approved devices present in that session and users who still
have marketplace authorization.

The existing plaintext Start/Send service methods reject new submissions with
`encryption_required`; their HTML equivalents also fail closed. Earlier history
is separately labeled at `?legacy=1` and is not retroactively encrypted. Old
plaintext database records and backups require a separate retention decision.
The encrypted client needs JavaScript, secure-context WebCrypto, Web Locks and
IndexedDB. No-JavaScript plaintext forms are not an alternative transport.

## Reporting

A recipient can choose to disclose one decrypted message and a reason. Only
that explicit disclosure enters `chat_encrypted_reports`; ordinary events stay
encrypted. The admin report queue labels these as **recipient-provided text,
not independently verified authorship evidence**. Reports must reference a
received event in a session containing the reporting device. A shop teammate
cannot report their own shop's messages as if they were the buyer. Moderator
resolution is audited. There is no general admin conversation-decryption path.

## Build and verification

Generated JS/WASM and dependency notices live under `internal/ui/static/crypto`
and are embedded in the Go binary. No runtime CDN or npm package is used.
Rebuild using `scripts/build-chat-crypto.sh` with Rust 1.99.0, the wasm32 target,
and wasm-bindgen-cli 0.2.114. `Cargo.lock` pins the complete dependency graph.
WASM compilation uses the narrow CSP `wasm-unsafe-eval` source expression;
JavaScript `unsafe-eval` remains disabled.

```
cargo test --manifest-path web/crypto/Cargo.toml --locked
SOKOMOKO_REQUIRE_DB_TESTS=1 go test -race ./internal/db ./cmd/sokomoko -run 'TestEncrypted|TestMessagingHTTP|TestChat' -count=1
go vet ./...
node --check internal/ui/static/scripts/ui/encrypted-chat.js
node --check internal/ui/static/scripts/ui/encrypted-store.js
```

Set `SOKOMOKO_TEST_DATABASE_URL` to an isolated disposable PostgreSQL 17 database
for database tests. Protocol tests cover offline Welcome join, persistence,
encryption/replies, tampering, replay and self-update. Database/HTTP tests cover
signatures, first-device enrollment, later-device approval, revocation, current
membership, ordering, idempotency, CSRF and explicit report disclosure.

Primary references:
- [MLS RFC 9420](https://www.rfc-editor.org/rfc/rfc9420.html)
- [MLS architecture RFC 9750](https://www.rfc-editor.org/rfc/rfc9750.html)
- [OpenMLS 0.9.2 source/API](https://docs.rs/openmls/0.9.2/openmls/)
- [OpenMLS storage requirements](https://book.openmls.tech/user_manual/persistence.html)
- [Web Locks](https://www.w3.org/TR/web-locks/)
- [CSP and WebAssembly](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/script-src)


## Dependency advisory status

The 2026-10-24 Rust lockfile audit has unresolved advisories, including the
RustCrypto provider's transitive `crypto-bigint` dependency. The application
rejects unsupported algorithm negotiation, but this does not fix the dependency
or prove complete non-reachability. See [dependency security](dependency-security.md)
for exact findings, evidence, mitigation and the required upstream update.
