#!/usr/bin/env bash
# Record TUI + CLI showcase casts with asciinema, render GIFs with agg.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${SHOWCASE_OUT:-$ROOT/scripts/showcase/out}"
ASSETS="${SHOWCASE_ASSETS:-$ROOT/assets}"
AGG="${AGG:-$HOME/.cargo/bin/agg}"
COLS="${SHOWCASE_COLS:-100}"
ROWS="${SHOWCASE_ROWS:-32}"
export PATH="$HOME/.cargo/bin:$PATH"
export SHOWCASE_COLS="$COLS" SHOWCASE_ROWS="$ROWS"
export PLEX2JELLYFIN_TEST_NO_ESCALATE=1

mkdir -p "$OUT" "$ASSETS"
command -v asciinema >/dev/null
command -v "$AGG" >/dev/null || command -v agg >/dev/null
[[ -x "$(command -v agg)" ]] && AGG="$(command -v agg)"

render() {
  local cast="$1" gif="$2"
  "$AGG" \
    --theme github-dark \
    --font-size 18 \
    --speed 1.15 \
    --idle-time-limit 1.2 \
    --fps-cap 30 \
    "$cast" "$gif"
  ls -lh "$gif"
}

echo "== TUI installer =="
export SHOWCASE_HOME=/tmp/p2j-tui-demo
rm -rf "$SHOWCASE_HOME"
mkdir -p "$SHOWCASE_HOME"
asciinema rec "$OUT/tui-installer.cast" \
  --overwrite \
  --quiet \
  --title "plex2jellyfin TUI installer" \
  --idle-time-limit 1.2 \
  --window-size "${COLS}x${ROWS}" \
  --command "python3 $ROOT/scripts/showcase/drive_tui.py"
render "$OUT/tui-installer.cast" "$ASSETS/tui-installer.gif"

echo "== CLI setup + scan =="
export SHOWCASE_HOME=/tmp/p2j
rm -rf "$SHOWCASE_HOME"
mkdir -p "$SHOWCASE_HOME"
asciinema rec "$OUT/cli-setup-scan.cast" \
  --overwrite \
  --quiet \
  --title "plex2jellyfin CLI setup + scan" \
  --idle-time-limit 1.2 \
  --window-size "${COLS}x${ROWS}" \
  --command "python3 $ROOT/scripts/showcase/drive_cli.py"
render "$OUT/cli-setup-scan.cast" "$ASSETS/cli-setup-scan.gif"

echo "Done."
ls -lh "$ASSETS"/tui-installer.gif "$ASSETS"/cli-setup-scan.gif
