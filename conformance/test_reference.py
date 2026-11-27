"""Run with python3 conformance/test_reference.py (requires the Go toolchain)."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class ReferenceTest(unittest.TestCase):
    def test_reference_passes_and_missing_standby_outcome_fails_at_first_rotation(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp)
            tracker = ROOT / "pkg/tracker/tracker.go"
            original = tracker.read_text()
            call = "ft.secondaryStructure.ReportOutcome(ctx, clientIdentifier, outcome)"
            self.assertEqual(original.count(call), 1)
            mutant = out / "tracker.go"
            mutant.write_text(original.replace(call, "// Deliberately skip warming the standby."))
            overlay = out / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {str(tracker): str(mutant)}}))
            for name, flags in [("reference", []), ("mutant", [f"-overlay={overlay}"])]:
                binary = out / name
                subprocess.run(["go", "build", *flags, "-o", binary, "./conformance/reference"],
                               cwd=ROOT, check=True)
                result = subprocess.run([sys.executable, ROOT / "conformance/check.py", binary],
                                        capture_output=True, text=True)
                if name == "reference":
                    self.assertEqual(result.returncode, 0, result.stderr)
                else:
                    self.assertEqual(result.returncode, 1, result.stderr)
                    self.assertIn("operation 6 (register): expected 0.5", result.stderr)


if __name__ == "__main__":
    unittest.main()
