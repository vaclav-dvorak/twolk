#!/usr/bin/env python3
"""Generate internal/db/catacosts_gen.go — Justice/Valor prices, PvP
currency classification and raid-token gating for Cataclysm vendor
items.

PROVENANCE: mixed-era. The npc_vendor linkage (which item is sold, for
which ExtendedCost) is ORIGINAL Cataclysm 4.3.4 world data — no 4.4.x
server dump exists publicly (vendor inventories are server-side and
Whitemane's core is proprietary), so the emitted tables encode the
original game's FINAL state. Two consequences for the Classic (4.4.x)
module the planner actually models:
  - currency assignment rotates per phase in Classic (the current
    tier's gear costs Valor, older tiers drop to Justice at the same
    amount); RuleSet.RotatePrice maps the end-state back per query.
  - Classic-only vendors (the Elemental Rune protocol economy) are
    invisible here; tools/gen_cata_protocol.py covers them from
    warcraft.wiki.gg. A future ORIGINAL-cata module would use this
    file's tables as-is — for that flavor the TDB is native truth.
The cost DEFINITIONS (ItemExtendedCost/CurrencyTypes CSVs) come from
the 4.4.x Classic client via wago.tools and are Classic-native.

Bulk upstreams (no scraping):
  - TCPP TDB world SQL (The Cataclysm Preservation Project — TrinityCore
    4.3.4, the same core family free cata servers run): npc_vendor says
    WHICH items are sold WITH which ExtendedCost id.
    https://github.com/The-Cataclysm-Preservation-Project/TrinityCore/
    releases TDB434.22011 (extract TDB_full_world_*.sql from the 7z).
  - wago.tools ItemExtendedCost + CurrencyTypes CSVs (cata classic
    client build): what each ExtendedCost id actually costs. Point
    currencies are stored x100 (CURRENCY_PRECISION).

Emitted tables (consumed in emblems.go / curated.go):
  - cataVendorPrices: cheapest Justice/Valor cost per item.
  - cataPvPVendorItems: items whose every non-gold offer is PvP currency
    (honor/conquest/arena rating) — unions into PvPItem.
  - cataTokenTradeItems: non-PvP items obtainable at vendors ONLY by
    trading a cost ITEM (tier tokens, Crystallized Firestone) — these
    are raid content in disguise, unions into RaidGated.
  - cataFirestoneUpgrades: heroic Firelands item -> the normal item it
    is traded up from (+ one Crystallized Firestone, Lurah Wrathvine
    at the raid's entrance) — the route the filter and source line
    model explicitly instead of a bare "raid tokens".

Usage:
    python3 tools/gen_cata_vendorcosts.py > internal/db/catacosts_gen.go

Env:
    TDB_WORLD_SQL       path to TDB_full_world_434.*.sql (required)
    ITEM_EXTENDED_COST  path to ItemExtendedCost.csv (required)
    WOWSIMS_CATA_DB     path to a local db.json (default: fetch pinned URL)
"""

import csv
import json
import os
import re
import sys
import urllib.request

PINNED = "70d87383a9b92f30fb9e370c4676d3ce33b6e6b6"
DB_URL = f"https://raw.githubusercontent.com/wowsims/cata/{PINNED}/assets/database/db.json"

JUSTICE, VALOR = 395, 396
FIRESTONE = 71617  # Crystallized Firestone
HONOR, CONQUEST = 392, 390
POINT_CURRENCIES = {JUSTICE, VALOR, HONOR, CONQUEST}  # stored x100


def load_db():
    path = os.environ.get("WOWSIMS_CATA_DB")
    if path:
        return json.load(open(path))
    print(f"fetching {DB_URL}", file=sys.stderr)
    with urllib.request.urlopen(DB_URL) as r:
        return json.load(r)


