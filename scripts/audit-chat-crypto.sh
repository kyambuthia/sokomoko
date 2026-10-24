#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
expected='0.22.2'
actual="$(cargo audit --version)"
if [[ "$actual" != *" $expected" ]]; then
  echo "Install cargo-audit $expected with: cargo install cargo-audit --version $expected --locked" >&2
  exit 1
fi
# Scan the complete lockfile, including optional dependencies. Do not ignore
# advisories based solely on the application's selected ciphersuite.
exec cargo audit --file web/crypto/Cargo.lock
