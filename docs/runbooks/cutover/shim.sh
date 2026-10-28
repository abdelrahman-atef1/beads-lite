#!/bin/bash
# Replace / restore the bd every caller uses. NOT run by the rehearsal; run by SRE at cutover only.
#   shim.sh status
#   shim.sh install <absolute path to lite bd.exe>     # one command in
#   shim.sh rollback                                    # one command out (restores upstream byte-for-byte)
# Every caller (PATH 'bd' shim, learner/link-inbox 'node bd.js', beads-ui BD_BIN, backup scripts, Telegram bridge)
# ends at <npm>/node_modules/@beads/bd/bin/bd.js, which spawns the bd.exe next to it. Swapping that one file moves all of them.
# The fork keeps the Dolt backend, so projects not yet migrated behave as before (passthrough).
# DRYRUN=1 prints what would happen. SHIM_DIR overrides the folder (used to rehearse on a copy).
set -eu
SHIM_DIR=${SHIM_DIR:-"$APPDATA/npm/node_modules/@beads/bd/bin"}
UPX="$SHIM_DIR/bd-upstream.exe"       # pristine upstream 1.3.1, kept forever; migrate needs it as its source
LOG="$SHIM_DIR/shim-swap.log"
run() { if [ "${DRYRUN:-}" = 1 ]; then echo "DRYRUN: $*"; else "$@"; fi; }
log() { echo "$(date '+%F %T') $*" | tee -a "$LOG"; }
sha() { sha256sum "$1" | cut -d' ' -f1; }
case "${1:-}" in
status)
  echo "dir: $SHIM_DIR"; for f in bd.exe bd-upstream.exe; do [ -f "$SHIM_DIR/$f" ] && echo "$f $(sha "$SHIM_DIR/$f" | cut -c1-12) $("$SHIM_DIR/$f" version 2>&1 | head -1)"; done
  [ -f "$SHIM_DIR/bd.exe.upstream.sha" ] && echo "recorded upstream sha: $(cut -c1-12 "$SHIM_DIR/bd.exe.upstream.sha")"; exit 0;;
install)
  NEW=${2:?absolute path of lite bd.exe}
  case "$NEW" in /*|[A-Za-z]:*) ;; *) echo "path must be absolute"; exit 2;; esac
  [ -f "$NEW" ] || { echo "no such file $NEW"; exit 2; }
  "$NEW" version | grep -q "^bd version" || { echo "new binary does not run"; exit 3; }
  [ -f "$SHIM_DIR/bd.exe" ] || { echo "no bd.exe in $SHIM_DIR"; exit 3; }
  if [ -f "$UPX" ]; then
    [ "$(sha "$UPX")" = "$(cat "$SHIM_DIR/bd.exe.upstream.sha")" ] || { echo "bd-upstream.exe does not match recorded sha: stop"; exit 3; }
    [ "$(sha "$SHIM_DIR/bd.exe")" = "$(sha "$UPX")" ] && echo "(live bd.exe is upstream)" || echo "(live bd.exe is already something else; replacing it, upstream copy untouched)"
  else
    "$SHIM_DIR/bd.exe" version | grep -q "1.3.1" || { echo "live bd.exe is not upstream 1.3.1: stop"; exit 3; }
    run cp -p "$SHIM_DIR/bd.exe" "$UPX"; [ "${DRYRUN:-}" = 1 ] || sha "$UPX" > "$SHIM_DIR/bd.exe.upstream.sha"
  fi
  TS=$(date +%Y%m%d-%H%M%S)
  run cp "$NEW" "$SHIM_DIR/bd.exe.new"
  # rename-away works even while a long-running caller (the board) has bd.exe open; delete would not
  run mv "$SHIM_DIR/bd.exe" "$SHIM_DIR/bd.exe.old-$TS"
  run mv "$SHIM_DIR/bd.exe.new" "$SHIM_DIR/bd.exe"
  log "install: bd.exe <- $NEW ($( [ "${DRYRUN:-}" = 1 ] || sha "$SHIM_DIR/bd.exe" | cut -c1-12)); upstream kept at bd-upstream.exe; previous at bd.exe.old-$TS"
  [ "${DRYRUN:-}" = 1 ] || "$SHIM_DIR/bd.exe" version;;
rollback)
  [ -f "$UPX" ] || { echo "no bd-upstream.exe: nothing to restore"; exit 3; }
  [ "$(sha "$UPX")" = "$(cat "$SHIM_DIR/bd.exe.upstream.sha")" ] || { echo "bd-upstream.exe sha mismatch: stop"; exit 3; }
  TS=$(date +%Y%m%d-%H%M%S)
  run cp "$UPX" "$SHIM_DIR/bd.exe.new"
  [ -f "$SHIM_DIR/bd.exe" ] && run mv "$SHIM_DIR/bd.exe" "$SHIM_DIR/bd.exe.lite-$TS"
  run mv "$SHIM_DIR/bd.exe.new" "$SHIM_DIR/bd.exe"
  log "rollback: bd.exe <- upstream ($(sha "$UPX" | cut -c1-12)); lite kept at bd.exe.lite-$TS"
  [ "${DRYRUN:-}" = 1 ] || "$SHIM_DIR/bd.exe" version;;
*) echo "usage: $0 status | install <abs lite bd.exe> | rollback"; exit 2;;
esac
