#!/usr/bin/env python3
"""Generate the Forever recipe routes -> internal/db/foreverrecipes_gen.go:

  foreverRecipes       craft spell -> how its recipe is learned (the
                       Alliance-side or neutral text)
  foreverRecipesHorde  craft spell -> the Horde side's Where, only where
                       it differs (ACA vs Durotar Supply and Logistics,
                       side-locked city vendors)

Routes, best first per craft (a craft taught by several recipe items
takes the most convenient one):

  1. the profession camps: the Azeroth Commerce Authority (Alliance,
     Three Corners) and its Horde twin Durotar Supply and Logistics sell
     the same recipes for Merchant's Favor. The list is a snapshot of
     Wowhead's ACA guide in tools/data/forever_aca_recipes.json (one
     page, refreshed by hand with --aca-snapshot); the client knows
     nothing about vendors.
  2. reputation recipes: the requirement is client data, exact
     (ItemSparse.MinFactionID / MinReputation); the selling vendor
     comes from QuestieDB when it knows one.
  3. QuestieDB vendors, quest rewards, named drops (up to MAX_NAMED
     NPCs), and world drops (more NPCs, or objects).
  4. crafts no recipe item teaches: the trainer.

Recipes none of these know stay out of the table: "source unknown" is
honest, a guess is not.

Bulk upstreams only: wago.tools client CSVs at the Forever build,
QuestieDB pinned at QUESTIE_COMMIT (GPLv3), the vendored wowsims data.

Usage:
    python3 tools/gen_forever_recipes.py | gofmt > internal/db/foreverrecipes_gen.go
    python3 tools/gen_forever_recipes.py --aca-snapshot guide.html  # refresh the ACA list

Env:
    WAGO_CSV     directory caching wago.tools CSVs (per-build subdirs)
    QUESTIE_LUA  directory caching the QuestieDB Forever Lua tables
"""

import csv
import gzip
import io
import json
import os
import re
import sys
import urllib.request

from gen_forever_worlddrops import QUESTIE_COMMIT, lua_fields, ids

BUILD = "1.60.1.71091"
WAGO = "https://wago.tools/db2/{}/csv?build={}"
QUESTIE = "https://raw.githubusercontent.com/Questie/QuestieDB/{}/data/Forever/{}.lua"
HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "..", "internal", "db", "data")
ACA = os.path.join(HERE, "data", "forever_aca_recipes.json")
UA = {"User-Agent": "twolk-gen-forever-recipes/1.0"}

ITEM_SPELLTRIGGER_LEARN = 6
MAX_NAMED = 3
RANKS = {3: "Neutral", 4: "Friendly", 5: "Honored", 6: "Revered", 7: "Exalted"}
# the camps render abbreviated; the screen's legend spells them out
# (db.ForeverRules().Legend)
ALLIANCE_CAMP = "ACA"
HORDE_CAMP = "DSL"

# QuestieDB keys, 0-based
ITEM_NPC_DROPS, ITEM_OBJECT_DROPS, ITEM_QUEST_REWARDS, ITEM_VENDORS = 1, 2, 5, 13
NPC_NAME, NPC_ZONE, NPC_FRIENDLY = 0, 8, 12
QUEST_NAME = 0


def fetch(url):
    with urllib.request.urlopen(urllib.request.Request(url, headers=UA), timeout=300) as r:
        return r.read().decode("utf-8", "replace")


def cached(env, sub, name, url):
    root = os.environ.get(env)
    fp = root and os.path.join(root, sub, name)
    if fp and os.path.exists(fp):
        with open(fp, encoding="utf-8") as f:
            return f.read()
    text = fetch(url)
    if fp:
        os.makedirs(os.path.dirname(fp), exist_ok=True)
        with open(fp, "w", encoding="utf-8") as f:
            f.write(text)
    return text


def wago(table):
    text = cached("WAGO_CSV", BUILD, table + ".csv", WAGO.format(table, BUILD))
    return list(csv.DictReader(io.StringIO(text)))


