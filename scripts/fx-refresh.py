#!/usr/bin/env python3
"""Fetch the ECB daily reference rates and rewrite internal/fx/rates.json.

Deterministic output (sorted keys, fixed formatting) so a re-run with unchanged
rates produces no diff, and the CI check stays quiet.

Exit codes: 0 = wrote/unchanged, 1 = fetch or parse failure.
"""

import json
import sys
import urllib.request
import xml.etree.ElementTree as ET

SOURCE = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
OUT = "internal/fx/rates.json"

# Currencies the ECB does not publish, computed from a documented peg. The note
# is shown to the reader, so a derived number is never mistaken for a quote.
PEGS = {
    "AED": ("USD", 3.6725, "AED: derived from its fixed peg to the USD at 3.6725 (not an ECB quote)"),
}
NS = {"e": "http://www.ecb.int/vocabulary/2002-08-01/eurofxref"}


def main() -> int:
    try:
        with urllib.request.urlopen(SOURCE, timeout=30) as res:
            raw = res.read()
    except Exception as exc:  # noqa: BLE001 - report and fail
        print(f"fx-refresh: fetch failed: {exc}", file=sys.stderr)
        return 1

    try:
        root = ET.fromstring(raw)
        cube = root.find(".//e:Cube[@time]", NS)
        if cube is None:
            raise ValueError("no dated Cube element")
        date = cube.get("time")
        rates = {"EUR": 1.0}
        for child in cube.findall("e:Cube", NS):
            rates[child.get("currency")] = float(child.get("rate"))
    except Exception as exc:  # noqa: BLE001
        print(f"fx-refresh: parse failed: {exc}", file=sys.stderr)
        return 1

    if len(rates) < 2:
        print("fx-refresh: suspiciously few currencies, refusing to write", file=sys.stderr)
        return 1

    derived = {}
    for code, (base, factor, note) in PEGS.items():
        if base in rates:
            rates[code] = rates[base] * factor
            derived[code] = note

    body = {
        "base": "EUR",
        "date": date,
        "source": SOURCE,
        "rates": dict(sorted(rates.items())),
    }
    if derived:
        body["derived"] = dict(sorted(derived.items()))
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(body, f, indent=2)
        f.write("\n")
    print(f"fx-refresh: wrote {OUT} for {date} ({len(rates)} currencies)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
