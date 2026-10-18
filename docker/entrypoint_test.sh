#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
mkdir -p "$tmp_dir/bin"

cat >"$tmp_dir/bin/getent" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$tmp_dir/bin/mkdir" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$tmp_dir/bin/chown" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$tmp_dir/bin/su-exec" <<'EOF'
#!/bin/sh
if [ "$2" = "plex2jellyfin-daemon" ]; then
  trap 'exit 0' TERM
  printf '%s\n' "$*" >>"$ENTRYPOINT_LOG"
  while :; do sleep 1; done
fi
printf '%s\n' "$*" >>"$ENTRYPOINT_LOG"
EOF
chmod +x "$tmp_dir/bin/"*

env -u P2J_WEB_HOST -u P2J_WEB_ALLOW_UNAUTHENTICATED_REMOTE \
  PATH="$tmp_dir/bin:$PATH" ENTRYPOINT_LOG="$tmp_dir/default.log" \
  sh "$repo_dir/docker/entrypoint.sh"
grep -Fxq 'p2j plex2jellyfin-web' "$tmp_dir/default.log" || {
  printf '%s\n' 'default container startup must keep the web binary defaults' >&2
  exit 1
}

P2J_WEB_HOST=0.0.0.0 P2J_WEB_ALLOW_UNAUTHENTICATED_REMOTE=true \
  PATH="$tmp_dir/bin:$PATH" ENTRYPOINT_LOG="$tmp_dir/bootstrap.log" \
  sh "$repo_dir/docker/entrypoint.sh"
grep -Fxq 'p2j plex2jellyfin-web --host 0.0.0.0 --allow-unauthenticated-remote' \
  "$tmp_dir/bootstrap.log" || {
  printf '%s\n' 'bootstrap env must pass the remote-bind override' >&2
  exit 1
}
