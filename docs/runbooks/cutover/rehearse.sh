#!/bin/bash
# Rehearsal of the per-project switch on a COPY. Never pass a live project.
# usage: rehearse.sh <copy-project-dir (absolute)> <lite-bd.exe (absolute)> <upstream-bd.exe (absolute)>
# Writes a timing line per step to stdout ("T <seconds> <step>"). Needs: the copy has .beads/ and a git-less folder is fine.
set -u
P=$1; LITE=$2; UP=$3
case "$P" in /*|[A-Za-z]:*) ;; *) echo "project dir must be absolute"; exit 2;; esac
case "$P" in *Projects/rayeh|*Projects/fatura|*Projects/abduu-tasks|*/company|*Projects/whisper-flow) echo "refusing: looks like a live project"; exit 2;; esac
export BD_ACTOR=sre
cd "$P" || exit 2
W="$P/.lite-work"; mkdir -p "$W"
t() { local n=$1; shift; local s=$(date +%s.%N); "$@" > "$W/$n.out" 2>&1; local rc=$?; local e=$(date +%s.%N); printf 'T %.2f %s rc=%s\n' "$(echo "$e - $s" | bc -l 2>/dev/null || awk "BEGIN{print $e-$s}")" "$n" "$rc"; return 0; }
b() { "$LITE" "$@"; }
export BD_LITE_UPSTREAM="$UP"
smoke() { # $1 = label
  local L=$1
  t "$L-list"   b list --json
  ID=$(b list --json -n 1 2>/dev/null | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{try{const a=JSON.parse(s);console.log((Array.isArray(a)?a:a.issues)[0].id)}catch(e){}})')
  t "$L-show"   b show "$ID" --json
  t "$L-ready"  b ready --json
  t "$L-create" b create "rehearsal $L" -p 3 --json
  NID=$(grep -m1 -o '"id": *"[^"]*"' "$W/$L-create.out" | cut -d'"' -f4)
  t "$L-update"   b update "$NID" --priority 2
  t "$L-comment"  b comments add "$NID" "rehearsal comment"
  t "$L-close"    b close "$NID" --reason "rehearsal"
  t "$L-showback" b show "$NID" --json
  echo "I $L smoke id=$ID new=$NID"
}
echo "I backend before: $(grep -o '"backend": *"[a-z]*"' .beads/metadata.json)"
smoke pre-dolt
t announce b lite-switch announce --at 23:59 --project rehearsal --dir "$P"
t status1   b lite-switch status
t ack       b lite-switch ack --as sre
t freeze    b lite-switch freeze --force
t write-while-frozen b create "must be refused" -p 4
echo "I write-while-frozen output: $(head -c 200 $W/write-while-frozen.out | tr '\n' ' ')"
t upstream-while-frozen "$UP" list --json -n 1
echo "I upstream-while-frozen output: $(head -c 200 $W/upstream-while-frozen.out | tr '\n' ' ')"
t migrate   b migrate-to-sqlite --bd "$UP" --work-dir "$W/migrate" --report "$W/migrate-report.json"
tail -5 "$W/migrate.out"
t flip      b lite-switch flip
echo "I backend after flip: $(grep -o '"backend": *"[a-z]*"' .beads/metadata.json)"
smoke post-flip
t rollback  b lite-switch rollback
echo "I backend after rollback: $(grep -o '"backend": *"[a-z]*"' .beads/metadata.json); archive: $(ls .beads/lite-archive 2>/dev/null)"
smoke post-rollback
# second round: freeze, migrate, flip again (needs no beads.db left over)
t announce2 b lite-switch announce --at 23:59 --project rehearsal --dir "$P"
t freeze2   b lite-switch freeze --force
t migrate2  b migrate-to-sqlite --bd "$UP" --work-dir "$W/migrate2" --report "$W/migrate2-report.json"
t flip2     b lite-switch flip
smoke post-flip2
echo "I done"
