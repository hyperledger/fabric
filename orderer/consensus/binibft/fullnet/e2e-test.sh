#!/usr/bin/env bash
# End-to-end test: a complete Fabric network (N BiniBFT orderers, 2 peer orgs,
# chaincode, client application) running the execute-order-validate flow.
#
#   bash e2e-test.sh          # N=5 orderers (default)
#   N=7 bash e2e-test.sh      # any odd number: 3, 5, 7, ...
#   KEEP=1 bash e2e-test.sh   # leave the network running afterwards
#   first time on a new machine: bash net.sh build
#
# Prints PASS/FAIL per check; exit code 0 only if every check passes.
set -uo pipefail

BIN="${BIN:-$HOME/bin}"; export BIN
export PATH="$HOME/opt/go/bin:$BIN:$PATH"
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE"
N="${N:-5}"
F=$(( (N - 1) / 3 ))
Q=$(( (N + F + 2) / 2 ))
CHANNEL=mychannel
O=crypto-config/ordererOrganizations/example.com
OADMIN=$O/users/Admin@example.com/msp
OCA=$O/orderers/orderer1.example.com/tls/ca.crt
CLIENT="$BIN/binibft-e2e-client"
FAILED=0

pass() { echo "  PASS  $*"; }
fail() { echo "  FAIL  $*"; FAILED=1; }
step() { echo; echo "== $*"; }
nocolor() { sed 's/\x1b\[[0-9;]*m//g'; }

o_height() { osnadmin channel list --channelID "$CHANNEL" -o "127.0.0.1:$((9150 + $1))" 2>&1 |
  tr -d '\n\t ' | grep -o '"height":[0-9]*' | grep -o '[0-9]*$'; }
p_height() { ( source <(bash net.sh env); as_org "$1"; peer channel getinfo -c "$CHANNEL" 2>/dev/null |
  grep -o '"height":[0-9]*' | grep -o '[0-9]*$' ); }
query() { ( source <(bash net.sh env); as_org "$1"; peer chaincode query -C "$CHANNEL" -n simple \
  -c "{\"Args\":[\"query\",\"$2\"]}" 2>/dev/null ); }
transfer() { # $1 = client org, $2 from, $3 to, $4 amount -> client output in /tmp/e2e-client.txt
  local try
  for try in 1 2 3 4 5 6 7 8; do
    "$CLIENT" -net "$HERE" -org "$1" -from "$2" -to "$3" -amount "$4" > /tmp/e2e-client.txt 2>&1
    # right after start-up, gossip may not yet have told the gateway about the other org's peer
    grep -q "no combination of peers" /tmp/e2e-client.txt || break
    echo "        (gateway discovery does not see the other org's peer yet, retry $try in 5s)"; sleep 5
  done
  sed 's/^/        /' /tmp/e2e-client.txt
  grep -q "status VALID" /tmp/e2e-client.txt && grep -q "^RESULT: OK" /tmp/e2e-client.txt && return 0
  cp /tmp/e2e-client.txt "/tmp/e2e-client-failed-$(date +%s).txt"; return 1
}
verify() { # $1 orderer, $2 last block
  binibft-testclient verify -server "127.0.0.1:$((7150 + $1))" -channel "$CHANNEL" -ca "$OCA" \
    -mspdir "$OADMIN" -mspid OrdererMSP -genesis genesis.block -from 1 -to "$2" 2>&1 |
    grep -E "^block |verify OK|^ERROR"
}
all_equal() { local x=$1; shift; for y in "$@"; do [ "$x" = "$y" ] || return 1; done; [ -n "$x" ]; }

if [ $((N % 2)) -eq 0 ]; then echo "N must be odd (got $N)"; exit 2; fi
for b in orderer-binibft peer osnadmin cryptogen configtxgen binibft-testclient binibft-e2e-client simple-cc-server ccaas_builder/bin/build; do
  [ -x "$BIN/$b" ] || { echo "missing $BIN/$b (run: bash net.sh build)"; exit 2; }
done

echo "BiniBFT end-to-end test: $N orderers (tolerates f=$F faults, block quorum $Q), 2 peer orgs"

step "0. The orderer binary is BiniBFT, not SmartBFT"
[ "$(strings "$BIN/orderer-binibft" | grep -c 'consensus/binibft.(\*Chain)')" -gt 0 ] \
  && pass "BiniBFT chain is linked into the orderer" || fail "BiniBFT chain not found in the binary"
[ "$(strings "$BIN/orderer-binibft" | grep -c 'consensus/smartbft.(\*BFTChain)')" -eq 0 ] \
  && pass "SmartBFT chain is NOT in the orderer" || fail "SmartBFT chain found in the binary"

step "1. Fresh network: $N orderers + 2 peers"
bash net.sh clean >/dev/null 2>&1
pkill -x orderer-binibft 2>/dev/null; pkill -x peer 2>/dev/null; pkill -f simple-cc-server 2>/dev/null; sleep 2
N=$N bash net.sh gen >/dev/null 2>&1 || { fail "net.sh gen"; exit 1; }
bash net.sh up > /tmp/e2e-up.txt 2>&1
n_active=$(grep -c '"status": "active"' /tmp/e2e-up.txt)
[ "$n_active" = "$N" ] && pass "all $N orderers joined the channel as active consenters" \
  || fail "only $n_active of $N orderers are active consenters"
grep -q "Starting BiniBFT chain on channel $CHANNEL: consenter 1 of .*, primary 1, block quorum $Q" logs/orderer1.log \
  && pass "BiniBFT started: primary 1, block quorum $Q of $N" || fail "BiniBFT start line not found"
[ "$(grep -c 'Successfully submitted proposal to join channel' /tmp/e2e-up.txt)" = 2 ] \
  && pass "peer0.org1 and peer0.org2 joined the channel" || fail "peers did not join"

