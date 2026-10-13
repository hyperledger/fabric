#!/usr/bin/env bash
# Copies the BiniBFT consensus engine into the Fabric tree (Fabric vendors its deps, so the
# engine is vendored in-tree instead of imported as an external module).
set -euo pipefail
SRC="${1:-$HOME/dev/Hyperledger-BiniBFT}/consensus"
DST="${2:-$HOME/dev/fabric}/orderer/consensus/binibft/engine"
FAB=github.com/hyperledger/fabric/orderer/consensus/binibft/engine
rm -rf "$DST"; mkdir -p "$DST/protos"
for f in "$SRC"/*.go; do
  case "$f" in *_test.go) continue;; esac
  sed -e "s#^package consensus\$#package engine#" \
      -e "s#\"binibft-poc/consensus/protos\"#\"$FAB/protos\"#g" "$f" > "$DST/$(basename "$f")"
done
for f in "$SRC"/protos/*.go; do
  case "$f" in *_test.go) continue;; esac
  cp "$f" "$DST/protos/"
done
echo "engine synced to $DST ($(ls "$DST" | wc -l) files)"
