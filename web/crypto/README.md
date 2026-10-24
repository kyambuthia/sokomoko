# Browser MLS boundary

This crate pins OpenMLS 0.9.2 and RustCrypto provider 0.6.0. It implements a
small JSON/WASM boundary, not a new cryptographic protocol. Private state must
stay in the encrypted browser IndexedDB record. `Cargo.lock` pins transitive
dependencies. Never enable `crypto-debug` or `content-debug` in builds.

Build prerequisites: Rust 1.99.0, `wasm32-unknown-unknown`, and
`wasm-bindgen-cli` 0.2.114. Run `scripts/build-chat-crypto.sh`; the generated
same-origin JS/WASM assets are embedded by Go. Run `cargo test --manifest-path
web/crypto/Cargo.toml --locked` for offline join, ciphertext, authentication,
replay, persistence and self-update checks.

The first version uses fresh groups when the approved device roster changes.
A device already present in the preceding group must initiate the new group.
Every client verifies the MLS member credentials and signature keys against
its explicitly trusted roster. No membership proposals are accepted in-band.
Within a stable roster, MLS self-update commits contribute fresh key material.

The application must serialize local operations and atomically save updated
state with the exact encrypted outbox entry before network transmission.
