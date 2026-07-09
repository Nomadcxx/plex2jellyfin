#!/usr/bin/env python3

import contextlib
import hashlib
import io
import os
import shutil
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(__file__))

from plex2jellyfin_deobfuscate import (  # noqa: E402
    ensure_sabnzbd_import_path,
    is_obfuscated,
    recover_from_par2,
    rename_single_episode,
    run,
    video_files,
)


class TestObfuscationDetection(unittest.TestCase):
    def test_detects_residual_obfuscated_patterns(self):
        for name in (
            "f14025510b144928a1c237cf902d98d7.mkv",
            "C5Ejq98Z9xRaKsAv92zuprqNaWkfiusA649.mkv",
            "JfkkCQvrfJvtrrhYQrv.mkv",
            "BW66.mkv",
            "AcX5.mkv",
            "h40M0EsPdAwG.mkv",
            "AUoZKFWH0aq.mkv",
            "RPN1oCFGNZFY.mkv",
            "OpEiAo6OeEceqjREbabYvAb6a9d7oGGR5.mkv",
            "HIep.mkv",
            "CRTJ.mkv",
            "ES3cr43WAPjs32.mkv",
            "abc.xyz.a4c567edbcbf27.mkv",
        ):
            with self.subTest(name=name):
                self.assertTrue(is_obfuscated(name))

    def test_detects_known_sab_unpack_tokens_from_history(self):
        for name in (
            "JTE0HnigOeQicSO6vVHCA3.mkv",
            "ZPiBWz966TH69O8YlfGWuo.mkv",
            "nNCdGmdpEsKWEylAALoSqaftk8tJjkLM.mkv",
            "pZgRawmMZdJ9OeiciGEuCwHOHCCB6FQj.mkv",
            "f56b710e65f94e7f9fa5ad584dddab73.mkv",
            "ET2W01mzIOu5BDitDjxZat48a2cmD4nF.mkv",
            "S3lgS5jM5fIspIAuJ6MFGO4xGR4c8TGQ.mkv",
            "5wJzv22iw32wGejXaVBeCNKFjc9az2Rt.mkv",
            "XfsGAMrTh3ZtCiWI6BQYWyuILF9rIAgd.mkv",
        ):
            with self.subTest(name=name):
                self.assertTrue(is_obfuscated(name))

    def test_preserves_named_media(self):
        for name in (
            "Show.Name.S03E01.1080p.WEB-DL.mkv",
            "Rick.and.Morty.S09E05.1080p.MAX.WEB-DL.mkv",
            "The.Matrix.1999.1080p.BluRay.mkv",
            "Blackadder Back And Forth.mkv",
            "DISC_1.mkv",
            "Henry.VIII.mkv",
            "Deadwood.mkv",
            "Rome.mkv",
            "From.mkv",
        ):
            with self.subTest(name=name):
                self.assertFalse(is_obfuscated(name))

    def test_preserves_parseable_obfuscated_release_names_from_history(self):
        for name in (
            "Euphoria.US-S01E04-Shook.Ones.PtII-WEBDL-1080p-x264-EAC3-5.1-Proper-AsmoFuscated.mkv",
            "The.Middle.S04E11.720p.HDTV.X264.1-DIMENSION-Obfuscated.mkv",
            "The Middle S02E13 720p HDTV x264-ORENJI-Obfuscated.mkv",
            "Gary.And.His.Demons.S01E14.The.Seven.1080p.WEB.x264.1-PLUTONiUM-Obfuscated.mkv",
            "Dracula.2020.S01.D02.1080p.Blu-ray.GBR.AVC.DTS-HD.MA.5.1-NOGRP-Obfuscated.iso",
        ):
            with self.subTest(name=name):
                self.assertFalse(is_obfuscated(name))


