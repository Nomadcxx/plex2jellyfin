import argparse
import tempfile
from pathlib import Path

try:
    from PIL import Image, ImageDraw, ImageFont
except ImportError as error:
    raise SystemExit("Pillow is required: install the python-pillow package") from error


ROOT = Path(__file__).resolve().parents[1]
# Full wordmark is white for docs/README chrome; compact P2J is brand yellow
# matching plexo.go ANSI (rgb 229,229,16).
ASSETS = (
    (ROOT / "docs-site/brand/plex2jellyfin.txt", ROOT / "docs-site/public/brand/plex2jellyfin-wordmark.png", (255, 255, 255)),
    (ROOT / "docs-site/brand/P2J.txt", ROOT / "docs-site/public/brand/p2j-mark.png", (229, 229, 16)),
)
FONT_CANDIDATES = (
    Path("/usr/share/fonts/TTF/DejaVuSansMono.ttf"),
    Path("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"),
)


def find_font() -> Path:
    for candidate in FONT_CANDIDATES:
        if candidate.is_file():
            return candidate
    raise FileNotFoundError("DejaVu Sans Mono was not found in a standard system font directory")


def render_ascii(source: Path, destination: Path, color: tuple[int, int, int] = (255, 255, 255)) -> None:
    text = source.read_text(encoding="utf-8").replace("\r\n", "\n").rstrip("\r\n")
    if not text.strip():
        raise ValueError(f"ASCII source is empty: {source}")

    font = ImageFont.truetype(str(find_font()), 32)
    probe = Image.new("RGBA", (1, 1))
    bounds = ImageDraw.Draw(probe).multiline_textbbox((0, 0), text, font=font, spacing=0)
    padding = 24
    size = (bounds[2] - bounds[0] + padding * 2, bounds[3] - bounds[1] + padding * 2)
    image = Image.new("RGBA", size, (0, 0, 0, 0))
    ImageDraw.Draw(image).multiline_text(
        (padding - bounds[0], padding - bounds[1]),
        text,
        font=font,
        fill=color + (255,),
        spacing=0,
    )
    destination.parent.mkdir(parents=True, exist_ok=True)
    image.save(destination, format="PNG", optimize=True)


def check_assets() -> None:
    with tempfile.TemporaryDirectory() as directory:
        temporary = Path(directory)
        for source, destination, color in ASSETS:
            candidate = temporary / destination.name
            render_ascii(source, candidate, color)
            if not destination.is_file() or candidate.read_bytes() != destination.read_bytes():
                raise SystemExit(f"Brand asset is stale: {destination.relative_to(ROOT)}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Render transparent documentation wordmarks")
    parser.add_argument("--check", action="store_true", help="verify committed PNGs are current")
    args = parser.parse_args()

    if args.check:
        check_assets()
        return
    for source, destination, color in ASSETS:
        render_ascii(source, destination, color)


if __name__ == "__main__":
    main()
