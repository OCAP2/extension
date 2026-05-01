#!/usr/bin/env python3
"""Deduplicate Go coverage profile blocks produced by `go test -coverpkg=...`.

When -coverpkg instruments the same file from multiple test packages, Go can
emit overlapping block ranges (different end positions for the same start) for
the same logical statement. A naive key-by-range dedup leaves the count=0
versions of those blocks intact, which downstream coverage tools then count as
"missed" statements. Result: phantom coverage drops on PRs that don't actually
reduce coverage.

This script keeps the highest-count entry per (file, start position), then
drops any remaining count=0 block whose range overlaps a covered block in the
same file.

Usage: dedup-coverage.py <input> <output>
"""
from __future__ import annotations

import sys
from collections import defaultdict


def parse_line(line: str):
    parts = line.split(" ")
    if len(parts) != 3:
        return None
    rng, stmt_s, count_s = parts
    file, span = rng.rsplit(":", 1)
    start, end = span.split(",")
    sl, sc = start.split(".")
    el, ec = end.split(".")
    return {
        "file": file,
        "start": (int(sl), int(sc)),
        "end": (int(el), int(ec)),
        "stmt": int(stmt_s),
        "count": int(count_s),
        "raw": line,
    }


def merge_intervals(intervals):
    intervals.sort()
    merged = []
    for s, e in intervals:
        if merged and s <= merged[-1][1]:
            merged[-1] = (merged[-1][0], max(merged[-1][1], e))
        else:
            merged.append((s, e))
    return merged


def main(in_path: str, out_path: str) -> int:
    with open(in_path) as f:
        header = f.readline()
        entries = [parse_line(l.rstrip()) for l in f if l.strip()]
    entries = [e for e in entries if e is not None]

    # Keep the best entry per (file, start): max count, then max end position.
    best: dict[tuple, dict] = {}
    for e in entries:
        key = (e["file"], e["start"])
        cur = best.get(key)
        if cur is None:
            best[key] = e
            continue
        if e["count"] > cur["count"] or (
            e["count"] == cur["count"] and e["end"] > cur["end"]
        ):
            best[key] = e

    # Build merged covered intervals per file.
    covered: dict[str, list] = defaultdict(list)
    for e in best.values():
        if e["count"] > 0:
            covered[e["file"]].append((e["start"], e["end"]))
    for f in covered:
        covered[f] = merge_intervals(covered[f])

    def overlaps_covered(file: str, s, e) -> bool:
        for cs, ce in covered.get(file, ()):
            if s <= ce and cs <= e:
                return True
        return False

    out = []
    for e in best.values():
        if e["count"] == 0 and overlaps_covered(e["file"], e["start"], e["end"]):
            continue
        out.append(e["raw"])
    out.sort()

    with open(out_path, "w") as f:
        f.write(header)
        for line in out:
            f.write(line + "\n")
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(f"usage: {sys.argv[0]} <input> <output>", file=sys.stderr)
        sys.exit(2)
    sys.exit(main(sys.argv[1], sys.argv[2]))
