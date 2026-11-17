# BiniBFT Fabric plugin: how to build, test and check that it works

_Last verified: 2026-10-13, Hyperledger Fabric v3.1.5, 4 orderers on one machine (WSL2 Ubuntu 24.04)._

This document explains how to test the BiniBFT ordering plugin that replaces
SmartBFT inside a real Hyperledger Fabric build, and how to tell from the output
whether it is working.

---

## 1. What you are testing

The plugin is a Fabric orderer consenter backed by the BiniBFT engine. It lives
**inside a clone of Fabric**, not in the `Hyperledger-BiniBFT` repo:

| Item | Location (inside WSL `Ubuntu-24.04`) |
|---|---|
| Fabric clone, branch `ragini/binibft-plugin` | `~/dev/fabric` |
| Plugin source | `~/dev/fabric/orderer/consensus/binibft/` |
| Vendored BiniBFT engine | `~/dev/fabric/orderer/consensus/binibft/engine/` |
| Registration (2-line change) | `~/dev/fabric/orderer/common/server/main.go` |
| Test client (broadcast + verify) | `~/dev/fabric/orderer/consensus/binibft/testclient/` |
| 4-orderer test network | `~/dev/fabric/orderer/consensus/binibft/testnet/` |
| Engine source repo (not edited) | `~/dev/Hyperledger-BiniBFT` |

Browse it from Windows: `\\wsl.localhost\Ubuntu-24.04\home\ragini\dev\fabric\orderer\consensus\binibft\`

**What "working" means here.** A real Fabric orderer binary, with BiniBFT instead
of SmartBFT, must:

1. start and join a channel as an *active consenter* (not a follower),
2. order transactions submitted to any orderer,
3. write **identical blocks on every orderer**,
4. attach a **BFT quorum of signatures** to every block, so that Fabric's own
   block verifier (the one peers use) accepts them,
5. keep ordering when one of the four orderers is down.

---

## 2. Prerequisites

Everything runs inside WSL. Open a terminal with:

```powershell
wsl -d Ubuntu-24.04
```

> Always pass `-d Ubuntu-24.04`. The default WSL distro on this machine is
> `docker-desktop`, which has no shell tools.

Required, and already installed on this machine:

| Need | Check | Notes |
|---|---|---|
| Go 1.26.4 | `~/opt/go/bin/go version` | Fabric v3.1.5 requires Go >= 1.26.4 |
| git, make, gcc, curl | `git --version` | |
| `bc` | `bc --version` | used by the smoke test |
| Free ports | see table below | 7050-7053, 7950-7953, 8440-8443 |
| ~2 GB RAM, ~2 GB disk | | the build cache is large |

Put Go on the PATH in every new shell (it is already in `~/.bashrc`):

```bash
export PATH=$HOME/opt/go/bin:$HOME/bin:$PATH
```

Ports used by the test network:

| Orderer | Consenter ID | Client/cluster port | Admin (osnadmin) | Operations |
|---|---|---|---|---|
| orderer1 (primary) | 1 | 7050 | 7950 | 8440 |
| orderer2 | 2 | 7051 | 7951 | 8441 |
| orderer3 | 3 | 7052 | 7952 | 8442 |
| orderer4 | 4 | 7053 | 7953 | 8443 |

---

## 3. Fastest check: one command

```bash
cd ~/dev/fabric/orderer/consensus/binibft/testnet
bash smoke-test.sh
```

It stops any running test orderers, wipes the test ledgers, generates a fresh
network, runs every check below and prints PASS/FAIL per check. The exit code is
`0` only if everything passes. To rebuild the binaries first: `BUILD=1 bash smoke-test.sh`.

**Expected output (all checks pass):**

```
== 1. Fresh network
  PASS  all 4 orderers joined and active at height 1
  PASS  BiniBFT chain started: primary 1, block quorum 3
== 2. Ordering through a non-primary orderer (forwarded to the primary)
  PASS  20 transactions accepted by orderer3
  PASS  all orderers at the same height (2)
