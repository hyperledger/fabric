# BiniBFT full network: end-to-end (execute-order-validate) test

A complete Hyperledger Fabric network on localhost whose ordering service is
**BiniBFT** (no SmartBFT):

* **N orderers**, odd: 3, 5 (default) or 7. Every orderer is a BiniBFT consenter.
* **2 peer organisations**: `peer0.org1` and `peer0.org2`. The endorsement policy is MAJORITY, so both must endorse.
* **Chaincode `simple`**: run as a service through Fabric's `ccaas` builder, so Docker is not needed.
* **A client application** (`client/`): uses the Fabric Gateway SDK and acts as `User1`, an ordinary client, not an admin.

The client submits a transfer and prints each phase: the peers endorse it (execute),
BiniBFT orders it into a block signed by a quorum of orderers (order), and the peers
validate and commit it (validate).

## Requirements

* Linux or WSL2 (tested on Ubuntu 24.04)
* Go >= 1.26.4 (required by Fabric v3.1.5), git, openssl, gcc
* Internet access on the first build, because the client downloads the Fabric Gateway SDK
* Free ports 7151-7157, 9151-9157, 9251-9257, 7311-7312, 7321-7322, 9311, 9321, 9911, 9921

## Run

```bash
cd orderer/consensus/binibft/fullnet
bash net.sh build            # once: builds all binaries into ~/bin (override with BIN=/path)
bash e2e-test.sh             # N=5; also N=3 / N=7: N=7 bash e2e-test.sh
```

The test ends with `RESULT: ALL CHECKS PASSED (N=5)`. It checks the following:

* the orderer binary contains BiniBFT and not SmartBFT
* all orderers are active consenters and both peers joined the channel
* the chaincode lifecycle (approve x2, commit, init) is VALID
* clients of both orgs commit VALID transactions, and both peers agree on the state
* all orderers and peers are at the same height with identical block hashes
* every block passes Fabric's BFT block verifier with at least the quorum of signatures
* transactions still commit with f orderers down
* the logs contain 0 SmartBFT lines

## Step by step

```bash
N=5 bash net.sh gen          # crypto material, channel config, genesis block
bash net.sh up               # start orderers + peers, join the channel
bash net.sh deploy           # chaincode lifecycle + init (a=100, b=200)
sleep 30                     # let gossip discovery find the other org's peer
~/bin/binibft-e2e-client -net . -org 1 -from a -to b -amount 10
bash net.sh stop-orderer 5   # crash one orderer, then send again: still commits
bash net.sh down             # stop everything   (clean: also delete generated files)
```

## Known limits

* If orderer1 (the primary) goes down, ordering stops, because the engine has no view change yet.
* A restarted orderer does not catch up on the blocks it missed.
* The test runs on a single machine only, and only crash faults are tested (no Byzantine faults).
