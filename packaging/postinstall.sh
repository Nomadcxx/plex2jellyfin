#!/bin/sh
set -e
systemctl daemon-reload || true
echo "plex2jellyfin installed."
echo "  1. Run: mkdir -p ~/.config/plex2jellyfin && cp /usr/share/doc/plex2jellyfin/config.toml.example ~/.config/plex2jellyfin/config.toml"
echo "  2. systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web"