step "2. Chaincode lifecycle (approve x2, commit, init: each an ordered transaction)"
bash net.sh deploy 2>&1 | nocolor > /tmp/e2e-deploy.txt
valid=$(grep -c "committed with status (VALID)" /tmp/e2e-deploy.txt)
[ "$valid" -ge 5 ] && pass "approve Org1, approve Org2, commit, init: all VALID ($valid commit events)" \
  || fail "lifecycle failed ($valid VALID events), see /tmp/e2e-deploy.txt"
all_equal 100 "$(query 1 a)" "$(query 2 a)" && all_equal 200 "$(query 1 b)" "$(query 2 b)" \
  && pass "ledger initialised on both peers: a=100 b=200" || fail "init state wrong: a=$(query 1 a) b=$(query 1 b)"

step "3. Client of Org1 submits a transfer: execute -> order (BiniBFT) -> validate"
transfer 1 a b 10 && pass "Org1 client transaction endorsed, ordered, committed VALID" || fail "Org1 client transaction failed"
sig=$(grep -o "orderer signatures [0-9]*" /tmp/e2e-client.txt | grep -o '[0-9]*$')
[ "${sig:-0}" -ge "$Q" ] && pass "the block carries $sig orderer signatures (quorum $Q of $N)" || fail "block has ${sig:-0} signatures, need $Q"

step "4. Client of Org2 (other gateway) submits a transfer back"
transfer 2 b a 5 && pass "Org2 client transaction committed VALID" || fail "Org2 client transaction failed"
sleep 2
all_equal 95 "$(query 1 a)" "$(query 2 a)" && all_equal 205 "$(query 1 b)" "$(query 2 b)" \
  && pass "both peers agree: a=95 b=205" || fail "state differs: org1 a=$(query 1 a) b=$(query 1 b), org2 a=$(query 2 a) b=$(query 2 b)"

step "5. Same chain everywhere, accepted by Fabric's BFT block verifier"
hs=(); for i in $(seq 1 "$N"); do hs+=("$(o_height "$i")"); done
ph1=$(p_height 1); ph2=$(p_height 2)
all_equal "${hs[@]}" "$ph1" "$ph2" && pass "all $N orderers and both peers at height ${hs[0]}" \
  || fail "heights differ: orderers ${hs[*]}, peers $ph1 $ph2"
LAST=$(( ${hs[0]:-2} - 1 ))
for i in $(seq 1 "$N"); do verify "$i" "$LAST" > "/tmp/e2e-verify-$i.txt"; done
ok=1; for i in $(seq 1 "$N"); do grep -q "verify OK" "/tmp/e2e-verify-$i.txt" || ok=0; done
[ $ok = 1 ] && pass "blocks 1..$LAST pass Fabric BFT verification on all $N orderers" \
  || fail "BFT verification failed: $(grep -h ERROR /tmp/e2e-verify-*.txt | head -1)"
# Signature counts may differ per orderer (each writes once it holds a quorum), so compare number and hash only.
norm() { grep "^block" "/tmp/e2e-verify-$1.txt" | awk '{print $2, $NF}'; }
ok=1; for i in $(seq 2 "$N"); do [ "$(norm 1)" = "$(norm "$i")" ] || ok=0; done
[ $ok = 1 ] && [ -n "$(norm 1)" ] && pass "identical block hashes on all $N orderers" || fail "block hashes differ"
minsig=$(grep "^block" /tmp/e2e-verify-1.txt | sed -E 's/.*signatures=([0-9]+).*/\1/' | sort -n | head -1)
[ "${minsig:-0}" -ge "$Q" ] && pass "every block has >= $Q signatures" || fail "a block has only ${minsig:-0} signatures"

step "6. Fault tolerance: $F orderer(s) down"
if [ "$F" -eq 0 ]; then
  echo "  SKIP  N=$N tolerates no faults (f=0)"
else
  down=$(seq $((N - F + 1)) "$N" | tr '\n' ' ')
  for i in $down; do bash net.sh stop-orderer "$i" >/dev/null; done
  echo "        stopped orderer(s): $down"
  sleep 2
  transfer 1 a b 20 && pass "transaction committed VALID with $F orderer(s) down" || fail "no commit with $F orderer(s) down"
  sleep 2
  all_equal 75 "$(query 1 a)" "$(query 2 a)" && pass "both peers: a=75 b=$(query 1 b)" || fail "state after fault: a=$(query 1 a)/$(query 2 a)"
  live=(); for i in $(seq 1 $((N - F))); do live+=("$(o_height "$i")"); done
  all_equal "${live[@]}" "$(p_height 1)" "$(p_height 2)" && pass "the $((N - F)) live orderers and both peers at height ${live[0]}" \
    || fail "heights differ: ${live[*]}"
fi

step "7. No SmartBFT anywhere at run time"
c=$(cat logs/orderer*.log | nocolor | grep -c "orderer.consensus.smartbft")
b=$(cat logs/orderer*.log | nocolor | grep -c "orderer.consensus.binibft")
[ "$c" = 0 ] && [ "$b" -gt 0 ] && pass "orderer logs: $b BiniBFT lines, 0 SmartBFT lines" || fail "smartbft lines: $c, binibft lines: $b"

step "Cleanup"
if [ "${KEEP:-0}" = 1 ]; then echo "  network left running (bash net.sh down to stop)"; else bash net.sh down >/dev/null; echo "  network stopped; logs in $HERE/logs"; fi

echo
if [ $FAILED = 0 ]; then echo "RESULT: ALL CHECKS PASSED (N=$N)"; else echo "RESULT: SOME CHECKS FAILED (N=$N)"; fi
exit $FAILED
