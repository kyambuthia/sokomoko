#!/usr/bin/env bash
# Verify the checked-in splat assets match what their pinned sources build.
#
# The models are compared member by member: the SOG container stores a
# timestamp, so two builds of the same model differ in the archive even when
# the gaussians are identical.
set -euo pipefail
cd "$(dirname "$0")/.."
check_dir="$(mktemp -d)"
trap 'rm -rf "$check_dir"' EXIT

MODELS_DIR="${SPLAT_SOURCE_DIR:-$HOME/Projects/asset-gen/outputs}"

compare_model() {
  local name="$1" built="$2"
  local a="$check_dir/a" b="$check_dir/b"
  rm -rf "$a" "$b"
  mkdir -p "$a" "$b"
  unzip -qo "internal/ui/static/models/products/$name.sog" -d "$a"
  unzip -qo "$built" -d "$b"
  local members
  members="$(cd "$a" && ls)"
  if [ "$(cd "$b" && ls)" != "$members" ]; then
    echo "Stale splat asset: $name.sog has different members. Run make splat-assets." >&2
    exit 1
  fi
  local member
  for member in $members; do
    if ! cmp -s "$a/$member" "$b/$member"; then
      echo "Stale splat asset: $name.sog ($member differs). Run make splat-assets." >&2
      exit 1
    fi
  done
}

if [ -d "$MODELS_DIR" ]; then
  SPLAT_MODELS_OUT="$check_dir/models" bash scripts/build-splat-assets.sh --models-only >/dev/null
  for name in ceramic-vase leather-backpack; do
    compare_model "$name" "$check_dir/models/$name.sog"
  done
  echo 'Splat models match their pinned sources.'
else
  echo "No full-fidelity models in $MODELS_DIR; the bundled splats were not verified."
fi

SPLAT_VIEWER_OUT="$check_dir/splat-viewer.html" bash scripts/build-splat-assets.sh --viewer-only >/dev/null
if ! cmp -s internal/ui/static/splat-viewer.html "$check_dir/splat-viewer.html"; then
  echo "Stale viewer asset: splat-viewer.html. Run make splat-assets and review the changes." >&2
  exit 1
fi
echo 'The bundled viewer matches its pinned source.'
