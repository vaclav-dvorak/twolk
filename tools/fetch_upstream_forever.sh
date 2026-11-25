#!/bin/sh
# Re-vendor the embedded WoWSims Forever data (internal/db/data/) at
# the commit pinned in internal/db/loadforever.go. Run after bumping the
# pin — Forever's item list keeps growing, so every release re-vendors.
#
# Both files are needed: db.json holds only the items a level-60 sim
# cares about; leftover_db.json holds the rest (low-level greens, white
# gear), which leveling characters wear.
set -eu
COMMIT=$(sed -n 's/const pinnedForeverCommit = "\(.*\)"/\1/p' internal/db/loadforever.go)
BASE="https://raw.githubusercontent.com/wowsims/forever/$COMMIT"
echo "vendoring wowsims/forever@$COMMIT"
curl -fsS "$BASE/assets/database/db.json" | gzip -9 > internal/db/data/forever-db.json.gz
curl -fsS "$BASE/assets/database/leftover_db.json" | gzip -9 > internal/db/data/forever-leftover-db.json.gz
ls -la internal/db/data/
echo "now regenerate the craft tables for the same build:"
echo "  python3 tools/gen_forever_crafts.py <build> | gofmt > internal/db/forevercrafts_gen.go"
echo "  python3 tools/gen_forever_worlddrops.py | gofmt > internal/db/foreverworlddrops_gen.go"
