#!/bin/bash
# Switch ONE live project to beads-lite. Run by SRE after the CoS has told that project's chats to stop.
#   project.sh <name> <absolute project dir>            # announce -> ack -> freeze -> migrate -> flip -> smoke; auto-rollback if migrate/flip/smoke fails
#   project.sh <name> <absolute project dir> rollback   # back to dolt (archives beads.db, never deletes it)
# Needs the lite bd installed by shim.sh (or LITE=<abs path> to use a binary directly) and the pristine upstream at bd-upstream.exe.
# Every path is ABSOLUTE (relative --work-dir/--report break: the freeze holder and the extractor run from other folders).
set -u
NAME=${1:?name}; DIR=${2:?absolute project dir}; MODE=${3:-switch}
case "$DIR" in /*|[A-Za-z]:*) ;; *) echo "dir must be absolute"; exit 2;; esac
[ -d "$DIR/.beads/embeddeddolt" ] || [ "$MODE" = rollback ] || { echo "$DIR/.beads/embeddeddolt missing: not a dolt project"; exit 2; }
SHIM_DIR=${SHIM_DIR:-"$APPDATA/npm/node_modules/@beads/bd/bin"}
BD=${LITE:-"$SHIM_DIR/bd.exe"}; UP=${UP:-"$SHIM_DIR/bd-upstream.exe"}
export BD_ACTOR=sre BD_LITE_UPSTREAM="$UP"
STAMP=$(date +%Y%m%d-%H%M%S)
WORK="E:/Tools/bd-cutover/work/$NAME-$STAMP"; mkdir -p "$WORK"
LOGF="$WORK/switch.log"
step() { local n=$1; shift; local s=$(date +%s); "$@" > "$WORK/$n.out" 2>&1; local rc=$?; echo "$(date +%T) $n rc=$rc $(( $(date +%s) - s ))s" | tee -a "$LOGF"; return $rc; }
cd "$DIR" || exit 2
if [ "$MODE" = rollback ]; then step rollback "$BD" lite-switch rollback; exit $?; fi
back() { echo "FAILED at $1: rolling back" | tee -a "$LOGF"; "$BD" lite-switch rollback >> "$LOGF" 2>&1; echo "rollback rc=$?" | tee -a "$LOGF"; echo "output: $WORK/$1.out"; exit 1; }
echo "== $NAME $DIR -> lite ($BD) =="
[ ! -f .beads/beads.db ] || { echo "beads.db already present: refusing (archive or inspect first)"; exit 2; }
step pre-count "$BD" list --json -n 0 --all || true
step announce  "$BD" lite-switch announce --at "$(date +%H:%M)" --project "$NAME" --dir "$DIR" || exit 1
step ack       "$BD" lite-switch ack --as sre
step freeze    "$BD" lite-switch freeze --force || back freeze
step migrate   "$BD" migrate-to-sqlite --bd "$UP" --work-dir "$WORK/extract" --report "$WORK/migrate-report.json" || back migrate
step flip      "$BD" lite-switch flip || back flip
# smoke on the live project (leaves one closed 'cutover smoke' bead, intentional evidence)
step s-list    "$BD" list --json -n 5 || back s-list
step s-ready   "$BD" ready --json   || back s-ready
step s-create  "$BD" create "cutover smoke $STAMP" -p 4 --json || back s-create
ID=$(grep -m1 -o '"id": *"[^"]*"' "$WORK/s-create.out" | cut -d'"' -f4)
[ -n "$ID" ] || back s-create
step s-show    "$BD" show "$ID" --json                      || back s-show
step s-update  "$BD" update "$ID" --priority 3              || back s-update
step s-comment "$BD" comments add "$ID" "cutover smoke comment" || back s-comment
step s-close   "$BD" close "$ID" --reason "cutover smoke"   || back s-close
grep -q '"backend": *"sqlite"' .beads/metadata.json || back flip
echo "OK $NAME on sqlite. restart: board, Telegram bridge, any long-running caller of this project. report: $WORK/migrate-report.json"