== 3. Blocks verify with Fabric's own BFT block verifier, hashes agree
  PASS  orderer1: blocks 1..1 pass BFT block verification
  ... (orderer2, orderer3, orderer4)
  PASS  identical block hashes and signature counts on all 4 orderers
  PASS  every block carries >= 3 signatures (BFT quorum for n=4)
  PASS  transaction count matches: sent 20, committed 20
== 4. Fault tolerance: one orderer down (f=1)
  PASS  10 transactions accepted with orderer4 down
  PASS  orderers 1-3 advanced together (2 -> 3) with orderer4 down
RESULT: ALL CHECKS PASSED
```

If any line says `FAIL`, go to section 8 (Troubleshooting).

---

## 4. Step-by-step manual test

Use this when you want to see each piece, or to debug.

### 4.1 Unit tests

```bash
cd ~/dev/fabric
go test ./orderer/consensus/binibft/
```

Expected: `ok  github.com/hyperledger/fabric/orderer/consensus/binibft`. These cover
the BFT quorum formula, config-transaction isolation, the wire codec (including
that a sender cannot be spoofed), signature verification and block/proposal
consistency.

### 4.2 Build

```bash
cd ~/dev/fabric
go build -o ~/bin/orderer-binibft ./cmd/orderer                          # orderer WITH BiniBFT
go build -o ~/bin/binibft-testclient ./orderer/consensus/binibft/testclient
for t in cryptogen configtxgen osnadmin; do go build -o ~/bin/$t ./cmd/$t; done
```

The first build takes about 2 minutes; later builds take seconds.

Prove that the binary contains BiniBFT and not SmartBFT:

```bash
strings ~/bin/orderer-binibft | grep -c "consensus/binibft.(\*Chain)"         # > 0  (BiniBFT chain is linked in)
strings ~/bin/orderer-binibft | grep -c "consensus/smartbft.(\*BFTChain)"     # 0    (SmartBFT chain is not)
```

A couple of other `smartbft` strings remain in the binary on purpose: Fabric's
`smartbft/util` package (a config-parsing helper) is still used elsewhere.
Only the SmartBFT *consenter* is replaced.

### 4.3 Generate and start the network

```bash
cd ~/dev/fabric/orderer/consensus/binibft/testnet
pkill -x orderer-binibft          # stop leftovers (use -x: -f would match your own shell)
rm -rf data logs
bash net.sh gen                   # crypto material + genesis block (crypto only created once)
bash net.sh up                    # start 4 orderers, join "mychannel"
```

`up` should print four lines like:

```
Status: 201 {"name": "mychannel", ... "consensusRelation": "other", "status": "active", "height": 1}
```

* `"consensusRelation": "other"` with `"status": "active"` means the orderer joined as a consenter.
* `"follower"` / `"onboarding"` means it was **not** recognised as a consenter. That is a failure (section 8).

Confirm the plugin started:

```bash
grep -h "binibft" logs/orderer1.log | grep -E "local consenter|Starting BiniBFT"
```

Expected:

```
HandleChain -> Channel mychannel: local consenter ID is 1, consenters [1 2 3 4]
Start -> Starting BiniBFT chain on channel mychannel: consenter 1 of [1 2 3 4], primary 1, block quorum 3
```

### 4.4 Shortcuts for the next steps

```bash
cd ~/dev/fabric/orderer/consensus/binibft/testnet
O=crypto-config/ordererOrganizations/example.com
A=$O/users/Admin@example.com/msp                     # signing identity
CA=$O/orderers/orderer1.example.com/tls/ca.crt       # TLS CA
C=~/bin/binibft-testclient
height() { ~/bin/osnadmin channel list --channelID mychannel -o 127.0.0.1:$((7949+$1)) 2>&1 | grep -o '"height": *[0-9]*'; }
```

### 4.5 Order transactions

Send 20 transactions to **orderer3, a non-primary**. It has to forward them to the
primary, so this exercises the cluster transport:

```bash
$C broadcast -server 127.0.0.1:7052 -channel mychannel -ca $CA -mspdir $A -mspid OrdererMSP -n 20 2>&1 | grep -E "broadcast OK|^ERROR"
sleep 4
for i in 1 2 3 4; do echo -n "orderer$i "; height $i; done
```

**Pass:** `broadcast OK: 20 transactions accepted`, and all four orderers show the
same height (at least 2).

### 4.6 Verify the blocks with Fabric's BFT verifier

```bash
for i in 1 2 3 4; do
  echo "--- orderer$i"
  $C verify -server 127.0.0.1:$((7049+i)) -channel mychannel -ca $CA -mspdir $A -mspid OrdererMSP \
     -genesis genesis.block -from 1 -to 1 2>&1 | grep -E "^block |verify OK|^ERROR"
