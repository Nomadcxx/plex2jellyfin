#!/usr/bin/env bash
set -euo pipefail

WORKSPACE="/home/nomadx/Documents/jellywatch"
PROMPT="${HOME}/.config/jellywatch/reports/latest/agent-prompt.md"
MESSAGE="JellyWatch postmortem bundle is ready.

cd ${WORKSPACE}

Ask Codex:
Read ${PROMPT} and analyze what worked, what failed, and what should be patched next.
"

if command -v kitty >/dev/null 2>&1; then
  kitty --working-directory "${WORKSPACE}" sh -lc "printf '%s\n' '${MESSAGE}'; exec zsh" &
elif command -v alacritty >/dev/null 2>&1; then
  alacritty --working-directory "${WORKSPACE}" -e sh -lc "printf '%s\n' '${MESSAGE}'; exec zsh" &
elif command -v gnome-terminal >/dev/null 2>&1; then
  gnome-terminal --working-directory="${WORKSPACE}" -- sh -lc "printf '%s\n' '${MESSAGE}'; exec zsh" &
else
  printf '%s\n' "${MESSAGE}"
fi
