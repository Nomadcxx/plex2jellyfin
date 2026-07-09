#!/usr/bin/env python3
"""SABnzbd TV post-processing deobfuscation helper.

Runs after SABnzbd's built-in deobfuscation. It only does low-risk renames:
PAR2 hash-confirmed names, plus a single obfuscated file when the job name
already contains an explicit SxxEyy marker.
"""

from __future__ import annotations

import hashlib
import os
import re
import sys
from typing import Callable

VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".m4v", ".mov", ".wmv", ".flv", ".ts", ".webm"}
TV_CATEGORIES = {"tv", "series", "shows", "sonarr"}

SXXEYY_RE = re.compile(r"(?i)s\d{1,2}e\d{1,3}")
X_FORMAT_RE = re.compile(r"(?i)\b\d{1,2}x\d{1,3}\b")
DATE_RE = re.compile(r"\b(?:19|20)\d{2}[-.]\d{2}[-.]\d{2}\b")
YEAR_RE = re.compile(r"(?:^|[ ._(])(?:19|20)\d{2}(?:$|[ ._)])")
HEX32_RE = re.compile(r"^[a-fA-F0-9]{32,}$")
HEX_DOTS_RE = re.compile(r"^[a-fA-F0-9.]{40,}$")
BRACKET_HEX_RE = re.compile(r"[a-fA-F0-9]{30,}")
EPISODE_MARKER_RE = re.compile(r"(?i)(s\d{1,2}e\d{1,3}|\b\d{1,2}x\d{1,3}\b)")

COMMON_WORDS = {
    "the",
    "and",
    "show",
    "episode",
    "season",
    "part",
    "disc",
    "blackadder",
    "back",
    "forth",
    "henry",
    "daily",
    "rick",
    "morty",
    "matrix",
    "deadwood",
    "from",
    "rome",
}


def ensure_sabnzbd_import_path() -> None:
    sab_paths = [
        os.environ.get("SAB_PROGRAM_DIR", ""),
        "/usr/lib/sabnzbd",
    ]
    for path in sab_paths:
        if path and os.path.isdir(path) and path not in sys.path:
            sys.path.insert(0, path)


def log(message: str) -> None:
	print(f"plex2jellyfin_deobfuscate: {message}", flush=True)


def is_obfuscated(filename: str) -> bool:
    stem = os.path.splitext(os.path.basename(filename))[0].strip()
    if not stem:
        return False
    if SXXEYY_RE.search(stem) or X_FORMAT_RE.search(stem) or DATE_RE.search(stem) or YEAR_RE.search(stem):
        return False
    if stem.upper().startswith("DISC_"):
        return False
    if has_common_word(stem):
        return False
    if HEX32_RE.fullmatch(stem) or HEX_DOTS_RE.fullmatch(stem):
        return True
    if stem.lower().startswith("abc.xyz"):
        return True
    if BRACKET_HEX_RE.search(stem) and len(re.findall(r"\[[^\]]+\]", stem)) >= 2:
        return True
    if is_random_alnum(stem, min_len=16, max_vowel_ratio=0.35):
        return True
    if is_medium_random_alnum(stem):
        return True
    if is_short_random_alnum(stem):
        return True
    return False


def has_common_word(value: str) -> bool:
    tokens = re.findall(r"[a-zA-Z]{3,}", value.lower())
    return any(token in COMMON_WORDS for token in tokens)


def vowel_ratio(value: str) -> float:
    letters = [ch.lower() for ch in value if ch.isalpha()]
    if not letters:
        return 0.0
    return sum(1 for ch in letters if ch in "aeiou") / len(letters)


def unique_ratio(value: str) -> float:
    return len(set(value)) / len(value) if value else 0.0


def is_random_alnum(value: str, min_len: int, max_vowel_ratio: float) -> bool:
    if len(value) < min_len or not value.isalnum():
        return False
    has_upper = any(ch.isupper() for ch in value)
    has_lower = any(ch.islower() for ch in value)
    has_digit = any(ch.isdigit() for ch in value)
    has_alpha = has_upper or has_lower
    return has_alpha and (has_digit or (has_upper and has_lower)) and vowel_ratio(value) <= max_vowel_ratio and unique_ratio(value) > 0.4


def is_medium_random_alnum(value: str) -> bool:
    # ponytail: separator-free tokens with digits are treated as random; if this
    # catches real titles later, add the title word to COMMON_WORDS.
    if not (9 <= len(value) <= 80) or not value.isalnum():
        return False
    has_upper = any(ch.isupper() for ch in value)
    has_lower = any(ch.islower() for ch in value)
    has_digit = any(ch.isdigit() for ch in value)
    return has_upper and has_lower and has_digit and unique_ratio(value) > 0.45