done
```

Expected, for every orderer:

```
block 1  verified  signatures=3  txs=20  hash=<64 hex chars>
verify OK: blocks 1..1 from 127.0.0.1:705x pass Fabric's BFT block verification
```

**Pass criteria, all three:**

1. `signatures=3` or more (BFT quorum = ceil((n+f+1)/2) = 3 for n = 4).
2. The `hash=` value is **identical on all four orderers**.
3. Each ends with `verify OK`.

The `verify` command uses `deliverclient.NewBlockVerificationAssistant`, the same
code a peer or an orderer follower uses to accept blocks from a BFT ordering
service. If it says `BLOCK N FAILED BFT VERIFICATION`, the blocks would be
rejected by real peers.

To check a longer chain, raise `-to` to the last block number (height - 1).

### 4.7 Check that no transaction was lost or duplicated

```bash
$C verify -server 127.0.0.1:7050 -channel mychannel -ca $CA -mspdir $A -mspid OrdererMSP \
   -genesis genesis.block -from 1 -to $(( $(height 1 | grep -o '[0-9]*$') - 1 )) 2>&1 \
   | grep "^block" | sed -E 's/.*txs=([0-9]+).*/\1/' | paste -sd+ | bc
```

The result must equal the number of transactions you broadcast.

### 4.8 Fault tolerance: one orderer down

```bash
bash net.sh stop 4
$C broadcast -server 127.0.0.1:7050 -channel mychannel -ca $CA -mspdir $A -mspid OrdererMSP -n 10 2>&1 | grep -E "broadcast OK|^ERROR"
sleep 4
for i in 1 2 3; do echo -n "orderer$i "; height $i; done
```

**Pass:** orderers 1, 2 and 3 all advance by one block while orderer4 is down
(n = 4 tolerates f = 1 failure).

### 4.9 Stop and clean up

```bash
bash net.sh down          # stop all orderers
bash net.sh clean         # also delete ledgers and logs
```

---

## 5. Expected failures (known gaps)

These are limitations of the current implementation, not environment problems.
If they unexpectedly *pass*, that is worth investigating.

| Test | What happens today | Why |
|---|---|---|
| `bash net.sh stop 1`, then send a transaction | rejected with `SERVICE_UNAVAILABLE`; no new blocks | no leader failover: the engine's view change is a stub |
| `bash net.sh start 4` after section 4.8 | orderer4 comes back `active` at its old height and never catches up | no block synchronizer |
| Config update that changes the consenter set | not applied until the chain is re-created | no live membership change |
| `BatchSize.MaxMessageCount` | blocks may exceed it (a block with 280 txs was seen with limit 20) | the engine's batcher ignores the count limit |

Not tested at all: joining a real **peer**, multiple machines, TLS between
separate hosts, large messages, long-running load.

---

## 6. Useful commands

```bash
# Height of every orderer
for i in 1 2 3 4; do ~/bin/osnadmin channel list --channelID mychannel -o 127.0.0.1:$((7949+i)); done

# Follow one orderer's log (colour codes stripped)
tail -f logs/orderer1.log | sed 's/\x1b\[[0-9;]*m//g'

