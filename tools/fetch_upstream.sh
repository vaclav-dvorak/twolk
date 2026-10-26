#!/bin/sh
# Re-vendor the embedded WoWSims data (internal/db/data/) at the commit
# pinned in internal/db/load.go. Run after bumping the pin.
set -eu
COMMIT=$(sed -n 's/const pinnedCommit = "\(.*\)"/\1/p' internal/db/load.go)
BASE="https://raw.githubusercontent.com/wowsims/wotlk/$COMMIT"
echo "vendoring wowsims@$COMMIT"
curl -fsS "$BASE/assets/database/db.json" | gzip -9 > internal/db/data/db.json.gz
curl -fsS "$BASE/sim/common/wotlk/stat_bonus_procs.go" > internal/db/data/stat_bonus_procs.go.txt
curl -fsS "$BASE/sim/common/wotlk/stat_bonus_stacking.go" > internal/db/data/stat_bonus_stacking.go.txt
curl -fsS "$BASE/sim/common/wotlk/stat_bonus_cds.go" > internal/db/data/stat_bonus_cds.go.txt
ls -la internal/db/data/
