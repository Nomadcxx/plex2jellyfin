#!/usr/bin/env bash
# Record TUI + CLI showcase casts (asciinema) and render:
#   TUI  → high-res MP4 (agg GIF master → ffmpeg)
#   CLI  → high-res GIF (autoplay in README)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${SHOWCASE_OUT:-$ROOT/scripts/showcase/out}"
ASSETS="${SHOWCASE_ASSETS:-$ROOT/assets}"
AGG="${AGG:-$HOME/.cargo/bin/agg}"
# Wide terminal so the TUI wordmark + panel aren't cramped
COLS="${SHOWCASE_COLS:-140}"
ROWS="${SHOWCASE_ROWS:-42}"
FONT_SIZE="${SHOWCASE_FONT_SIZE:-28}"
FPS="${SHOWCASE_FPS:-60}"
SPEED="${SHOWCASE_SPEED:-1.0}"
IDLE="${SHOWCASE_IDLE:-2.0}"
export PATH="$HOME/.cargo/bin:$PATH"
export SHOWCASE_COLS="$COLS" SHOWCASE_ROWS="$ROWS"
export PLEX2JELLYFIN_TEST_NO_ESCALATE=1

mkdir -p "$OUT" "$ASSETS"
command -v asciinema >/dev/null
command -v ffmpeg >/dev/null
if [[ ! -x "$AGG" ]]; then
  AGG="$(command -v agg)"
fi
[[ -x "$AGG" ]]

render_gif() {
  local cast="$1" gif="$2"
  "$AGG" \
    --theme github-dark \
    --font-size "$FONT_SIZE" \
    --line-height 1.35 \
    --speed "$SPEED" \
    --idle-time-limit "$IDLE" \
    --fps-cap "$FPS" \
    --font-antialiasing 8 \
    "$cast" "$gif"
  ls -lh "$gif"
}

gif_to_mp4() {
  local gif="$1" mp4="$2"
  # Even dimensions required for yuv420p; keep native agg resolution.
  ffmpeg -y -i "$gif" \
    -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2,fps=${FPS}" \
    -c:v libx264 -pix_fmt yuv420p -crf 18 -preset medium \
    -movflags +faststart \
    "$mp4"
  ls -lh "$mp4"
}

echo "== TUI installer (MP4) =="
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
render_gif "$OUT/tui-installer.cast" "$OUT/tui-installer-master.gif"
gif_to_mp4 "$OUT/tui-installer-master.gif" "$ASSETS/tui-installer.mp4"
# Poster for README <video>
ffmpeg -y -ss 2 -i "$ASSETS/tui-installer.mp4" -frames:v 1 -update 1 \
  "$ASSETS/tui-installer-poster.png" 2>/dev/null || \
ffmpeg -y -i "$ASSETS/tui-installer.mp4" -frames:v 1 -update 1 \
  "$ASSETS/tui-installer-poster.png"
rm -f "$ASSETS/tui-installer.gif"

echo "== CLI setup + scan (GIF) =="
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
ls -lh "$ASSETS"/tui-installer.mp4 "$ASSETS"/tui-installer-poster.png "$ASSETS"/cli-setup-scan.gif
