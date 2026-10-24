#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
check_dir="$(mktemp -d)"
trap 'rm -rf "$check_dir"' EXIT
CHAT_CRYPTO_OUT_DIR="$check_dir" bash scripts/build-chat-crypto.sh
for asset in sokomoko_mls.js sokomoko_mls_bg.wasm THIRD_PARTY_NOTICES.txt; do
  if ! cmp -s "internal/ui/static/crypto/$asset" "$check_dir/$asset"; then
    echo "Stale encryption asset: $asset. Run make chat-crypto and review the changes." >&2
    exit 1
  fi
done
echo 'Encryption assets match their pinned source and license inventory.'
