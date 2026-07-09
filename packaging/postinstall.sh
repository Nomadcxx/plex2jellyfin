#!/bin/sh
set -e
systemctl daemon-reload || true
echo "plex2jellyfin installed."
echo "  1. Copy /usr/share/doc/plex2jellyfin/config.toml.example to ~/.config/plex2jellyfin/config.toml and edit it"
echo "  2. systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web"
