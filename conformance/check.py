"""Score the deterministic rotation pilot. Run candidate binaries only in a sandbox."""

import argparse
import json
import math
from pathlib import Path
import subprocess
import sys


def check(binary):
    # The second rotation distinguishes a newly warmed standby from a copy of the active one.
    steps = [("register", 0), ("failure", None), ("failure", None), ("register", .5),
             ("rotate", None), ("register", .5), ("success", None), ("register", .375),
             ("failure", None), ("register", .625), ("rotate", None), ("register", .25),
             ("rotate", None), ("register", 0),
             # Consecutive rotations must finish independently: the first
             # promotes the client penalty and the second discards it.
             ("failure", None), ("rotate", None), ("rotate", None), ("register", 0)]
    inputs = [{"op": op} if op == "rotate" else {"op": op, "client": "noisy"} for op, _ in steps]
    result = subprocess.run([binary], input="".join(json.dumps(x) + "\n" for x in inputs),
                            stdout=subprocess.PIPE, text=True, timeout=10, check=True)
    lines = result.stdout.splitlines()
    if len(lines) != len(steps):
        raise ValueError(f"expected {len(steps)} responses, received {len(lines)}")
    for index, ((op, expected), line) in enumerate(zip(steps, lines), start=1):
        actual = json.loads(line)
        if expected is None and actual == {}:
            continue
        if expected is not None and isinstance(actual, dict) and actual.keys() == {"probability"}:
            p = actual["probability"]
            if type(p) in (int, float) and math.isclose(p, expected, abs_tol=1e-12, rel_tol=0):
                continue
        raise ValueError(f"operation {index} ({op}): expected {expected if expected is not None else {}}, got {actual!r}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    try:
        check(args.binary.resolve())
    except (OSError, ValueError, OverflowError, subprocess.SubprocessError) as exc:
        sys.exit(f"FAIL rotation: {exc}")
    print("PASS rotation")