class TestPar2Recovery(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp(prefix="jw-sab-")

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def write_video(self, name, content):
        path = os.path.join(self.tmpdir, name)
        with open(path, "wb") as handle:
            handle.write(content)
        return path

    def test_renames_every_hash_matched_obfuscated_video(self):
        first = b"a" * 16384 + b"tail"
        second = b"b" * 16384 + b"tail"
        self.write_video("BW66.mkv", first)
        self.write_video("AcX5.mkv", second)

        hash_table = {
            hashlib.md5(first[:16384]).digest(): "Show.Name.S03E01.mkv",
            hashlib.md5(second[:16384]).digest(): "Show.Name.S03E02.mkv",
        }

        renamed = recover_from_par2(self.tmpdir, lambda _directory: hash_table)

        self.assertEqual(len(renamed), 2)
        self.assertTrue(os.path.exists(os.path.join(self.tmpdir, "Show.Name.S03E01.mkv")))
        self.assertTrue(os.path.exists(os.path.join(self.tmpdir, "Show.Name.S03E02.mkv")))

    def test_does_not_rename_named_or_duplicate_hash_targets(self):
        content = b"a" * 16384
        self.write_video("Show.Name.S03E01.mkv", content)
        self.write_video("BW66.mkv", content)

        hash_table = {
            hashlib.md5(content).digest(): "Show.Name.S03E01.mkv",
        }

        renamed = recover_from_par2(self.tmpdir, lambda _directory: hash_table)

        self.assertEqual(renamed, [])
        self.assertTrue(os.path.exists(os.path.join(self.tmpdir, "BW66.mkv")))


class TestSingleEpisodeFallback(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp(prefix="jw-sab-")

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def write_video(self, name):
        path = os.path.join(self.tmpdir, name)
        with open(path, "wb") as handle:
            handle.write(b"x" * 100)
        return path

    def test_renames_one_obfuscated_file_when_job_has_episode_marker(self):
        self.write_video("BW66.mkv")

        renamed = rename_single_episode(self.tmpdir, "Show.Name.S03E04.1080p.WEB-DL-GRP")

        self.assertEqual(len(renamed), 1)
        self.assertTrue(os.path.exists(os.path.join(self.tmpdir, "Show.Name.S03E04.1080p.WEB-DL-GRP.mkv")))

    def test_leaves_season_pack_obfuscated_files_alone(self):
        self.write_video("BW66.mkv")
        self.write_video("AcX5.mkv")

        renamed = rename_single_episode(self.tmpdir, "Show.Name.S03.1080p.WEB-DL-GRP")

        self.assertEqual(renamed, [])
        self.assertEqual(len([p for p in video_files(self.tmpdir) if is_obfuscated(p)]), 2)


class TestRun(unittest.TestCase):
    def test_exits_zero_for_non_tv_or_failed_jobs(self):
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(run(["script", "/tmp", "x.nzb", "x", "", "movies", "", "0", ""]), 0)
            self.assertEqual(run(["script", "/tmp", "x.nzb", "x", "", "tv", "", "1", ""]), 0)

    def test_treats_sonarr_category_as_tv(self):
        tmpdir = tempfile.mkdtemp(prefix="jw-sab-run-")
        try:
            with open(os.path.join(tmpdir, "BW66.mkv"), "wb") as handle:
                handle.write(b"x" * 100)

            with contextlib.redirect_stdout(io.StringIO()):
                result = run(
                    [
                        "script",
                        tmpdir,
                        "Show.Name.S01E02.1080p.WEB-DL-GRP.nzb",
                        "Show.Name.S01E02.1080p.WEB-DL-GRP",
                        "",
                        "sonarr",
                        "",
                        "0",
                        "",
                    ]
                )

            self.assertEqual(result, 0)
            self.assertTrue(os.path.exists(os.path.join(tmpdir, "Show.Name.S01E02.1080p.WEB-DL-GRP.mkv")))
        finally:
            shutil.rmtree(tmpdir, ignore_errors=True)

    def test_adds_sab_program_dir_to_import_path(self):
        old_env = os.environ.get("SAB_PROGRAM_DIR")
        old_path = list(sys.path)
        tmpdir = tempfile.mkdtemp(prefix="jw-sab-program-")
        try:
            os.environ["SAB_PROGRAM_DIR"] = tmpdir
            if tmpdir in sys.path:
                sys.path.remove(tmpdir)

            ensure_sabnzbd_import_path()

            self.assertIn(tmpdir, sys.path)
        finally:
            sys.path[:] = old_path
            shutil.rmtree(tmpdir, ignore_errors=True)
            if old_env is None:
                os.environ.pop("SAB_PROGRAM_DIR", None)
            else:
                os.environ["SAB_PROGRAM_DIR"] = old_env


if __name__ == "__main__":
    unittest.main()
