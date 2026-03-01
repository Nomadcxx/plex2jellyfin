#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="${1:-$HOME/.local/share/jellyfin/plugins/JellyWatch}"
JELLYFIN_SERVICE="${JELLYFIN_SERVICE:-jellyfin}"

echo "Installing JellyWatch Plugin..."
echo "Target directory: $PLUGIN_DIR"

mkdir -p "$PLUGIN_DIR"

ARTIFACT="$(ls -t "$SCRIPT_DIR"/artifacts/jellywatch_*.zip 2>/dev/null | head -1 || true)"
if [[ -z "$ARTIFACT" ]]; then
	echo "Error: No plugin artifact found. Run $SCRIPT_DIR/build.sh first." >&2
	exit 1
fi

echo "Installing from: $ARTIFACT"
unzip -o "$ARTIFACT" -d "$PLUGIN_DIR" >/dev/null

echo ""
echo "Installation complete."
echo "Plugin files written to: $PLUGIN_DIR"
echo ""
echo "Next steps:"
echo "  1. Restart Jellyfin: sudo systemctl restart $JELLYFIN_SERVICE"
echo "  2. Open Jellyfin Dashboard -> Plugins -> JellyWatch"
echo "  3. Configure JellyWatch URL + Shared Secret"
