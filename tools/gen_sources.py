#!/usr/bin/env python3
"""Generate internal/db/sources_gen.go — true 3.3.5 sources for items the
upstream (WoWSims) data cannot source:
  - items whose only upstream sources are WotLK-Classic-only and get
    stripped at parse time (Titan Rune drops, Sidereal vendors);
  - items with no upstream source at all (ilvl >= 200, non-PvP): quest
    rewards (Onyxia head turn-ins, legendaries), raid trash BoEs, world
    drops.
Scraped from wotlkdb.com (original 3.3.5a database) item pages. Items
wotlkdb cannot source either are emitted as unobtainableItems and
dropped from the pool at load — they exist only as data leftovers (or
are WotLK-Classic-era inventions like the 211xxx Hallow's End gear).

Usage:
    python3 tools/gen_sources.py > internal/db/sources_gen.go

Env:
    WOWSIMS_DB        path to a local db.json (default: fetch pinned URL)
    GEN_SOURCES_CACHE directory caching item html pages across runs
"""

import json
import os
import re
import sys
import time
import urllib.request

PINNED = "02e0d9fba5acb5c0a3d9248cb82285641540a1a9"
DB_URL = f"https://raw.githubusercontent.com/wowsims/wotlk/{PINNED}/assets/database/db.json"
# keep in sync with internal/db/db.go drop335Irrelevant
CLASSIC_VENDORS = {"Animated Constellation", "Korralin Hoperender", "Kolara Dreamsmasher"}


def load_db():
    path = os.environ.get("WOWSIMS_DB")
    if path:
        with open(path) as f:
            return json.load(f)
    with urllib.request.urlopen(DB_URL, timeout=60) as r:
        return json.load(r)


def emblem_ids():
    """Items already covered by the emblem price overlay."""
    path = os.path.join(os.path.dirname(__file__), "..", "internal", "db", "emblems_gen.go")
    with open(path) as f:
        return {int(m.group(1)) for m in re.finditer(r"^\t(\d+): \{", f.read(), re.M)}


def survives_strip(s):
    drop = s.get("drop")
    if drop and drop.get("difficulty") in (7, 8):
        return False
    sold = s.get("soldBy")
    if sold and sold.get("npcName") in CLASSIC_VENDORS:
        return False
    return True


def fetch_item(iid):
    cache = os.environ.get("GEN_SOURCES_CACHE")
    fp = f"{cache}/item{iid}.html" if cache else None
    if fp and os.path.exists(fp):
        with open(fp) as f:
            return f.read()
    req = urllib.request.Request(
        f"https://wotlkdb.com/?item={iid}",
        headers={"User-Agent": "twolk-gen-sources/1.0"},
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        html = r.read().decode("utf-8", "replace")
    if fp:
        with open(fp, "w") as f:
            f.write(html)
    time.sleep(0.4)
    return html


def listview(html, lid):
    m = html.find(f'"id":"{lid}"')
    if m < 0:
        return []
    i = html.find("[", html.find('"data":[', m))
    depth, start = 0, i
    while True:
        if html[i] == "[":
            depth += 1
        elif html[i] == "]":
            depth -= 1
            if depth == 0:
                break
        i += 1
    return json.loads(html[start : i + 1])


def clean_name(name):
    # aowow prefixes item/object names with a quality digit
    return name[1:] if name and name[0].isdigit() else name


def clean_npc(name):
    # aowow disambiguates duplicate NPC entries: "Onyxia (1)", "Rotface (2)"
    return re.sub(r" \(\d+\)$", "", name)


def best_source(html):
    """Returns (kind, npcID, name, zoneID) or None."""
    drops = listview(html, "dropped-by")
    if drops:
        # spread across many mobs in many zones at tiny rates = world
        # drop (raid trash BoEs stay "drop": one zone, few droppers)
        zones = {(r.get("location") or [0])[0] for r in drops}
        top = max((r.get("percent") or 0) for r in drops)
        if len(drops) >= 8 and len(zones) >= 5 and top < 2:
            return ("world", 0, "", 0)
        # prefer bosses, then highest drop chance (trash aggregates)
        drops.sort(key=lambda r: (-r.get("boss", 0), -(r.get("percent") or 0)))
        r = drops[0]
        zone = (r.get("location") or [0])[0]
        return ("drop", r["id"], clean_npc(r.get("name", "")), zone)
    sold = listview(html, "sold-by")
    if sold:
        r = sold[0]
        zone = (r.get("location") or [0])[0]
        return ("vendor", r["id"], r.get("name", ""), zone)
    objs = listview(html, "contained-in-object")
    if objs:
        r = objs[0]
        zone = (r.get("location") or [0])[0]
        return ("object", 0, clean_name(r.get("name", "")), zone)
    for lid in ("reward-from-q", "reward-from-quest"):
        quests = listview(html, lid)
        if quests:
            r = quests[0]
            return ("quest", r.get("id", 0), clean_name(r.get("name", "")), 0)
    inside = listview(html, "contained-in-item")
    if inside:
        return ("object", 0, clean_name(inside[0].get("name", "")), 0)
    return None


def is_pvp(it):
    stats = it.get("stats") or []
    resil = stats[27] if len(stats) > 27 else 0  # proto stat enum: Resilience
    return resil > 0 or "Gladiator" in it["name"] or "Battlemaster" in it["name"]


def main():
    db = load_db()
    covered = emblem_ids()
    orphans = sorted(
        it["id"]
        for it in db["items"]
        if (
            # upstream sources exist but are all Classic-only (stripped)
            (it.get("sources") and not [s for s in it["sources"] if survives_strip(s)])
            # or upstream has no source at all: quest rewards, world
            # drops, unobtainable leftovers. Excluded: PvP gear (honor/
            # arena routes live in the PvP tables) and Sanctified tier
            # (token+mark trades wotlkdb can't list — RaidGated handles
            # them; without this they'd be wrongly marked unobtainable)
            or (
                not it.get("sources")
                and it.get("ilvl", 0) >= 200
                and not is_pvp(it)
                and not it["name"].startswith("Sanctified ")
            )
        )
        and it["id"] not in covered
    )
    print(f"{len(orphans)} orphaned items to resolve", file=sys.stderr)

    entries = {}
    misses = []
    for n, iid in enumerate(orphans, 1):
        try:
            src = best_source(fetch_item(iid))
        except Exception as e:  # noqa: BLE001 - a miss is logged, not fatal
            print(f"  {iid}: fetch failed: {e}", file=sys.stderr)
            src = None
        if src:
            entries[iid] = src
        else:
            misses.append(iid)
        if n % 25 == 0:
            print(f"  {n}/{len(orphans)}", file=sys.stderr)

    print("// Code generated by tools/gen_sources.py from wotlkdb.com")
    print("// (3.3.5a database); DO NOT EDIT.")
    print()
    print("package db")
    print()
    print("var supplementalSources = map[int]genSource{")
    for iid in sorted(entries):
        kind, npc, name, zone = entries[iid]
        name = name.replace('"', '\\"')
        print(f'\t{iid}: {{"{kind}", {npc}, "{name}", {zone}}},')
    print("}")
    print()
    print("// Items wotlkdb (the 3.3.5a world database) cannot source either:")
    print("// data-only leftovers and WotLK-Classic-era additions. Dropped from")
    print("// the pool at load — nothing on a 3.3.5 server can drop them.")
    print("var unobtainableItems = []int{")
    for iid in misses:
        print(f"\t{iid},")
    print("}")
    if misses:
        print(f"no 3.3.5 source found (emitted as unobtainable): {misses}", file=sys.stderr)


if __name__ == "__main__":
    main()
