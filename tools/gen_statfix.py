#!/usr/bin/env python3
"""Generate internal/db/statfix_gen.go — original 3.3.5 stats for items
where the WoWSims (WotLK Classic) data diverges. Classic buffed Ulduar
loot by ~1 ilvl tier; free 3.3.5 servers run the originals.

Source: AzerothCore's item_template base SQL (the authoritative 3.3.5
world database), one bulk download from GitHub's CDN.

Usage:
    python3 tools/gen_statfix.py > internal/db/statfix_gen.go

Env:
    WOWSIMS_DB        path to a local db.json (default: fetch pinned URL)
    ACORE_ITEM_SQL    path to a local item_template.sql (default: fetch)
"""

import json
import os
import sys
import urllib.request

PINNED = "02e0d9fba5acb5c0a3d9248cb82285641540a1a9"
DB_URL = f"https://raw.githubusercontent.com/wowsims/wotlk/{PINNED}/assets/database/db.json"
ACORE_URL = ("https://raw.githubusercontent.com/azerothcore/azerothcore-wotlk/"
             "master/data/sql/base/db_world/item_template.sql")

# WotLK-era stat count: both the WoWSims db.json wire arrays and the
# emitted map keys stop here. The keys are CANONICAL db.Stat indices
# (internal/db/db.go) — safe because the canonical enum is append-only
# with the WotLK positions frozen (TestCanonicalStatContract).
NUM_STATS = 35
# canonical Stat enum indices (internal/db/db.go)
STR, AGI, STA, INT, SPI = 0, 1, 2, 3, 4
SP, MP5, SPHIT, SPCRIT, SPHASTE, SPPEN = 5, 6, 7, 8, 9, 10
AP, MHIT, MCRIT, MHASTE, ARP, EXP = 11, 12, 13, 14, 15, 16
MANA, ARMOR, RAP, DEF, BLOCK, BLOCKV = 17, 20, 21, 22, 23, 24
DODGE, PARRY, RESIL, HEALTH = 25, 26, 27, 28
ARCR, FIRR, FROR, NATR, SHAR = 29, 30, 31, 32, 33

# server ITEM_MOD -> list of our stat indices, following the WoWSims
# convention of duplicating unified ratings into both sides
ITEM_MOD = {
    0: [MANA], 1: [HEALTH], 3: [AGI], 4: [STR], 5: [INT], 6: [SPI], 7: [STA],
    12: [DEF], 13: [DODGE], 14: [PARRY], 15: [BLOCK],
    16: [MHIT], 17: [MHIT], 18: [SPHIT],
    19: [MCRIT], 20: [MCRIT], 21: [SPCRIT],
    28: [MHASTE], 29: [MHASTE], 30: [SPHASTE],
    31: [MHIT, SPHIT], 32: [MCRIT, SPCRIT], 36: [MHASTE, SPHASTE],
    35: [RESIL], 37: [EXP],
    38: [AP, RAP], 39: [RAP],
    43: [MP5], 44: [ARP], 45: [SP], 47: [SPPEN], 48: [BLOCKV],
    # 40 feral AP and 41/42 legacy heal/spell damage are intentionally
    # skipped: WoWSims doesn't carry them in stat arrays either
}


def load_wowsims():
    path = os.environ.get("WOWSIMS_DB")
    if path:
        with open(path) as f:
            return json.load(f)
    with urllib.request.urlopen(DB_URL, timeout=60) as r:
        return json.load(r)


def load_sql():
    path = os.environ.get("ACORE_ITEM_SQL")
    if path:
        with open(path, encoding="utf-8", errors="replace") as f:
            return f.read()
    req = urllib.request.Request(ACORE_URL, headers={"User-Agent": "twolk-gen-statfix/1.0"})
    with urllib.request.urlopen(req, timeout=120) as r:
        return r.read().decode("utf-8", "replace")


def columns(sql):
    """Ordered column names from the CREATE TABLE block."""
    cols = []
    in_create = False
    for line in sql.splitlines():
        s = line.strip()
        if s.upper().startswith("CREATE TABLE"):
            in_create = True
            continue
        if in_create:
            if s.startswith("`"):
                cols.append(s.split("`")[1])
            elif s.startswith(")"):
                break
    return cols


