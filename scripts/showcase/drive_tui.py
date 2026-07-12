#!/usr/bin/env python3
"""Drive plex2jellyfin-installer through welcome → paths → skips → quit before install."""
from __future__ import annotations

import os
import sys
import time

import pexpect

INSTALLER = os.environ.get("SHOWCASE_INSTALLER", "plex2jellyfin-installer")
COLS = int(os.environ.get("SHOWCASE_COLS", "100"))
ROWS = int(os.environ.get("SHOWCASE_ROWS", "32"))


def pause(s: float = 0.4) -> None:
    time.sleep(s)


def type_slow(child: pexpect.spawn, text: str, delay: float = 0.055) -> None:
    for ch in text:
        child.send(ch)
        time.sleep(delay)


def wait_screen(child: pexpect.spawn, pattern: str, timeout: float = 20) -> None:
    child.expect(pattern, timeout=timeout)
    pause(0.7)


def main() -> int:
    env = os.environ.copy()
    env.setdefault("TERM", "xterm-256color")
    env.setdefault("COLORTERM", "truecolor")
    demo = env.get("SHOWCASE_HOME", "/tmp/p2j-tui-demo")
    os.makedirs(os.path.join(demo, ".config"), exist_ok=True)
    env["HOME"] = demo
    env["XDG_CONFIG_HOME"] = os.path.join(demo, ".config")
    env["PLEX2JELLYFIN_TEST_NO_ESCALATE"] = "1"
    env.pop("SUDO_USER", None)

    child = pexpect.spawn(
        INSTALLER,
        encoding="utf-8",
        timeout=45,
        dimensions=(ROWS, COLS),
        env=env,
    )
    # Forward child pty → this process stdout so asciinema can record it.
    child.logfile_read = sys.stdout

    wait_screen(child, "Install Plex2Jellyfin")
    pause(1.0)
    child.send("\r")

    wait_screen(child, "Watch Folders")
    pause(0.5)

    # TV watch, Movies watch, TV library, Movies library (no spaces — safer for demos)
    for path in (
        "/downloads/tv",
        "/downloads/movies",
        "/media/TV",
        "/media/Movies",
    ):
        type_slow(child, path)
        pause(0.35)
        child.send("\t")
        pause(0.35)

    pause(0.6)
    child.send("\r")

    for title in (
        "Sonarr",
        "Radarr",
        "Jellyfin",
        "Ollama|AI /",
    ):
        wait_screen(child, title)
        pause(0.5)
        child.send("s")
        pause(0.6)

    for title in ("Permissions|ownership|Group", "Service|systemd|Daemon", "Web|port|5522"):
        wait_screen(child, title)
        pause(0.7)
        child.send("\r")
        pause(0.5)

    wait_screen(child, "Confirm")
    pause(1.5)
    # Back out to welcome, then quit — do not install
    for _ in range(12):
        child.send("\x1b")
        pause(0.2)
    pause(0.4)
    child.send("q")
    pause(1.0)

    try:
        child.expect(pexpect.EOF, timeout=8)
    except pexpect.TIMEOUT:
        child.sendcontrol("c")
        pause(0.3)
        child.close(force=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
