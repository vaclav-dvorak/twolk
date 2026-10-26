#!/usr/bin/env python3
"""Generate internal/db/emblems_gen.go AND internal/db/pvp_gen.go from
authoritative vendor data — replaces the earlier per-currency wotlkdb
scrapes (gen_emblems.py, gen_pvp.py), whose listview pages truncate at
300 rows and silently lost the ilvl-232 Emblem of Triumph tier and all
sub-232 PvP-currency items.

Pipeline (no truncation anywhere):
 1. AzerothCore's npc_vendor base SQL (the authoritative 3.3.5 world
    database, one bulk download) says WHICH vendors sell WHICH items
    with an extended (non-gold) cost.
 2. For the vendors selling items in our WoWSims item pool (~164 NPCs),
    fetch the wotlkdb.com NPC page once: its item listview carries the
    complete cost bundle per item — gold, currencies (emblems, honor,
    arena, marks) and item-costs (raid tokens, orbs). Vendor stocks are
    far below the 300-row listview cap, so nothing is cut.
 3. Emit:
    - emblemPrices: cheapest emblem cost per item (display + vendor
      source synthesis);
    - pvpCurrencyItems: items ONLY obtainable via PvP currencies
      (honor / arena points / battleground marks / venture coins) —
      an offer payable with gold, emblems or PvE tokens exempts the
      item.

Usage:
    python3 tools/gen_vendorcosts.py emblems > internal/db/emblems_gen.go
    python3 tools/gen_vendorcosts.py pvp     > internal/db/pvp_gen.go

Env:
    WOWSIMS_DB        path to a local db.json (default: fetch pinned URL)
    ACORE_VENDOR_SQL  path to a local npc_vendor.sql (default: fetch)
    GEN_VENDOR_CACHE  directory caching npc<ID>.html pages across runs
"""

import json
import os
import re
import sys
import time
import urllib.request

PINNED = "02e0d9fba5acb5c0a3d9248cb82285641540a1a9"
DB_URL = f"https://raw.githubusercontent.com/wowsims/wotlk/{PINNED}/assets/database/db.json"
VENDOR_URL = ("https://raw.githubusercontent.com/azerothcore/azerothcore-wotlk/"
              "master/data/sql/base/db_world/npc_vendor.sql")

EMBLEMS = {  # wotlkdb currency ID -> Go enum name
    101: "EmblemHeroism",
    102: "EmblemValor",
    221: "EmblemConquest",
    301: "EmblemTriumph",
    341: "EmblemFrost",
}
# PvP acquisition: currency IDs and mark/coin item IDs as they appear in
# wotlkdb cost bundles
PVP_CURRENCIES = {103, 104, 121, 122, 123, 124, 125, 126, 321, 201}
PVP_COST_ITEMS = {
    20558,  # Warsong Gulch Mark of Honor
    20559,  # Arathi Basin Mark of Honor
    20560,  # Alterac Valley Mark of Honor
    29024,  # Eye of the Storm Mark of Honor
    42425,  # Strand of the Ancients Mark of Honor
    43589,  # Wintergrasp Mark of Honor
    47395,  # Isle of Conquest Mark of Honor
    37836,  # Venture Coin
}

LISTVIEW_CAP = 300  # aowow truncates listviews at this many rows


def fetch_text(url):
    req = urllib.request.Request(url, headers={"User-Agent": "twolk-gen-vendorcosts/1.0"})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return resp.read().decode("utf-8", "replace")


def load_db():
    path = os.environ.get("WOWSIMS_DB")
    if path:
        with open(path) as f:
            return json.load(f)
    return json.loads(fetch_text(DB_URL))


def load_vendor_sql():
    path = os.environ.get("ACORE_VENDOR_SQL")
    if path:
        with open(path) as f:
            return f.read()
    return fetch_text(VENDOR_URL)


def fetch_npc(nid):
    cache = os.environ.get("GEN_VENDOR_CACHE")
    fn = cache and f"{cache}/npc{nid}.html"
    if fn and os.path.exists(fn):
        with open(fn) as f:
            return f.read()
    html = fetch_text(f"https://wotlkdb.com/?npc={nid}")
    time.sleep(1)
    if fn:
        with open(fn, "w") as f:
            f.write(html)
    return html