def load_extended_costs(path):
    """ExtendedCost id -> (currencies [(id, count)], items [(id, count)],
    arena_rating)."""
    out = {}
    for row in csv.DictReader(open(path)):
        cid = int(row["ID"])
        curr, items = [], []
        for i in range(5):
            c, n = int(row[f"CurrencyID_{i}"]), int(row[f"CurrencyCount_{i}"])
            if c and n:
                curr.append((c, n // 100 if c in POINT_CURRENCIES else n))
            it, m = int(row[f"ItemID_{i}"]), int(row[f"ItemCount_{i}"])
            if it and m:
                items.append((it, m))
        out[cid] = (curr, items, int(row["RequiredArenaRating"]))
    return out


def load_vendor_offers(path):
    """item id -> set of ExtendedCost ids it is sold with (0 = gold)."""
    offers = {}
    ins = re.compile(r"INSERT INTO `npc_vendor` VALUES\s*(.*?);\s*$", re.S)
    tup = re.compile(r"\((-?\d+),(-?\d+),(-?\d+),(\d+),(\d+),(\d+),")
    buf, capturing = [], False
    with open(path, encoding="utf-8", errors="replace") as f:
        for line in f:
            if line.startswith("INSERT INTO `npc_vendor`"):
                capturing = True
            if capturing:
                buf.append(line)
                if line.rstrip().endswith(";"):
                    chunk = "".join(buf)
                    for m in tup.finditer(chunk):
                        item, cost = int(m.group(3)), int(m.group(6))
                        offers.setdefault(item, set()).add(cost)
                    buf, capturing = [], False
    return offers


def main():
    sql = os.environ["TDB_WORLD_SQL"]
    iec = os.environ["ITEM_EXTENDED_COST"]
    db = load_db()
    pool = {it["id"]: it["name"] for it in db["items"]}

    costs = load_extended_costs(iec)
    offers = load_vendor_offers(sql)
    print(f"npc_vendor items: {len(offers)}, in pool: "
          f"{len(set(offers) & set(pool))}", file=sys.stderr)

    prices, pvp, tokens, upgrades = {}, [], [], {}
    for iid in sorted(set(offers) & set(pool)):
        point_offers, has_gold, has_pvp, has_token = [], False, False, False
        for cid in offers[iid]:
            if cid == 0:
                has_gold = True
                continue
            curr, items, rating = costs.get(cid, ([], [], 0))
            ids = {c for c, _ in curr}
            if ids & {HONOR, CONQUEST} or rating:
                has_pvp = True
            elif ids & {JUSTICE, VALOR}:
                point_offers.extend((c, n) for c, n in curr
                                    if c in (JUSTICE, VALOR))
            elif items and not ids:
                has_token = True
                rest = [it for it, _ in items if it != FIRESTONE]
                if len(rest) == 1 and len(items) == 2:
                    upgrades[iid] = rest[0]
        if point_offers:
            # cheapest: justice preferred over valor, then lowest count
            c, n = min(point_offers, key=lambda p: (p[0] == VALOR, p[1]))
            prices[iid] = (c, n)
        elif has_pvp and not has_gold and not has_token:
            pvp.append(iid)
        elif has_token and not has_gold and not has_pvp:
            tokens.append(iid)

    print(f"prices {len(prices)}, pvp-only {len(pvp)}, "
          f"token-only {len(tokens)}", file=sys.stderr)

    w = sys.stdout.write
    w("// Code generated by tools/gen_cata_vendorcosts.py from the TCPP\n")
    w("// TDB 434.22011 npc_vendor table + the cata-classic\n")
    w("// ItemExtendedCost client data (wago.tools); DO NOT EDIT.\n\n")
    w("package db\n\n")

    w("var cataVendorPrices = map[int]EmblemPrice{\n")
    for iid in sorted(prices):
        c, n = prices[iid]
        cur = "EmblemJustice" if c == JUSTICE else "EmblemValorPoints"
        w(f"\t{iid}: {{{cur}, {n}}}, // {pool[iid]}\n")
    w("}\n\n")

    w("// cataPvPVendorItems: every non-gold vendor offer costs honor,\n")
    w("// conquest or an arena rating.\n")
    w("var cataPvPVendorItems = map[int]bool{\n")
    for iid in sorted(pvp):
        w(f"\t{iid}: true, // {pool[iid]}\n")
    w("}\n\n")

    w("// cataTokenTradeItems: only obtainable by trading a cost ITEM\n")
    w("// (tier tokens, heroic Firestones) — raid content in disguise.\n")
    w("var cataTokenTradeItems = map[int]bool{\n")
    for iid in sorted(tokens):
        w(f"\t{iid}: true, // {pool[iid]}\n")
    w("}\n\n")

    w("// cataFirestoneUpgrades: heroic item -> the normal item traded in\n")
    w("// with one Crystallized Firestone.\n")
    w("var cataFirestoneUpgrades = map[int]int{\n")
    for iid in sorted(upgrades):
        w(f"\t{iid}: {upgrades[iid]}, // {pool[iid]}\n")
    w("}\n")


if __name__ == "__main__":
    main()
