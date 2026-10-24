#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
output_dir="${CHAT_CRYPTO_OUT_DIR:-internal/ui/static/crypto}"
expected='wasm-bindgen 0.2.114'
if [[ "$(wasm-bindgen --version)" != "$expected" ]]; then
  echo 'Install wasm-bindgen-cli 0.2.114 with: cargo install wasm-bindgen-cli --version 0.2.114 --locked' >&2
  exit 1
fi
export RUSTFLAGS="${RUSTFLAGS:-} --remap-path-prefix=$PWD=. --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=cargo-home"
cargo build --manifest-path web/crypto/Cargo.toml --target wasm32-unknown-unknown --release --locked
wasm-bindgen web/crypto/target/wasm32-unknown-unknown/release/sokomoko_mls.wasm \
  --target web --out-dir "$output_dir" --out-name sokomoko_mls --no-typescript
# No package manager, third-party CDN, runtime compiler, or debug-key features.
python3 scripts/chat-crypto-notices.py "$output_dir"
