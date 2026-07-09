#!/bin/sh
set -e
systemctl stop plex2jellyfin-web plex2jellyfin-daemon 2>/dev/null || true
systemctl disable plex2jellyfin-web plex2jellyfin-daemon 2>/dev/null || true
