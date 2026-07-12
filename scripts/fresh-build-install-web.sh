#!/usr/bin/env bash
# Fresh clone + build + install, then start the web UI for browser setup.
#
#   bash <(curl -fsSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/scripts/fresh-build-install-web.sh)
#
# Run as your normal user (not sudo). The script prompts for sudo only when
# installing into /usr/bin and /etc/systemd.
set -euo pipefail

REPO_URL="${PLEX2JELLYFIN_REPO:-https://github.com/Nomadcxx/plex2jellyfin.git}"
REPO_REF="${PLEX2JELLYFIN_REF:-main}"
PREFIX="${PLEX2JELLYFIN_PREFIX:-/usr/bin}"
WORKDIR="${PLEX2JELLYFIN_WORKDIR:-}"

die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

if [[ "$(id -u)" -eq 0 ]]; then
  [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != root ]] || die "do not run as root; run: bash $0"
  echo "re-executing as ${SUDO_USER} (build must not run as root)..."
  exec sudo -u "$SUDO_USER" -H -- "$0" "$@"
fi

need git
need go
need npm
need sudo

export PLEX2JELLYFIN_SETUP_MODE=web
export PLEX2JELLYFIN_PREFIX="$PREFIX"
export PLEX2JELLYFIN_REAL_USER="$USER"

echo "Plex2Jellyfin fresh build/install (web setup)"
echo "  repo:   $REPO_URL @$REPO_REF"
echo "  user:   $USER  (SUDO_USER for systemd)"
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
bash scripts/fresh-build-from-tree.sh
