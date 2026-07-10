#!/bin/sh
set -e
systemctl daemon-reload || true
cat <<'EOF'
plex2jellyfin installed. The daemon will not run until it is configured:
  1. As your normal user:  plex2jellyfin config init
  2. Edit ~/.config/plex2jellyfin/config.toml (watch paths, libraries, *arr keys)
  3. Verify:               plex2jellyfin config test
  4. Point the services at your user's config:
       sudo systemctl edit plex2jellyfin-daemon
       sudo systemctl edit plex2jellyfin-web
     and add to each:
       [Service]
       Environment=SUDO_USER=<your username>
  5. sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
EOF
