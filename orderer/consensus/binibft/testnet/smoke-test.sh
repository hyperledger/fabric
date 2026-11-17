#!/usr/bin/env bash
# End-to-end smoke test of the BiniBFT orderer plugin on a 4-orderer localhost network.
#
#   bash smoke-test.sh            # builds nothing; uses ~/bin/orderer-binibft and ~/bin/binibft-testclient
#   BUILD=1 bash smoke-test.sh    # (re)build both binaries first
#
# It stops any running test orderers, wipes the test ledgers, starts a fresh
# network, and prints PASS/FAIL per check. Exit code is 0 only if all pass.
set -uo pipefail

export PATH="$HOME/opt/go/bin:$HOME/bin:$PATH"
HERE="$(cd "$(dirname "$0")" && pwd)"
FABRIC="$(cd "$HERE/../../../.." && pwd)"
cd "$HERE"

CHANNEL=mychannel
O=crypto-config/ordererOrganizations/example.com
ADMIN=$O/users/Admin@example.com/msp
CA=$O/orderers/orderer1.example.com/tls/ca.crt
CLIENT="$HOME/bin/binibft-testclient"
FAILED=0

pass() { echo "  PASS  $*"; }
fail() { echo "  FAIL  $*"; FAILED=1; }
step() { echo; echo "== $*"; }

height() { # $1 = orderer number -> prints height or empty
  "$HOME/bin/osnadmin" channel list --channelID "$CHANNEL" -o "127.0.0.1:$((7949 + $1))" 2>&1 |
    grep -o '"height": *[0-9]*' | grep -o '[0-9]*$' | head -1
}
broadcast() { # $1 = orderer number, $2 = count
  "$CLIENT" broadcast -server "127.0.0.1:$((7049 + $1))" -channel "$CHANNEL" -ca "$CA" \
    -mspdir "$ADMIN" -mspid OrdererMSP -n "$2" 2>&1 | grep -E "broadcast OK|^ERROR"
}
verify() { # $1 = orderer number, $2 = last block
  "$CLIENT" verify -server "127.0.0.1:$((7049 + $1))" -channel "$CHANNEL" -ca "$CA" \
    -mspdir "$ADMIN" -mspid OrdererMSP -genesis genesis.block -from 1 -to "$2" 2>&1 |
    grep -E "^block |verify OK|^ERROR"
}

if [ "${BUILD:-0}" = "1" ]; then
  step "0. Build"
  ( cd "$FABRIC" &&
    go build -o "$HOME/bin/orderer-binibft" ./cmd/orderer &&
    go build -o "$HOME/bin/binibft-testclient" ./orderer/consensus/binibft/testclient &&
    for t in cryptogen configtxgen osnadmin; do go build -o "$HOME/bin/$t" "./cmd/$t" || exit 1; done ) \
    && pass "binaries built" || { fail "build failed"; exit 1; }
fi

for b in orderer-binibft binibft-testclient cryptogen configtxgen osnadmin; do
  [ -x "$HOME/bin/$b" ] || { echo "missing $HOME/bin/$b (run with BUILD=1)"; exit 2; }
done

step "1. Fresh network"
pkill -x orderer-binibft 2>/dev/null; sleep 2
rm -rf data logs
bash net.sh gen >/dev/null 2>&1 || { fail "net.sh gen"; exit 1; }
bash net.sh up >/dev/null 2>&1
sleep 5
ok=1
for i in 1 2 3 4; do
  h=$(height $i)
  [ "$h" = "1" ] || ok=0
done
[ $ok = 1 ] && pass "all 4 orderers joined and active at height 1" || fail "orderers did not all join (heights: $(height 1) $(height 2) $(height 3) $(height 4))"

if grep -q "primary 1, block quorum 3" logs/orderer1.log; then
  pass "BiniBFT chain started: primary 1, block quorum 3"
else
  fail "BiniBFT chain start line not found in logs/orderer1.log"
fi

step "2. Ordering through a non-primary orderer (forwarded to the primary)"
sent=20
broadcast 3 $sent | grep -q "broadcast OK" && pass "$sent transactions accepted by orderer3" || fail "broadcast to orderer3 failed"
sleep 5
hs="$(height 1) $(height 2) $(height 3) $(height 4)"
set -- $hs
if [ -n "$1" ] && [ "$1" = "$2" ] && [ "$2" = "$3" ] && [ "$3" = "$4" ] && [ "$1" -ge 2 ]; then
  pass "all orderers at the same height ($1)"
  LAST=$(($1 - 1))
else
  fail "heights differ or did not advance: $hs"
  LAST=1
fi

step "3. Blocks verify with Fabric's own BFT block verifier, hashes agree"
for i in 1 2 3 4; do verify $i $LAST > "/tmp/binibft-verify-$i.txt"; done
for i in 1 2 3 4; do
  if grep -q "verify OK" "/tmp/binibft-verify-$i.txt"; then
    pass "orderer$i: blocks 1..$LAST pass BFT block verification"
  else
    fail "orderer$i: block verification failed: $(grep ERROR /tmp/binibft-verify-$i.txt | head -1)"
  fi
done
# Signature counts may differ per orderer (each writes once it holds a quorum), so compare number and hash only.
norm() { grep "^block" "/tmp/binibft-verify-$1.txt" | awk '{print $2, $NF}'; }
agree=1
for i in 2 3 4; do [ "$(norm 1)" = "$(norm $i)" ] || agree=0; done
[ $agree = 1 ] && [ -n "$(norm 1)" ] && pass "identical block hashes and signature counts on all 4 orderers" || fail "block hashes differ between orderers"
minsig=$(grep "^block" /tmp/binibft-verify-1.txt | sed -E 's/.*signatures=([0-9]+).*/\1/' | sort -n | head -1)
[ "${minsig:-0}" -ge 3 ] && pass "every block carries >= 3 signatures (BFT quorum for n=4)" || fail "a block has fewer than 3 signatures (min=${minsig:-none})"
total=$(grep "^block" /tmp/binibft-verify-1.txt | sed -E 's/.*txs=([0-9]+).*/\1/' | paste -sd+ | bc)
[ "${total:-0}" = "$sent" ] && pass "transaction count matches: sent $sent, committed $total" || fail "transaction count mismatch: sent $sent, committed ${total:-0}"

step "4. Fault tolerance: one orderer down (f=1)"
before=$(height 1)
bash net.sh stop 4 >/dev/null 2>&1; sleep 2
broadcast 1 10 | grep -q "broadcast OK" && pass "10 transactions accepted with orderer4 down" || fail "broadcast failed with orderer4 down"
sleep 5
a=$(height 1); b=$(height 2); c=$(height 3)
if [ "$a" -gt "$before" ] && [ "$a" = "$b" ] && [ "$b" = "$c" ]; then
  pass "orderers 1-3 advanced together ($before -> $a) with orderer4 down"
else
  fail "orderers 1-3 did not advance together: before=$before now=$a $b $c"
fi

step "Cleanup"
bash net.sh down >/dev/null 2>&1
echo "  (network stopped; logs in $HERE/logs)"

echo
if [ $FAILED = 0 ]; then echo "RESULT: ALL CHECKS PASSED"; else echo "RESULT: SOME CHECKS FAILED"; fi
exit $FAILED
