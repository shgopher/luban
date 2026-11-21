#!/bin/bash
REPO=/Users/shgopher/Desktop/family/luban
export GOMODCACHE=$REPO/.cache/ch66/gomodcache GOCACHE=$REPO/.cache/gocache GOBIN=$REPO/.cache/ch66/bin GOFLAGS=-mod=mod GOWORK=off
for s in s661 s662 s663 s664 s665; do
  d=$REPO/.cache/ch66/$s/code
  echo "########## $s ##########"
  if [ -f "$d/go.mod" ]; then head -3 "$d/go.mod"; fi
  echo "--- go build ./... ---"
  ( cd "$d" && go build ./... 2>&1 | sed "s|^|  |" ; echo "  [build exit ${PIPESTATUS[0]}]" )
  echo "--- go vet ./... ---"
  ( cd "$d" && go vet ./... 2>&1 | sed "s|^|  |" ; echo "  [vet exit ${PIPESTATUS[0]}]" )
done