# Only plugin lines, warnings and errors
grep -h "binibft\|ERRO\|WARN\|PANI" logs/orderer*.log | sed 's/\x1b\[[0-9;]*m//g' | cut -c1-250

# Debug logging of the plugin and engine (restart the network with it)
FABRIC_LOGGING_SPEC="info:orderer.consensus.binibft=debug" bash net.sh up

# Are the orderers running?
pgrep -x orderer-binibft | wc -l
```

---

## 7. Changing the code

```bash
# After editing the plugin (orderer/consensus/binibft/*.go):
cd ~/dev/fabric
gofmt -l orderer/consensus/binibft/*.go          # no output means formatted
go vet ./orderer/consensus/binibft/
go test ./orderer/consensus/binibft/
go build -o ~/bin/orderer-binibft ./cmd/orderer
bash orderer/consensus/binibft/testnet/smoke-test.sh

# After changing the ENGINE (Hyperledger-BiniBFT/consensus), re-vendor it:
bash orderer/consensus/binibft/sync-engine.sh
# then rebuild and re-run the smoke test as above
```

The engine is copied into the Fabric tree (not imported as a module) because
Fabric vendors its dependencies. `sync-engine.sh` copies `consensus/*.go`
(without tests) and renames the package to `engine`.

Commit your work in `~/dev/fabric` on branch `ragini/binibft-plugin`. Nothing outside git
(`~/bin`, `testnet/data`, `testnet/crypto-config`) is preserved by a clone.

---

## 8. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `wsl: ... execvpe(bash) failed` | default WSL distro is `docker-desktop` | use `wsl -d Ubuntu-24.04` |
| `go: command not found` | Go not on PATH | `export PATH=$HOME/opt/go/bin:$HOME/bin:$PATH` |
| `up` says `follower` / `onboarding`, never `active` | orderer did not recognise itself as a consenter (identity mismatch), or stale certificates | check `logs/orderer1.log` for `local consenter ID`; make sure `crypto-config` and `genesis.block` were generated together, then `net.sh clean`, `gen`, `up` |
| `x509: certificate signed by unknown authority`, `tls: bad certificate` | crypto material was regenerated while orderers were running | `pkill -x orderer-binibft`, `rm -rf data logs`, `bash net.sh up`. Never delete `crypto-config` under running orderers |
| `Orderer capability V3_0 is required but not supported` | `V3_0` set under *Orderer* capabilities | `V3_0` belongs under *Channel* (and Application) capabilities only |
| `address already in use` | leftover orderers | `pkill -x orderer-binibft` |
| `Permission denied` running `./net.sh` | exec bit lost after an edit | run it as `bash net.sh ...` |
| `pkill -f orderer-binibft` killed your shell | `-f` matches the shell's own command line | use `pkill -x orderer-binibft` |
| broadcast: `rejected: SERVICE_UNAVAILABLE ... connection refused` | the primary (orderer1) is down | known gap, restart orderer1 |
| `verify` fails: `BLOCK N FAILED BFT VERIFICATION` | a real defect: the blocks would be rejected by peers | keep `logs/` and report it |
| `no such file ... orderer-binibft` | binaries not built | `BUILD=1 bash smoke-test.sh` or section 4.2 |

---

## 9. What each check proves

| Check | Proves |
|---|---|
| Orderers `active`, not `follower` | `IsChannelMember` and `HandleChain` work; the consenter identity mapping is right |
| Tx via non-primary is ordered | cluster transport and request forwarding work |
| Equal heights and identical hashes | all consenters commit the same blocks in the same order |
| `signatures >= 3` and `verify OK` | blocks carry a valid BFT quorum in Fabric's format, so peers will accept them |
| Sent count equals committed count | no loss or duplication |
| Progress with one orderer down | crash-fault tolerance for f = 1 |

What these checks do **not** prove: Byzantine behaviour (malicious or
equivocating nodes), view change, recovery after restart, performance, or
interoperability with a real peer. See section 5.
