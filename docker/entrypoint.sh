#!/bin/sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if ! getent group p2j >/dev/null 2>&1; then
    addgroup -g "$PGID" p2j
fi
if ! getent passwd p2j >/dev/null 2>&1; then
    adduser -D -H -u "$PUID" -G p2j -h /config p2j
fi

mkdir -p /config
chown -R "$PUID:$PGID" /config

# Allow `docker run <image> <command> ...` to run a one-off command
# (e.g. `plex2jellyfin version`) instead of starting the daemon+web services.
if [ "$#" -gt 0 ]; then
    exec su-exec p2j "$@"
fi

su-exec p2j plex2jellyfin-daemon &
DAEMON_PID=$!

trap 'kill $DAEMON_PID 2>/dev/null' TERM INT
exec su-exec p2j plex2jellyfin-web
