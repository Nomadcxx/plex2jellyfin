#!/usr/bin/env python3
"""Drive plex2jellyfin setup (abort before write) then a short library scan."""
from __future__ import annotations

import os
import pathlib
import sys
import textwrap
import time

import pexpect

CLI = os.environ.get("SHOWCASE_CLI", "plex2jellyfin")
COLS = int(os.environ.get("SHOWCASE_COLS", "100"))
ROWS = int(os.environ.get("SHOWCASE_ROWS", "32"))
DEMO = pathlib.Path(os.environ.get("SHOWCASE_HOME", "/tmp/p2j"))


def pause(s: float = 0.35) -> None:
    time.sleep(s)


def answer(child: pexpect.spawn, label: str, value: str, timeout: float = 25) -> None:
    child.expect(label, timeout=timeout)
    pause(0.55)
    if value:
        for ch in value:
            child.send(ch)
            time.sleep(0.045)
        pause(0.2)
    child.send("\r")
    pause(0.55)


def prepare_fixture() -> None:
    watch_tv = DEMO / "downloads" / "tv"
    watch_movies = DEMO / "downloads" / "movies"
    lib_tv = DEMO / "media" / "TV" / "Showcase (2024)" / "Season 01"
    lib_movies = DEMO / "media" / "Movies" / "Demo Movie (2024)"
    for d in (watch_tv, watch_movies, lib_tv, lib_movies):
        d.mkdir(parents=True, exist_ok=True)
    (lib_tv / "Showcase (2024) S01E01.mkv").write_bytes(b"\x00" * 64)
    (lib_tv / "Showcase (2024) S01E02.mkv").write_bytes(b"\x00" * 64)
    (lib_movies / "Demo Movie (2024).mkv").write_bytes(b"\x00" * 64)
    (watch_tv / "Incoming.Show.S01E03.mkv").write_bytes(b"\x00" * 32)


def write_scan_config() -> None:
    cfg_dir = DEMO / ".config" / "plex2jellyfin"
    cfg_dir.mkdir(parents=True, exist_ok=True)
    cfg = textwrap.dedent(
        f"""
        [watch]
        tv = ["{DEMO / 'downloads' / 'tv'}"]
        movies = ["{DEMO / 'downloads' / 'movies'}"]

        [libraries]
        tv = ["{DEMO / 'media' / 'TV'}"]
        movies = ["{DEMO / 'media' / 'Movies'}"]

        [daemon]
        enabled = false
        scan_frequency = "5m"

        [options]
        dry_run = true

        [setup]
        completed = true
        version = 1
        """
    ).lstrip()
    (cfg_dir / "config.toml").write_text(cfg)


def main() -> int:
    prepare_fixture()
    env = os.environ.copy()
    env.setdefault("TERM", "xterm-256color")
    env.setdefault("COLORTERM", "truecolor")
    env["HOME"] = str(DEMO)
    env["XDG_CONFIG_HOME"] = str(DEMO / ".config")
    env["PLEX2JELLYFIN_TEST_NO_ESCALATE"] = "1"
    env.pop("SUDO_USER", None)

    cfg = DEMO / ".config" / "plex2jellyfin" / "config.toml"
    if cfg.exists():
        cfg.unlink()

    child = pexpect.spawn(
        CLI,
        ["setup"],
        encoding="utf-8",
        timeout=60,
        dimensions=(ROWS, COLS),
        env=env,
    )
    # Forward child pty → this process stdout so asciinema can record it.
    child.logfile_read = sys.stdout

    answer(child, r"TV incoming paths", str(DEMO / "downloads" / "tv"))
    answer(child, r"TV library paths", str(DEMO / "media" / "TV"))
    answer(child, r"Movie incoming paths", str(DEMO / "downloads" / "movies"))
    answer(child, r"Movie library paths", str(DEMO / "media" / "Movies"))

    answer(child, r"Connect Sonarr", "n")
    answer(child, r"Connect Radarr", "n")
    answer(child, r"Connect Jellyfin", "n")
    answer(child, r"Use a local Ollama", "n")

    answer(child, r"Library scan frequency", "")
    answer(child, r"Move files", "")
    answer(child, r"Verify checksums", "n")

    answer(child, r"Owner username", "")
    answer(child, r"Owner group", "")
    answer(child, r"File mode", "")
    answer(child, r"Directory mode", "")

    answer(child, r"Write this configuration", "n")
    pause(1.0)
    try:
        child.expect(pexpect.EOF, timeout=10)
    except pexpect.TIMEOUT:
        child.sendcontrol("c")
        child.close(force=True)

    pause(0.6)
    write_scan_config()
    # Visible separator in the cast (stdout of the driver is the recorded pty)
    sys.stdout.write("\n# Library scan\n\n")
    sys.stdout.flush()
    pause(0.4)

    scan = pexpect.spawn(
        CLI,
        ["scan"],
        encoding="utf-8",
        timeout=120,
        dimensions=(ROWS, COLS),
        env=env,
    )
    scan.logfile_read = sys.stdout
    try:
        scan.expect(pexpect.EOF, timeout=120)
    except pexpect.TIMEOUT:
        scan.sendcontrol("c")
        scan.close(force=True)
    pause(0.8)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
