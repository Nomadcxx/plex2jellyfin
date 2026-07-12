#!/usr/bin/env bash
# Fresh clone + build + install of plex2jellyfin binaries and systemd units.
# Does NOT run setup and does NOT start services — run `plex2jellyfin setup` after.
#
# Run as your normal user (not sudo). The script will prompt for sudo only
# when installing into /usr/bin and /etc/systemd.
set -euo pipefail

REPO_URL="${PLEX2JELLYFIN_REPO:-https://github.com/Nomadcxx/plex2jellyfin.git}"
REPO_REF="${PLEX2JELLYFIN_REF:-main}"
PREFIX="${PLEX2JELLYFIN_PREFIX:-/usr/bin}"
WORKDIR="${PLEX2JELLYFIN_WORKDIR:-}"

die() { echo "error: $*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

# If someone ran `sudo ./script`, drop back to the real user for clone/build.
# Building npm/go as root breaks PATH to web/node_modules/.bin and pollutes caches.
if [[ "$(id -u)" -eq 0 ]]; then
  [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != root ]] || die "do not run as root; run: bash $0"
  echo "re-executing as ${SUDO_USER} (build must not run as root)..."
  exec sudo -u "$SUDO_USER" -H -- "$0" "$@"
fi

need git
need go
need npm
need sudo

REAL_USER="$USER"
echo "Plex2Jellyfin fresh build/install"
echo "  repo:   $REPO_URL @$REPO_REF"
echo "  user:   $REAL_USER  (SUDO_USER for systemd)"
echo "  prefix: $PREFIX"
echo

if [[ -z "$WORKDIR" ]]; then
  WORKDIR="$(mktemp -d /tmp/plex2jellyfin-build-XXXXXX)"
  CLEANUP_WORKDIR=1
else
  CLEANUP_WORKDIR=0
  mkdir -p "$WORKDIR"
fi

cleanup() {
  if [[ "${CLEANUP_WORKDIR}" -eq 1 && -d "${WORKDIR}" ]]; then
    rm -rf "${WORKDIR}"
  fi
}
trap cleanup EXIT

echo "==> cloning"
git clone --depth 1 --branch "$REPO_REF" "$REPO_URL" "$WORKDIR/src"
cd "$WORKDIR/src"

echo "==> frontend deps + build (embedded into plex2jellyfin-web)"
# Fresh clone has no node_modules; Makefile frontend assumes they exist.
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
    printf '\n# injected by fresh-build-install.sh\n[Service]\nEnvironment=SUDO_USER=%s\n' "$REAL_USER"
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
echo "Services are installed but not enabled/started."
echo "Next:"
echo "  plex2jellyfin setup"
echo "Then (if setup does not enable them):"
echo "  sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web"
echo "  open http://localhost:5522/"
