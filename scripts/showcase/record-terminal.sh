#!/usr/bin/env bash
# Record TUI + CLI showcase casts → README GIFs.
# Aspect: ~88x40 (wordmark is 80 cols) so the UI isn't lost in ultrawide padding.
# GitHub READMEs strip relative <video>; GIFs are what actually render.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${SHOWCASE_OUT:-$ROOT/scripts/showcase/out}"
ASSETS="${SHOWCASE_ASSETS:-$ROOT/assets}"
AGG="${AGG:-$HOME/.cargo/bin/agg}"
# Hard-set geometry (ignore inherited SHOWCASE_COLS/ROWS from older runs).
# 90x36 ≈ 1.2 pixel aspect with our agg font metrics — fits the 80-col
# wordmark without ultrawide side gutters or portrait letterboxing.
COLS=90
ROWS=36
FONT_SIZE="${SHOWCASE_FONT_SIZE:-30}"
FPS="${SHOWCASE_FPS:-60}"
SPEED="${SHOWCASE_SPEED:-1.0}"
IDLE="${SHOWCASE_IDLE:-1.5}"
export PATH="$HOME/.cargo/bin:$PATH"
export SHOWCASE_COLS="$COLS" SHOWCASE_ROWS="$ROWS"
export PLEX2JELLYFIN_TEST_NO_ESCALATE=1

mkdir -p "$OUT" "$ASSETS"
command -v asciinema >/dev/null
if [[ ! -x "$AGG" ]]; then
  AGG="$(command -v agg)"
fi
[[ -x "$AGG" ]]

render_gif() {
  local cast="$1" gif="$2"
  "$AGG" \
    --theme github-dark \
    --font-size "$FONT_SIZE" \
    --line-height 1.3 \
    --speed "$SPEED" \
    --idle-time-limit "$IDLE" \
    --fps-cap "$FPS" \
    --font-antialiasing 8 \
    "$cast" "$gif"
  ls -lh "$gif"
  python3 - "$gif" <<'PY'
import sys
from PIL import Image
im = Image.open(sys.argv[1])
print(f"  {im.size[0]}x{im.size[1]} frames={im.n_frames} aspect={im.size[0]/im.size[1]:.2f}")
PY
}

echo "== TUI installer =="
export SHOWCASE_HOME=/tmp/p2j-tui-demo
rm -rf "$SHOWCASE_HOME"
mkdir -p "$SHOWCASE_HOME"
asciinema rec "$OUT/tui-installer.cast" \
  --overwrite \
  --quiet \
  --title "plex2jellyfin TUI installer" \
  --idle-time-limit "$IDLE" \
  --window-size "${COLS}x${ROWS}" \
  --command "python3 $ROOT/scripts/showcase/drive_tui.py"
render_gif "$OUT/tui-installer.cast" "$ASSETS/tui-installer.gif"
# Keep an MP4 master in assets for local viewing (not embedded in README)
if command -v ffmpeg >/dev/null; then
  ffmpeg -y -i "$ASSETS/tui-installer.gif" \
    -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2,fps=${FPS}" \
    -c:v libx264 -pix_fmt yuv420p -crf 18 -preset medium -movflags +faststart \
    "$ASSETS/tui-installer.mp4" >/dev/null 2>&1
  ffmpeg -y -ss 1.5 -i "$ASSETS/tui-installer.mp4" -frames:v 1 -update 1 \
    "$ASSETS/tui-installer-poster.png" >/dev/null 2>&1 || true
fi

echo "== CLI setup + scan =="
export SHOWCASE_HOME=/tmp/p2j
rm -rf "$SHOWCASE_HOME"
mkdir -p "$SHOWCASE_HOME"
asciinema rec "$OUT/cli-setup-scan.cast" \
  --overwrite \
  --quiet \
  --title "plex2jellyfin CLI setup + scan" \
  --idle-time-limit "$IDLE" \
  --window-size "${COLS}x${ROWS}" \
  --command "python3 $ROOT/scripts/showcase/drive_cli.py"
render_gif "$OUT/cli-setup-scan.cast" "$ASSETS/cli-setup-scan.gif"

echo "Done."
ls -lh "$ASSETS"/tui-installer.gif "$ASSETS"/cli-setup-scan.gif
