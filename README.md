<div align="center">

# ⚔️ twolk

**A terminal gear planner for World of Warcraft private servers**<br>
in the spirit of Ask Mr. Robot

[![Release](https://img.shields.io/github/v/release/vaclav-dvorak/twolk?style=flat-square)](https://github.com/vaclav-dvorak/twolk/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=flat-square)

</div>

Import your character and twolk scores your gear **as a whole set**, so
hit and expertise caps, set bonuses, gems, enchants and reforges all
count. Then it:

- 🎯 shows the best upgrade for each slot, and where it drops or who sells it
- 💎 tunes your gems, enchants and reforges
- 🏆 builds a best-in-slot list with a shopping list for your current raid phase

## 🎮 Supported game versions

| Game | Server / realm | Import |
|---|---|---|
| 🧊 **WotLK 3.3.5a** (legacy) | Warmane: Onyxia, Icecrown | 📋 paste (browser export) |
| 🔥 **Cataclysm 4.4.2** (classic) | Whitemane: Gilneas | ⚡ fetched directly from the armory API |

**WotLK:** all 10 classes, every talent tree.

**Cataclysm** scored specs:

| Class | Specs |
|---|---|
| 🏹 Hunter | Beast Mastery · Marksmanship · Survival |
| 🐻 Druid | Balance · Feral (cat) · Feral (bear) · Restoration |
| 🛡️ Warrior | Arms · Fury (Titan's Grip) · Fury (Single-Minded Fury) · Protection |
| 🔮 Mage | Arcane · Fire |
| 💀 Warlock | Affliction · Demonology · Destruction |

Other Cataclysm classes import and show their gear, but they aren't
scored yet.

🗓️ **Raid phases** for the upgrade filter:
- **WotLK:** Naxx (T7), Ulduar (T8), ToC (T9), ICC (T10), Ruby Sanctum.
- **Cataclysm:** T11, Zandalari, Firelands (T12), Dragon Soul (T13).

## ✨ Features

| Key | Screen | What it does |
|---|---|---|
| `enter` | 📈 **Upgrades** | Ranks every item for the selected slot by how much it would improve your whole set, filtered to your phase and to the sources you can reach (raid, dungeon, vendor, crafted). |
| `o` | ⚙️ **Optimize** | Solves gems, enchants and reforges together, since all three feed the same caps. `c` copies the suggestions. |
| `b` | 🏆 **BiS** | Builds the best-in-slot set for your phase and filters, with a shopping list. |
| `p` | 🪙 **Emblems** | Plans your currency spend (Emblems in WotLK, Justice and Valor in Cataclysm): which vendor items are worth buying. |
| `f` | 🔍 **Filter** | Sets your raid phase, loot routes and unlocks (such as the Molten Front vendors). Phase is saved per realm. |
| `w` | ⚖️ **Weights** | Adjusts the stat weights for your spec. Defaults come from the WoWSims sims. |
| `t` | 🌳 **Talents** | Sets talent points the import couldn't read (WotLK only, where the armory gives tree totals only). |
| `i` / `r` | ➕ **Add / Refresh** | Adds a character (realm and name), or loads its current gear from the armory. |
| `c` | 👥 **Characters** | Switches between or deletes imported characters. |

💡 The tips pane on the main screen points out quick fixes, such as a
missing enchant, an empty socket or an unused profession bonus.

## 🚀 Zero to hero

### 1. 📦 Install

Download the archive for your system from the
[releases page](https://github.com/vaclav-dvorak/twolk/releases/latest)
and unpack it:

| System | File |
|---|---|
| 🍎 macOS (Apple silicon) | `twolk_<version>_darwin_arm64.tar.gz` |
| 🐧 Linux (x86-64) | `twolk_<version>_linux_amd64.tar.gz` |
| 🪟 Windows (x86-64) | `twolk_<version>_windows_amd64.zip` |

Put the `twolk` binary (`twolk.exe` on Windows) somewhere on your
`PATH`.

> [!NOTE]
> The binary is not signed, so macOS blocks it on first launch. Clear
> the quarantine flag once with `xattr -d com.apple.quarantine twolk`.

🔌 The item databases are built into the binary, so twolk never needs
the network except to import characters.

### 2. ▶️ Start it

```sh
twolk
```

On first launch, twolk says *no character imported yet*. Press `i`.

### 3. 🧙 Add your character

Adding a character takes a realm and a name; its gear always comes in
through a **refresh**, which opens right after you add it (and later
with `r`).

1. Pick the realm (for example **Whitemane — Gilneas** or
   **Warmane — Onyxia**) and press `enter`.
2. Type the character name and press `enter`.

**🔥 Whitemane (Cataclysm):** the refresh fetches the character from the
armory straight away. It picks up gear, gems, enchants, reforges, exact
talents and professions.

**🧊 Warmane (WotLK):** the armory sits behind a browser bot check, so
the refresh goes through your browser:

1. Open the armory link twolk shows in your browser.
2. Back in twolk, press `y` to copy the export snippet to your clipboard.
3. In the browser, open the devtools console (`F12`, or `cmd+opt+J` on
   macOS), paste the snippet and press `enter`. It reads gems and
   enchants from the page and puts the export JSON on your clipboard.
4. Back in twolk, paste with `cmd+v` or `ctrl+v` and press `enter` to save.
5. The armory gives only talent tree totals, so if twolk asks, press
   `t` and set the talents that affect scoring.

### 4. 🛠️ Make it useful

1. 🗓️ **Set your phase:** press `f` and choose the raid tier your realm
   is on, and which loot routes you run. Every screen respects this
   filter.
2. 🔧 **Fix the basics:** read the tips pane, then press `o` and apply
   the gem, enchant and reforge suggestions in game.
3. 📈 **Find upgrades:** move to a slot and press `enter`. The top row
   is the best gain you can get right now, and each row shows where the
   item comes from.
4. 🏆 **Plan ahead:** press `b` for the full BiS set and shopping list,
   and `p` to decide what to spend your currency on.
5. 🔄 **Stay current:** after a raid night, press `r` to refresh. Press
   `c` to switch between characters.

🔒 Your characters and settings are saved in `~/.twolk/config.json`.
Character data stays on your machine.

## 🙏 Credits

- **[WoWSims](https://github.com/wowsims)**: this project would not
  exist without it. The item, gem and enchant databases, trinket and
  proc effects, and default stat weights come from
  [wowsims/wotlk](https://github.com/wowsims/wotlk) and
  [wowsims/cata](https://github.com/wowsims/cata) (MIT licensed),
  pinned to fixed commits and embedded under `internal/db/data/`.
  Cataclysm hunter, druid, warrior, mage and warlock stat weights,
  trinket procs and tier bonuses were checked by running the WoWSims
  cata simulator. Thank you to the WoWSims team.
- **[AzerothCore](https://github.com/azerothcore/azerothcore-wotlk)**
  and **[TrinityCore](https://github.com/TrinityCore/TrinityCore)**
  world databases supply vendor prices, recipe sources and 3.3.5 stat
  corrections.
- **[wago.tools](https://wago.tools)** supplies the client data tables
  for currency costs.
- **[Wowhead](https://www.wowhead.com)** and
  **[wotlkdb.com](https://wotlkdb.com)** supply item sources and
  vendor inventories, taken from bulk data dumps rather than scraped
  page by page.

The generators that build the `*_gen.go` tables live in `tools/`.

## 📄 License

MIT. See [LICENSE](LICENSE). The bundled WoWSims data stays under the
WoWSims MIT license.

World of Warcraft is a trademark of Blizzard Entertainment. twolk is a
fan project and isn't affiliated with Blizzard, WoWSims or any private
server.
