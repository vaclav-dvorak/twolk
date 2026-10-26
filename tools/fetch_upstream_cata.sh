#!/bin/sh
# Re-vendor the embedded WoWSims Cataclysm data (internal/db/data/) at
# the commit pinned in internal/db/loadcata.go. Run after bumping the pin.
set -eu
COMMIT=$(sed -n 's/const pinnedCataCommit = "\(.*\)"/\1/p' internal/db/loadcata.go)
BASE="https://raw.githubusercontent.com/wowsims/cata/$COMMIT"
echo "vendoring wowsims/cata@$COMMIT"
curl -fsS "$BASE/assets/database/db.json" | gzip -9 > internal/db/data/cata-db.json.gz
# proc trigger masks (melee/ranged/spell) — absent from db.json, sim source only
curl -fsS "$BASE/sim/common/cata/stat_bonus_procs.go" > internal/db/data/cata_stat_bonus_procs.go.txt
curl -fsS "$BASE/sim/common/cata/stat_bonus_stacking.go" > internal/db/data/cata_stat_bonus_stacking.go.txt
ls -la internal/db/data/