def questie(table):
    lua = cached("QUESTIE_LUA", QUESTIE_COMMIT, table + ".lua", QUESTIE.format(QUESTIE_COMMIT, table))
    return {int(m.group(1)): lua_fields(m.group(2)) for m in re.finditer(r"^\[(\d+)\] = \{(.*)$", lua, re.M)}


def unquote(s):
    s = s.strip()
    if len(s) >= 2 and s[0] in "'\"" and s[-1] == s[0]:
        s = s[1:-1]
    return s.replace("\\'", "'").replace('\\"', '"')


def field(f, i):
    return f[i] if len(f) > i else "nil"


def aca_snapshot(path):
    """Extract the profession vendors' recipe tables from a saved copy of
    Wowhead's ACA guide (www.wowhead.com/forever/guide/azeroth-commerce-
    authority-reputation-professions)."""
    s = open(path, encoding="utf-8").read().replace("\\/", "/")
    sparse = {int(r["ID"]): r for r in wago("ItemSparse")}
    out = []
    for m in re.finditer(r"\[npc=(\d+)\][^\[]*?\[center\]\[table(.*?)\[/table\]", s, re.S):
        rows = re.findall(r"\[tr\]\[td\]\[item=(\d+)\]\[/td\]\[td\](\d+)\[/td\]\[td\](\d+)\[/td\]\[/tr\]", m.group(2))
        for it, skill, cost in rows:
            r = sparse.get(int(it))
            if r and r["RequiredSkill"] != "0":
                out.append({"id": int(it), "name": r["Display_lang"], "skill": int(skill),
                            "cost": int(cost), "npc": int(m.group(1))})
    with open(ACA, "w", encoding="utf-8") as f:
        json.dump(sorted(out, key=lambda o: o["id"]), f, indent=1)
    print(f"{len(out)} ACA recipes -> {ACA}", file=sys.stderr)


def crafted_spells():
    spells = set()
    for name in ("forever-db.json.gz", "forever-leftover-db.json.gz"):
        with gzip.open(os.path.join(DATA, name), "rt", encoding="utf-8") as f:
            doc = json.load(f)
            zones = {z["id"]: z["name"] for z in doc["zones"]}
            for it in doc["items"]:
                for s in it.get("sources", []):
                    c = s.get("crafted")
                    if c and c.get("spellId"):
                        spells.add(c["spellId"])
    return spells, zones


