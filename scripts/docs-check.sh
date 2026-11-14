#!/usr/bin/env bash
# Checks the project record for the mechanical failure modes — the ones a
# grep catches and a reading pass misses. Judgment-level drift (a claim
# that quietly went stale) is the reconcile pass's job, not this.
#
# Every check below reports through note(), which sets `fail`. That means
# no check may run inside a subshell: `cmd | while read` forks, so a note()
# in there prints its warning and loses the flag, and the script exits 0
# having just told you what's broken. Four of these seven checks shipped
# that way. Use `while read; do ...; done < <(cmd)` instead — same loop,
# current shell, flag survives.
set -uo pipefail
cd "$(dirname "$0")/.."
fail=0
note() { echo "  $1"; fail=1; }

LIVE_MD=(CLAUDE.md ARCHITECTURE.md ROADMAP.md LOG.md DECISIONS.md GOTCHAS.md README.md)

echo "1. ID definitions"
defD=$(grep -o '^- \*\*D[0-9]\+\*\*' DECISIONS.md | grep -o 'D[0-9]\+' | sort -u)
defG=$(grep -o '^### G[0-9]\+' GOTCHAS.md | grep -o 'G[0-9]\+' | sort -u)
defT=$( { grep -o '\*\*T[0-9]\+[a-z]*\*\*' ROADMAP.md; ls handovers/T*.md 2>/dev/null; } \
        | grep -o 'T[0-9]\+[a-z]*' | sort -u)
echo "   $(echo "$defD" | grep -c .) decisions, $(echo "$defG" | grep -c .) gotchas, $(echo "$defT" | grep -c .) live tasks"

# Duplicate IDs would make a pointer ambiguous.
for f in "DECISIONS.md:^- \*\*D[0-9]\+\*\*" "GOTCHAS.md:^### G[0-9]\+"; do
  dup=$(grep -o "${f#*:}" "${f%%:*}" | grep -o '[DG][0-9]\+' | sort | uniq -d)
  [ -n "$dup" ] && note "duplicate IDs in ${f%%:*}: $dup"
done

echo "2. Dangling references (live ID range only; T1-T57 resolve in archive/)"
for id in $(grep -rhoE '\b[DGT][0-9]+[a-z]?\b' --include='*.go' --include='*.md' \
            internal/ cmd/ platform/ docs/ handovers/ "${LIVE_MD[@]}" 2>/dev/null | sort -u); do
  n=${id//[!0-9]/}
  case "$id" in
    D*) echo "$defD" | grep -qx "$id" || note "dangling $id (no such decision)" ;;
    G*) echo "$defG" | grep -qx "$id" || note "dangling $id (no such gotcha)" ;;
    T*) [ "$n" -ge 58 ] && { echo "$defT" | grep -qx "$id" || note "dangling $id (no such task)"; } ;;
  esac
done

echo "3. ID formatting"
# An ID must be its own bold span, or every grep above silently misses it.
while read -r l; do note "ID not its own bold span: ${l:0:80}"; done \
  < <(grep -n '^- \*\*[DT][0-9]\+[a-z]* [^*]' ROADMAP.md DECISIONS.md 2>/dev/null)

echo "4. Relative links"
for f in "${LIVE_MD[@]}" handovers/*.md; do
  [ -e "$f" ] || continue
  while read -r l; do
    t="${l%%#*}"; [ -z "$t" ] && continue
    [ -e "$(dirname "$f")/$t" ] || [ -e "$t" ] || note "broken link in $f -> $l"
  done < <(grep -ohE '\]\([^)]+\)' "$f" | tr -d '])' | sed 's/^(//' | grep -v '^http')
done

echo "5. References to archived files from live files"
while read -r l; do note "source cites archived doc: ${l:0:90}"; done \
  < <(grep -rnE '(ATOMIC_TASKS|BACKLOG|FEATURES|CONCEPTUAL_GUIDE|DESIGN_[A-Z_]+)\.md' \
      --include='*.go' internal/ cmd/ platform/ 2>/dev/null | grep -v 'archive/')

echo "6. Claims in CLAUDE.md"
for t in $(grep -oE '^\| `task [a-z-]+`' CLAUDE.md | grep -oE 'task [a-z-]+' | cut -d' ' -f2 | sort -u); do
  grep -qE "^  $t:" Taskfile.yaml || note "CLAUDE.md names 'task $t', Taskfile has no such target"
done
gov=$(grep -oE '^go [0-9.]+' go.mod | cut -d' ' -f2)
grep -q "Go $gov" CLAUDE.md || note "CLAUDE.md's Go version disagrees with go.mod ($gov)"

echo "7. Append-only files carry no row counts in prose"
while read -r l; do note "row count in prose (drifts): ${l:0:70}"; done \
  < <(grep -nE '\([0-9]+ rows\)' "${LIVE_MD[@]}" handovers/*.md 2>/dev/null)

if [ "$fail" -eq 0 ]; then echo; echo "record check: clean"; else echo; echo "record check: issues above"; fi
exit "$fail"