def is_short_random_alnum(value: str) -> bool:
    if not (4 <= len(value) <= 8) or not value.isalnum():
        return False
    has_upper = any(ch.isupper() for ch in value)
    has_lower = any(ch.islower() for ch in value)
    has_digit = any(ch.isdigit() for ch in value)
    if len(value) <= 5 and has_upper and has_digit:
        return True
    if 4 <= len(value) <= 5 and value[:2].isupper():
        return True
    if 4 <= len(value) <= 6 and value.isupper() and vowel_ratio(value) <= 0.25:
        return True
    return has_upper and has_digit and (has_lower or len(value) <= 5) and vowel_ratio(value) <= 0.25


def video_files(directory: str) -> list[str]:
    out: list[str] = []
    for root, dirs, files in os.walk(directory):
        dirs[:] = [d for d in dirs if not d.startswith("__")]
        for name in files:
            if os.path.splitext(name)[1].lower() in VIDEO_EXTS:
                out.append(os.path.join(root, name))
    return sorted(out)


def par2_hash_table(directory: str) -> dict[bytes, str]:
    ensure_sabnzbd_import_path()
    try:
        from sabnzbd.par2file import is_par2_file, parse_par2_file
    except Exception as exc:
        log(f"SABnzbd PAR2 parser unavailable; skipping PAR2 recovery ({exc})")
        return {}

    table: dict[bytes, str] = {}
    for root, _dirs, files in os.walk(directory):
        for name in files:
            if not name.lower().endswith(".par2"):
                continue
            path = os.path.join(root, name)
            try:
                if is_par2_file(path):
                    parse_par2_file(path, table)
            except Exception as exc:
                log(f"PAR2 parse failed for {path}: {exc}")
    return table


def recover_from_par2(directory: str, loader: Callable[[str], dict[bytes, str]] = par2_hash_table) -> list[tuple[str, str]]:
    table = loader(directory)
    if not table:
        return []

    renamed: list[tuple[str, str]] = []
    for path in video_files(directory):
        name = os.path.basename(path)
        if not is_obfuscated(name):
            continue
        try:
            with open(path, "rb") as handle:
                digest = hashlib.md5(handle.read(16384)).digest()
        except OSError as exc:
            log(f"could not read {path}: {exc}")
            continue

        target_name = table.get(digest)
        if not target_name or target_name == name:
            continue
        target = os.path.join(os.path.dirname(path), os.path.basename(target_name))
        if os.path.exists(target):
            log(f"PAR2 target exists; skipping {path} -> {target}")
            continue
        try:
            os.rename(path, target)
            renamed.append((path, target))
        except OSError as exc:
            log(f"rename failed {path} -> {target}: {exc}")
    return renamed


def rename_single_episode(directory: str, job_name: str) -> list[tuple[str, str]]:
    match = EPISODE_MARKER_RE.search(job_name)
    if not match:
        return []
    remaining = [path for path in video_files(directory) if is_obfuscated(path)]
    if len(remaining) != 1:
        return []

    source = remaining[0]
    stem = sanitize_job_stem(job_name)
    if not stem:
        return []
    target = os.path.join(os.path.dirname(source), stem + os.path.splitext(source)[1].lower())
    if os.path.exists(target):
        log(f"single-episode target exists; skipping {source} -> {target}")
        return []
    try:
        os.rename(source, target)
        return [(source, target)]
    except OSError as exc:
        log(f"single-episode rename failed {source} -> {target}: {exc}")
        return []


def sanitize_job_stem(job_name: str) -> str:
    base = os.path.basename(job_name)
    for suffix in (".nzb", ".gz"):
        if base.lower().endswith(suffix):
            base = base[: -len(suffix)]
    base = re.sub(r"[\\/]+", ".", base)
    base = re.sub(r"\s+", ".", base.strip())
    return base.strip(" ._-")


def run(argv: list[str]) -> int:
    try:
        if len(argv) < 8:
            log("usage: script <dir> <nzbname> <jobname> <report#> <category> <group> <status> [failure_url]")
            return 0

        directory = argv[1]
        job_name = argv[3] or argv[2]
        category = (argv[5] or "").strip().lower()
        status = argv[7]

        if category not in TV_CATEGORIES:
            log(f"skipping category={category or '<empty>'}")
            return 0
        if status not in ("0", "SUCCESS", "Completed", "completed"):
            log(f"skipping non-success status={status}")
            return 0
        if not os.path.isdir(directory):
            log(f"complete dir is not a directory: {directory}")
            return 0

        before = [path for path in video_files(directory) if is_obfuscated(path)]
        if not before:
            log("no obfuscated videos found")
            return 0

        par2_renamed = recover_from_par2(directory)
        single_renamed = rename_single_episode(directory, job_name)
        after = [path for path in video_files(directory) if is_obfuscated(path)]
        log(
            "summary "
            f"initial_obfuscated={len(before)} "
            f"par2_renamed={len(par2_renamed)} "
            f"single_episode_renamed={len(single_renamed)} "
            f"remaining_obfuscated={len(after)}"
        )
    except Exception as exc:
        log(f"unexpected error ignored: {exc}")
    return 0


if __name__ == "__main__":
    sys.exit(run(sys.argv))