def main():
    if len(sys.argv) == 3 and sys.argv[1] == "--aca-snapshot":
        return aca_snapshot(sys.argv[2])
    crafts, zones = crafted_spells()
    sparse = {int(r["ID"]): r for r in wago("ItemSparse")}
    effects = {int(r["ID"]): r for r in wago("ItemEffect")}
    factions = {int(r["ID"]): r["Name_lang"] for r in wago("Faction")}
    trainable = {int(r["Spell"]) for r in wago("SkillLineAbility")}
    teach = {}
    for x in wago("ItemXItemEffect"):
        e = effects.get(int(x["ItemEffectID"]))
        it = sparse.get(int(x["ItemID"]))
        if not e or not it or e["TriggerType"] != str(ITEM_SPELLTRIGGER_LEARN) or it["RequiredSkill"] == "0":
            continue
        sp = int(e["SpellID"])
        if sp in crafts:
            teach.setdefault(sp, set()).add(int(x["ItemID"]))
    with open(ACA, encoding="utf-8") as f:
        aca = {o["id"]: o for o in json.load(f)}
    qitems, qnpcs, qquests = questie("foreverItemDB"), questie("foreverNpcDB"), questie("foreverQuestDB")

    def npc_names(npcs, side):
        """names of the vendors friendly to side ("A"/"H"), else all"""
        rows = [qnpcs[n] for n in npcs if n in qnpcs]
        mine = [f for f in rows if side in unquote(field(f, NPC_FRIENDLY))]
        return sorted({unquote(f[NPC_NAME]) for f in (mine or rows)})

    def named(names):
        return " / ".join(names[:MAX_NAMED]) + (f" +{len(names) - MAX_NAMED}" if len(names) > MAX_NAMED else "")

    def route(item):
        """(priority, kind, where, horde where, rep) for one recipe item; None unknown"""
        r = sparse[item]
        if item in aca:
            cost = f" · {aca[item]['cost']} Favor"
            return 0, "RecipeVendor", ALLIANCE_CAMP + cost, HORDE_CAMP + cost, ""
        q = qitems.get(item)
        vendors = ids(field(q, ITEM_VENDORS)) if q else []
        fid = int(r["MinFactionID"])
        if fid:
            rep = f"{factions.get(fid, f'faction {fid}')} — {RANKS.get(int(r['MinReputation']), 'Neutral')}"
            a, h = npc_names(vendors, "A"), npc_names(vendors, "H")
            return 1, "RecipeVendor", named(a) or "quartermaster", named(h) or "quartermaster", rep
        if not q:
            return None
        if vendors:
            return 2, "RecipeVendor", named(npc_names(vendors, "A")), named(npc_names(vendors, "H")), ""
        quests = [unquote(qquests[i][QUEST_NAME]) for i in ids(field(q, ITEM_QUEST_REWARDS)) if i in qquests]
        if quests:
            w = named(sorted(set(quests)))
            return 3, "RecipeQuest", w, w, ""
        npcs = [n for n in ids(field(q, ITEM_NPC_DROPS)) if n in qnpcs]
        objects = ids(field(q, ITEM_OBJECT_DROPS))
        if npcs and len(npcs) <= 3 and not objects:
            names = sorted({unquote(qnpcs[n][NPC_NAME]) for n in npcs})
            zs = sorted({zones[int(field(qnpcs[n], NPC_ZONE))] for n in npcs
                         if field(qnpcs[n], NPC_ZONE).isdigit() and int(field(qnpcs[n], NPC_ZONE)) in zones})
            w = named(names) + (" · " + " / ".join(zs) if zs else "")
            return 4, "RecipeDrop", w, w, ""
        if npcs or objects:
            return 5, "RecipeWorldDrop", "", "", ""
        return None

    table, unknown = {}, 0
    for sp in sorted(crafts):
        items = teach.get(sp)
        if not items:
            if sp in trainable:
                table[sp] = ("", "RecipeTrainer", "", "", "")
            continue
        best = None
        for item in sorted(items):
            rt = route(item)
            if rt and (best is None or rt[0] < best[1][0]):
                best = (item, rt)
        if best is None:
            unknown += 1
            continue
        item, (_, kind, where, horde, rep) = best
        table[sp] = (sparse[item]["Display_lang"], kind, where, horde, rep)

    def go(s):
        return json.dumps(s, ensure_ascii=False)

    out = sys.stdout
    out.write("// Code generated by tools/gen_forever_recipes.py from wago.tools client tables\n")
    out.write(f"// (build {BUILD}), QuestieDB ({QUESTIE_COMMIT[:12]}, GPLv3), the ACA snapshot in\n")
    out.write("// tools/data/forever_aca_recipes.json and the vendored wowsims data; DO NOT EDIT.\n\n")
    out.write("package db\n\n")
    out.write(f"// foreverRecipes: {len(table)} crafts; {unknown} recipe items with no known route are left out.\n")
    out.write("var foreverRecipes = map[int]RecipeSource{\n")
    for sp, (name, kind, where, _, rep) in sorted(table.items()):
        out.write(f"\t{sp}: {{{go(name)}, {kind}, {go(where)}, {go(rep)}, 0}},\n")
    out.write("}\n\n")
    out.write("// foreverRecipesHorde: the Horde side's Where where it differs.\n")
    out.write("var foreverRecipesHorde = map[int]string{\n")
    for sp, (_, _, where, horde, _) in sorted(table.items()):
        if horde != where:
            out.write(f"\t{sp}: {go(horde)},\n")
    out.write("}\n")
    kinds = {}
    for v in table.values():
        kinds[v[1]] = kinds.get(v[1], 0) + 1
    print(f"{len(table)} routes {kinds}, {unknown} unknown", file=sys.stderr)


if __name__ == "__main__":
    main()
