#!/usr/bin/env python3
"""
Convert plex2jellyfin ASCII art to PNG with transparent background.

Requirements:
    pip install pillow

Usage:
    python3 scripts/ascii_to_png.py [output_path]

Example:
    python3 scripts/ascii_to_png.py assets/plex2jellyfin-header.png

Note: This script is for generating the README header image.
      The ASCII art source of truth is internal/clitheme/header.txt
      (the same art the CLI embeds and prints).
"""

from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
import sys

# ASCII art — read from the CLI theme package so the two never drift
ASCII_ART = (
    Path(__file__).resolve().parent.parent / "internal/clitheme/header.txt"
).read_text().rstrip("\n")

# README / marketing wordmark — white on transparent for dark GitHub chrome
WORDMARK_COLOR = "#FFFFFF"


def create_ascii_png(output_path="assets/plex2jellyfin-header.png"):
    """Create PNG from ASCII art with transparent background."""

    lines = ASCII_ART.split("\n")
    max_chars = max(len(line) for line in lines)

    # Try common monospace font locations (Arch, Debian, macOS)
    font_size = 28
    font = None
    for font_name in [
        "/usr/share/fonts/TTF/DejaVuSansMono.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
        "/usr/share/fonts/TTF/LiberationMono-Regular.ttf",
        "/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
        "/System/Library/Fonts/Monaco.dfont",
    ]:
        try:
            font = ImageFont.truetype(font_name, font_size)
            break
        except OSError:
            continue
    if font is None:
        font = ImageFont.load_default()

    # Measure real glyph metrics instead of guessing
    char_width = font.getlength("█")
    ascent, descent = (font.getmetrics() if hasattr(font, "getmetrics") else (16, 4))
    char_height = ascent + descent

    # Image dimensions with padding
    pad = 20
    img_width = int(max_chars * char_width) + pad * 2
    img_height = int(len(lines) * char_height) + pad * 2

    # Create image with transparent background (RGBA)
    img = Image.new("RGBA", (img_width, img_height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Convert hex color to RGB tuple
    color = tuple(int(WORDMARK_COLOR.lstrip("#")[i : i + 2], 16) for i in (0, 2, 4))

    # Draw text line by line
    y = pad
    for line in lines:
        draw.text((pad, y), line, fill=color + (255,), font=font)
        y += char_height

    # Save the image
    img.save(output_path, "PNG")
    print(f"Created {output_path}")
    print(f"Dimensions: {img_width}x{img_height}")


if __name__ == "__main__":
    output = sys.argv[1] if len(sys.argv) > 1 else "assets/plex2jellyfin-header.png"
    create_ascii_png(output)
