# Dependency security review

Reviewed 2026-10-24 against the checked-in Go modules and Rust lockfile. This is a
point-in-time dependency review, not an independent cryptographic audit.

Run `make vuln` for Go and `make chat-crypto-audit` for Rust. The latter requires
`cargo install cargo-audit --version 0.22.2 --locked`. The scheduled dependency
audit workflow runs both weekly and on manual dispatch because new advisories
can appear without source changes. Its Rust check currently fails for the open
findings below; no advisory ignore list or success override is configured.

## Findings still open

| Finding | Current evidence | Required resolution |
| --- | --- | --- |
| `crypto-bigint` 0.5.5, RUSTSEC-2026-0387 | Active transitive dependency through RustCrypto's elliptic-curve implementations. The advisory marks versions before 0.7.6 affected. | Obtain a compatible upstream provider/dependency update, rebuild browser assets and repeat protocol tests plus the audit. |
| `libcrux-kem` 0.0.9, RUSTSEC-2026-0330 and RUSTSEC-2026-0331 | Present in the complete lockfile; absent from `cargo tree --locked -i libcrux-kem` with this application's enabled features. | Resolve the optional dependency graph to a patched version (0.0.10 or later), or an upstream release that removes it. Do not enable it while affected. |
| `proc-macro-error2` 2.0.1, RUSTSEC-2026-0173 | Unmaintained dependency warning in the lockfile. | Review the upstream replacement path alongside provider updates. |

`make vuln` found no reachable Go vulnerabilities, but reported one advisory in a
required module outside imported packages. This does not imply that all locked
Go modules are advisory-free.

## Algorithm boundary hardening

The browser application creates only
`MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519` groups. It now rejects other
ciphersuites in key-package TLS prefixes **before** signature validation,
Welcomes **before** HPKE processing, and loaded group state **before** send,
receive or update. Restored signature keys must use Ed25519. Tests construct
actual P-256 key packages, Welcomes and stored groups with OpenMLS and verify
that the application rejects them; normal X25519 join, reply, update, replay and
tamper tests still pass.

This limits accidental algorithm negotiation and exposure to the provider's
other algorithms. It is a mitigation, **not** evidence that the transitive
`crypto-bigint` advisory is fixed or a complete proof of non-reachability. The
pinned OpenMLS RustCrypto 0.6.0 provider and basic credential 0.6.0 require the
older elliptic-curve dependency line. A forced incompatible replacement or
locally rewritten arithmetic would need a separate compatibility and security
review; neither is used here. Treat the Rust audit findings as unresolved before
promoting this messaging implementation beyond the demo.

Primary references:

- [RustSec database and cargo-audit](https://rustsec.org/).
- [crypto-bigint 0.7.6 upstream security fixes](https://github.com/RustCrypto/crypto-bigint/pull/1365).
- [libcrux short-key decoding advisory](https://rustsec.org/advisories/RUSTSEC-2026-0331.html).
- Pinned OpenMLS 0.9.2 sources: `KeyPackageTbsIn` TLS field order,
  `KeyPackageIn::validate`, `Welcome::ciphersuite` and `MlsGroup::ciphersuite`.