def tuples(sql):
    """Yield value tuples from INSERT statements as lists of raw strings."""
    i = 0
    n = len(sql)
    while True:
        i = sql.find("INSERT INTO", i)
        if i < 0:
            return
        i = sql.find("VALUES", i)
        i = sql.find("(", i)
        # parse tuples until the terminating ';'
        while i < n and sql[i] == "(":
            i += 1
            fields, buf, depth, instr = [], [], 0, False
            while i < n:
                c = sql[i]
                if instr:
                    if c == "\\":
                        buf.append(sql[i:i+2])
                        i += 2
                        continue
                    buf.append(c)
                    if c == "'":
                        instr = False
                elif c == "'":
                    instr = True
                    buf.append(c)
                elif c == "(":
                    depth += 1
                    buf.append(c)
                elif c == ")" and depth > 0:
                    depth -= 1
                    buf.append(c)
                elif c == "," and depth == 0:
                    fields.append("".join(buf))
                    buf = []
                elif c == ")":
                    fields.append("".join(buf))
                    i += 1
                    break
                else:
                    buf.append(c)
                i += 1
            yield fields
            # skip ",(" between tuples or ";" at end
            while i < n and sql[i] in ",;\r\n ":
                if sql[i] == ";":
                    i += 1
                    break
                i += 1


def main():
    ws = load_wowsims()
    ws_items = {i["id"]: i for i in ws["items"]}
    sql = load_sql()
    cols = columns(sql)
    ix = {c: n for n, c in enumerate(cols)}
    print(f"{len(cols)} columns, {len(ws_items)} wowsims items", file=sys.stderr)

    fixes = {}
    seen = 0
    for row in tuples(sql):
        if len(row) != len(cols):
            continue
        entry = int(row[ix["entry"]])
        it = ws_items.get(entry)
        if it is None:
            continue
        seen += 1
        # random-suffix items carry their stats in the suffix, and
        # scaling items (heirloom-style) in ScalingStatDistribution —
        # neither can be compared via the stat columns
        if int(row[ix["RandomProperty"]]) != 0 or int(row[ix["RandomSuffix"]]) != 0:
            continue
        if int(row[ix["ScalingStatDistribution"]]) != 0:
            continue

        stats = [0.0] * NUM_STATS
        for k in range(1, 11):
            mod = int(row[ix[f"stat_type{k}"]])
            val = float(row[ix[f"stat_value{k}"]])
            if val == 0:
                continue
            for target in ITEM_MOD.get(mod, []):
                stats[target] += val
        stats[ARMOR] += float(row[ix["armor"]])
        stats[BLOCKV] += float(row[ix["block"]]) # shield block value
        for col, target in (("fire_res", FIRR), ("nature_res", NATR),
                            ("frost_res", FROR), ("shadow_res", SHAR),
                            ("arcane_res", ARCR)):
            stats[target] += float(row[ix[col]])
        ilvl = int(row[ix["ItemLevel"]])
        dmg_min = float(row[ix["dmg_min1"]])
        dmg_max = float(row[ix["dmg_max1"]])
        speed = int(row[ix["delay"]]) / 1000.0  # ms → s

        # Only ilvl changes mark Classic re-tuning (the Ulduar buff bumped
        # ilvls). Stat-only differences are almost always vanilla/TBC items
        # whose stats come from equip spells, which the template encodes as
        # spell IDs rather than stat columns — not real divergence.
        if ilvl == it.get("ilvl"):
            continue
        # never destroy stats: if WoWSims has a stat the template lacks
        # entirely, it comes from an equip spell we can't see — skip the
        # item rather than emit a lossy fix
        ws_stats = (it.get("stats") or []) + [0.0] * NUM_STATS
        lossy = [s for s in range(NUM_STATS)
                 if float(ws_stats[s]) > 0 and stats[s] == 0]
        if lossy:
            print(f"  SKIP {entry} {it.get('name')} (phase {it.get('phase')}): "
                  f"stats {lossy} would be lost (equip-spell stats)", file=sys.stderr)
            continue
        fixes[entry] = (ilvl, stats, dmg_min, dmg_max, speed,
                        it.get("phase"), it.get("name"))

    from collections import Counter
    print(f"matched {seen}, fixes {len(fixes)}, by phase:",
          Counter(p for *_, p, _ in fixes.values()), file=sys.stderr)

    print("// Code generated by tools/gen_statfix.py from AzerothCore's")
    print("// item_template (authoritative 3.3.5 world DB); DO NOT EDIT.")
    print("//")
    print("// Restores original 3.3.5 ilvl+stats where the WoWSims (WotLK")
    print("// Classic) data diverges — mainly Classic's Ulduar loot buff.")
    print()
    print("package db")
    print()
    print("var statFixes = map[int]statFix{")
    for entry in sorted(fixes):
        ilvl, stats, dmin, dmax, speed, _, name = fixes[entry]
        parts = ", ".join(
            f"{s}: {stats[s]:g}" for s in range(NUM_STATS) if stats[s] != 0)
        dmg = ""
        if dmin or dmax:
            dmg = f", {dmin:g}, {dmax:g}, {speed:g}"
        else:
            dmg = ", 0, 0, 0"
        print(f"\t{entry}: {{{ilvl}, Stats{{{parts}}}{dmg}}}, // {name}")
    print("}")


if __name__ == "__main__":
    main()
