#!/usr/bin/env bash
# Rebuild the bundled product splats and the viewer that renders them.
#
# The models are decimated from a full-fidelity copy with splat-transform,
# which needs a PLY intermediate. The viewer is minified with esbuild.
# Both steps are reproducible from the inputs below, so the checked-in assets
# can be verified rather than trusted.
#
# Usage: scripts/build-splat-assets.sh [--models-only|--viewer-only]
#
# Inputs:
#   $SPLAT_SOURCE_DIR          full-fidelity .sog files
#                              (default ~/Projects/asset-gen/outputs)
#   $SPLAT_VIEWER_SOURCE       the unminified viewer shell
#                              (default web/splat-viewer/splat-viewer.source.html)
#
# Outputs:
#   $SPLAT_MODELS_OUT          default internal/ui/static/models/products
#   $SPLAT_VIEWER_OUT          default internal/ui/static/splat-viewer.html
set -euo pipefail
cd "$(dirname "$0")/.."

MODELS_DIR="${SPLAT_SOURCE_DIR:-$HOME/Projects/asset-gen/outputs}"
VIEWER_SOURCE="${SPLAT_VIEWER_SOURCE:-web/splat-viewer/splat-viewer.source.html}"
MODELS_OUT="${SPLAT_MODELS_OUT:-internal/ui/static/models/products}"
VIEWER_OUT="${SPLAT_VIEWER_OUT:-internal/ui/static/splat-viewer.html}"

# The viewer is much heavier than a model, so it carries the load on a first
# visit; the models are kept small enough to fetch on a phone connection.
SPLAT_DECIMATE="${SPLAT_DECIMATE:-25%}"

mode="all"
case "${1:-}" in
  --models-only) mode="models" ;;
  --viewer-only) mode="viewer" ;;
  "") ;;
  *) echo "unknown option: $1" >&2; exit 2 ;;
esac

if ! command -v npx >/dev/null 2>&1; then
  echo "npx is required" >&2
  exit 1
fi

build_models() {
  if [ ! -d "$MODELS_DIR" ]; then
    echo "no full-fidelity models in $MODELS_DIR" >&2
    exit 1
  fi
  local work
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' RETURN
  mkdir -p "$MODELS_OUT"
  for name in ceramic-vase leather-backpack; do
    local source="$MODELS_DIR/$name.sog"
    if [ ! -f "$source" ]; then
      echo "missing $source" >&2
      exit 1
    fi
    # Decimation needs a PLY intermediate, so the model is decimated and
    # recompressed in two passes.
    npx --yes @playcanvas/splat-transform@3.10.3 "$source" \
      -d "$SPLAT_DECIMATE" -w "$work/$name.ply" >/dev/null
    npx --yes @playcanvas/splat-transform@3.10.3 \
      "$work/$name.ply" -w "$MODELS_OUT/$name.sog" >/dev/null
    printf '  %-22s %9s bytes\n' "$name.sog" "$(wc -c <"$MODELS_OUT/$name.sog")"
  done
}

build_viewer() {
  if [ ! -f "$VIEWER_SOURCE" ]; then
    echo "missing $VIEWER_SOURCE" >&2
    exit 1
  fi
  local work
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' RETURN
  python3 - "$VIEWER_SOURCE" "$work" <<'PY'
import re
import sys

source_path, work = sys.argv[1], sys.argv[2]
html = open(source_path, encoding="utf8", errors="replace").read()
blocks = re.findall(r'(<script type="module">)(.*?)(</script>)', html, re.S)
if not blocks:
    raise SystemExit("no module script found in the viewer source")
_, body, _ = max(blocks, key=lambda b: len(b[1]))
with open(f"{work}/viewer-script.js", "w", encoding="utf8") as handle:
    handle.write(body)
print(f"  extracted {len(body)} bytes of viewer script")
PY
  npx --yes esbuild "$work/viewer-script.js" --minify --format=esm --target=es2022 \
    --legal-comments=none --outfile="$work/viewer.min.js" >/dev/null
  python3 - "$VIEWER_SOURCE" "$work/viewer.min.js" "$VIEWER_OUT" <<'PY'
import re
import sys

source_path, minified_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
html = open(source_path, encoding="utf8", errors="replace").read()
minified = open(minified_path, encoding="utf8", errors="replace").read()
blocks = re.findall(r'(<script type="module">)(.*?)(</script>)', html, re.S)
_, body, _ = max(blocks, key=lambda b: len(b[1]))
out = html.replace(body, minified)
with open(out_path, "w", encoding="utf8") as handle:
    handle.write(out)
print(f"  viewer {len(html)} -> {len(out)} bytes")
PY
}

case "$mode" in
  models) build_models ;;
  viewer) build_viewer ;;
  all)
    build_models
    build_viewer
    ;;
esac
