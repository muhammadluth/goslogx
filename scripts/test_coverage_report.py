"""Regression checks for cross-module Go coverage aggregation."""
import importlib.util
import pathlib
import tempfile
import sys
import unittest

sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location("coverage_report", pathlib.Path(__file__).with_name("coverage-report.py"))
report = importlib.util.module_from_spec(spec)
spec.loader.exec_module(report)


class CoverageMergeTest(unittest.TestCase):
    def test_shared_source_blocks_count_once(self):
        with tempfile.TemporaryDirectory() as directory:
            first, second = [pathlib.Path(directory) / name for name in ("core.out", "plugin.out")]
            first.write_text("mode: atomic\n" + report.PREFIX + "internal/shared.go:1.1,2.1 3 0\n")
            second.write_text("mode: atomic\n" + report.PREFIX + "internal/shared.go:1.1,2.1 3 4\n" + "example.org/external/file.go:1.1,2.1 20 5\n")
            self.assertEqual(report.merge_profiles([first, second]), {"internal/shared.go:1.1,2.1": (3, 4)})

    def test_mismatched_source_profiles_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            paths = [pathlib.Path(directory) / name for name in ("old.out", "new.out")]
            for count, path in enumerate(paths, 1):
                path.write_text(f"mode: atomic\n{report.PREFIX}file.go:1.1,2.1 {count} 1\n")
            with self.assertRaisesRegex(ValueError, "incompatible coverage"):
                report.merge_profiles(paths)


if __name__ == "__main__":
    unittest.main()
