import tempfile
import unittest
from pathlib import Path

from PIL import Image

from scripts.render_docs_brand import render_ascii


class RenderAsciiTest(unittest.TestCase):
    def test_rejects_empty_source(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "empty.txt"
            source.write_text("\n", encoding="utf-8")

            with self.assertRaisesRegex(ValueError, "empty"):
                render_ascii(source, Path(directory) / "empty.png")

    def test_renders_transparent_rgba_at_source_proportions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            full_source = root / "full.txt"
            compact_source = root / "compact.txt"
            full_output = root / "full.png"
            compact_output = root / "compact.png"
            full_source.write_text("████████████████████\n██  FULL BRAND  ██\n", encoding="utf-8")
            compact_source.write_text("██████\n P2J\n", encoding="utf-8")

            render_ascii(full_source, full_output)
            render_ascii(compact_source, compact_output)

            with Image.open(full_output) as full, Image.open(compact_output) as compact:
                self.assertEqual(full.mode, "RGBA")
                self.assertEqual(compact.mode, "RGBA")
                self.assertEqual(full.getchannel("A").getextrema()[0], 0)
                self.assertGreater(full.getchannel("A").getextrema()[1], 0)
                self.assertGreater(full.width, compact.width * 2)


if __name__ == "__main__":
    unittest.main()
