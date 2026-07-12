#!/usr/bin/env bash
# Build + install from an already-checked-out plex2jellyfin tree (cwd = repo root).
# Set PLEX2JELLYFIN_SETUP_MODE=cli|web (default cli).
set -euo pipefail

MODE="${PLEX2JELLYFIN_SETUP_MODE:-cli}"
PREFIX="${PLEX2JELLYFIN_PREFIX:-/usr/bin}"
REAL_USER="${PLEX2JELLYFIN_REAL_USER:-$USER}"

die() { echo "error: $*" >&2; exit 1; }
[[ -f go.mod && -d cmd/plex2jellyfin ]] || die "run from a plex2jellyfin checkout (go.mod missing)"
[[ "$MODE" == "cli" || "$MODE" == "web" ]] || die "PLEX2JELLYFIN_SETUP_MODE must be cli or web"

echo "==> frontend deps + build (embedded into plex2jellyfin-web)"
(cd web && npm install && npm run build)
rm -rf embedded/frontend
cp -r web/out embedded/frontend

echo "==> binaries"
mkdir -p bin
go build -o bin/plex2jellyfin ./cmd/plex2jellyfin
go build -o bin/plex2jellyfin-daemon ./cmd/plex2jellyfin-daemon
go build -o bin/plex2jellyfin-web ./cmd/plex2jellyfin-web
go build -o bin/plex2jellyfin-installer ./cmd/installer

echo "==> installing to $PREFIX (sudo)"
sudo install -m 755 bin/plex2jellyfin bin/plex2jellyfin-daemon bin/plex2jellyfin-web bin/plex2jellyfin-installer "$PREFIX/"

echo "==> systemd units"
for unit in plex2jellyfin-daemon plex2jellyfin-web; do
  tmp="$(mktemp)"
  {
    cat "systemd/${unit}.service"
    printf '\n# injected by fresh-build-install\n[Service]\nEnvironment=SUDO_USER=%s\n' "$REAL_USER"
  } >"$tmp"
  sudo install -m 644 "$tmp" "/etc/systemd/system/${unit}.service"
  rm -f "$tmp"
done
sudo systemctl daemon-reload

echo
echo "Installed:"
"$PREFIX/plex2jellyfin" version || true
ls -la "$PREFIX"/plex2jellyfin "$PREFIX"/plex2jellyfin-daemon "$PREFIX"/plex2jellyfin-web "$PREFIX"/plex2jellyfin-installer
echo

if [[ "$MODE" == "web" ]]; then
  echo "==> enabling web UI (setup wizard in the browser)"
  sudo systemctl enable --now plex2jellyfin-web
  # Daemon stays stopped until the wizard writes config; unit is installed for ApplySetup.
  host_ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  echo
  echo "Open the setup wizard:"
  echo "  http://127.0.0.1:5522/"
  if [[ -n "${host_ip:-}" ]]; then
    echo "  http://${host_ip}:5522/"
  fi
  echo
  echo "The wizard saves config, enables the daemon, and runs the initial library scan."
  echo "Status: systemctl status plex2jellyfin-web plex2jellyfin-daemon"
else
  echo "Services are installed but not enabled/started."
  echo "Next:"
  echo "  plex2jellyfin setup"
  echo "Setup enables and starts plex2jellyfin-daemon and (optionally) plex2jellyfin-web."
  echo "  open http://localhost:5522/   # after setup starts the web unit"
fi