def item_listview(html):
    """Extract the rows of the page's item listview (the sells table)."""
    m = re.search(r'new Listview\(\{"template":"item"', html)
    if not m:
        return []
    i = html.find("[", html.find('"data":[', m.start()))
    depth, start = 0, i
    while True:
        if html[i] == "[":
            depth += 1
        elif html[i] == "]":
            depth -= 1
            if depth == 0:
                break
        i += 1
    return json.loads(html[start:i + 1])


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if mode not in ("emblems", "pvp"):
        sys.exit("usage: gen_vendorcosts.py emblems|pvp")

    pool = {it["id"] for it in load_db()["items"]}
    rows = re.findall(r"\((\d+),(-?\d+),(\d+),(\d+),(\d+),(\d+),(?:\d+|NULL)\)",
                      load_vendor_sql())
    npcs = set()          # vendors selling pool items for extended costs
    gold_only = set()     # pool items some vendor sells for plain gold
    for entry, _slot, item, _mx, _inc, ec in rows:
        iid = int(item)
        if iid not in pool:
            continue
        if ec == "0":
            gold_only.add(iid)
        else:
            npcs.add(int(entry))

    # offers: item -> list of (currencies {id:count}, itemcosts {id:count})
    offers = {}
    for n, nid in enumerate(sorted(npcs)):
        lv = item_listview(fetch_npc(nid))
        if len(lv) >= LISTVIEW_CAP:
            print(f"WARNING: npc {nid} sells listview hit the {LISTVIEW_CAP}-row cap",
                  file=sys.stderr)
        for r in lv:
            cost = r.get("cost")
            if not isinstance(cost, list) or len(cost) < 2:
                continue
            curs = {c: cnt for c, cnt in (cost[1] or [])}
            itemcosts = {c: cnt for c, cnt in (cost[2] or [])} if len(cost) > 2 else {}
            if not curs and not itemcosts:
                gold_only.add(r["id"])
                continue
            offers.setdefault(r["id"], []).append((curs, itemcosts))
        if (n + 1) % 25 == 0:
            print(f"  {n + 1}/{len(npcs)} vendors fetched", file=sys.stderr)

    if mode == "emblems":
        table = {}  # itemID -> (enum, count)
        for iid, offs in offers.items():
            if iid not in pool:
                continue
            for curs, _ in offs:
                for cid, enum in EMBLEMS.items():
                    if cid in curs:
                        cnt = curs[cid]
                        if iid not in table or table[iid][1] > cnt:
                            table[iid] = (enum, cnt)
        print("// Code generated by tools/gen_vendorcosts.py from AzerothCore")
        print("// npc_vendor + wotlkdb.com NPC pages (3.3.5a); DO NOT EDIT.")
        print()
        print("package db")
        print()
        print("var emblemPrices = map[int]EmblemPrice{")
        for iid in sorted(table):
            enum, count = table[iid]
            print(f"\t{iid}: {{{enum}, {count}}},")
        print("}")
        print(f"emblems: {len(table)} items", file=sys.stderr)
        return

    # pvp: every offer requires a PvP currency/mark, and no gold-only
    # or PvE-currency route exists
    pvp = set()
    for iid, offs in offers.items():
        if iid not in pool or iid in gold_only:
            continue
        def pvp_offer(o):
            curs, itemcosts = o
            return (any(c in PVP_CURRENCIES for c in curs) or
                    any(c in PVP_COST_ITEMS for c in itemcosts))
        if offs and all(pvp_offer(o) for o in offs):
            pvp.add(iid)
    print("// Code generated by tools/gen_vendorcosts.py from AzerothCore")
    print("// npc_vendor + wotlkdb.com NPC pages (3.3.5a); DO NOT EDIT.")
    print()
    print("package db")
    print()
    print("// pvpCurrencyItems holds every item obtainable ONLY via PvP")
    print("// currencies (honor, arena points, battleground marks, venture")
    print("// coins): PvP-acquired even when it carries no resilience.")
    print("var pvpCurrencyItems = map[int]bool{")
    line = []
    for iid in sorted(pvp):
        line.append(f"{iid}: true")
        if len(line) == 6:
            print("\t" + ", ".join(line) + ",")
            line = []
    if line:
        print("\t" + ", ".join(line) + ",")
    print("}")
    print(f"pvp: {len(pvp)} items", file=sys.stderr)


if __name__ == "__main__":
    main()
