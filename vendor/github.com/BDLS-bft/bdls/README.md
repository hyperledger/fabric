# BDLS
Efficient BFT in partial synchronous networks 

[![GoDoc][1]][2] [![License][3]][4] [![Build Status][5]][6] [![Go Report Card][7]][8] [![Coverage Statusd][9]][10] [![Sourcegraph][11]][12]

[1]: https://godoc.org/github.com/ahmed82/bdls?status.svg
[2]: https://godoc.org/github.com/ahmed82/bdls
[3]: https://img.shields.io/badge/License-Apache_2.0-blue.svg
[4]: LICENSE
[5]: https://travis-ci.org/BDLS-bft/bdls.svg?branch=master
[6]: https://travis-ci.org/BDLS-bft/bdls
[7]: https://goreportcard.com/badge/github.com/BDLS-bft/bdls?bdls
[8]: https://goreportcard.com/report/github.com/BDLS-bft/bdls
[9]: https://codecov.io/gh/BDLS-bft/bdls/branch/master/graph/badge.svg
[10]: https://codecov.io/gh/BDLS-bft/bdls
[11]: https://sourcegraph.com/github.com/BDLS-bft/bdls/-/badge.svg
[12]: https://sourcegraph.com/github.com/BDLS-bft/bdls?badge

# BDLS Consensus
* ## [Contributing info](CONTRIBUTING.md)

## Introduction

BDLS is an innovative BFT consensus algorithm that features safety and liveness by
presenting a mathematically proven secure BFT protocol that is resilient in open networks such as
the Internet. BDLS overcomes many problems, such as the deadlock problem caused by unreliable
p2p/broadcast channels. These problems are all very relevant to existing realistic open
network scenarios, and are the focus of extensive work in improving Internet security, but it
is an area largely ignored by most in mainstream BFT protocol design.
(Paper: https://eprint.iacr.org/2019/1460.pdf or https://dl.acm.org/doi/abs/10.1145/3538227 or  https://doi.org/10.1145/3538227 or https://www.doi.org/10.1007/978-3-030-91859-0_2 )

For this library, to make the runtime behavior of consensus algorithm predictable as function:
y = f(x, t), where 'x' is the message it received, and 't' is the time while being called,
  then'y' is the deterministic status of consensus after 'x' and 't' applied to 'f',
it has been designed in a deterministic scheme, without parallel computing, networking, and
the correctness of program implementation can be proven with proper test cases.

## Features

1. Pure algorithm implementation in deterministic and predictable behavior, easily to be integrated into existing projects, refer to [DFA](https://en.wikipedia.org/wiki/Deterministic_finite_automaton) for more.
2. Well-tested on various platforms with complicated cases.
3. Auto back-off under heavy payload, guaranteed finalization(worst case gurantee).
4. Easy integratation into Blockchain & non-Blockchain consensus, like [WAL replication](https://en.wikipedia.org/wiki/Replication_(computing)#Database_replication) in database.
5. Builtin network emulation for various network latency with comprehensive statistics.



## Documentation

For complete documentation, see the associated [Godoc](https://pkg.go.dev/github.com/BDLS-bft/bdls).


## Install BDLS on Ubuntu Server 20.04 

```
sudo apt-get update
sudo apt-get -y upgrade
sudo apt-get install autoconf automake libtool curl make g++ unzip
cd /tmp
wget https://go.dev/dl/go1.17.5.linux-amd64.tar.gz
sudo tar -xvf go1.17.5.linux-amd64.tar.gz
sudo mv go /usr/local
cd
echo 'export GOROOT=/usr/local/go' >> .profile
echo 'export GOPATH=$HOME/go' >> .profile
echo 'export PATH=$GOPATH/bin:$GOROOT/bin:$PATH' >> .profile
source ~/.profile 
go version
go env
git clone https://github.com/hyperledger-labs/bdls.git
cd bdls/
git checkout master
cd cmd/emucon/
go build .
./emucon help genkeys
./emucon genkeys --count 4

[open four terminals to run four participants. if you log to remote Linux, 
you may use tmux commands. In tmux, you can switch termian using "ctrl+b d" 
and use "tmux attach -t 0" to enter the terminal. Use "tmux list-session" 
to check the current active terminals]


./emucon run --id 0 --listen ":4680"
./emucon run --id 1 --listen ":4681"
./emucon run --id 2 --listen ":4682"
./emucon run --id 3 --listen ":4683"

cd ../..
go test -v -cpuprofile=cpu.out -memprofile=mem.out -timeout 2h
```
## Regenerate go.mod and go.sum
```
rm go.*
go mod init github.com/hyperledger-labs/bdls
go mod tidy
go mod vendor
```

See benchmark ourput at: [AMD-NORMAL.TXT](benchmarks/AMD-NORMAL.TXT) and [PI4-OVERLOAD.TXT](benchmarks/PI4-OVERLOAD.TXT)

## Specification

1. Consensus messages are specified in [message.proto](message.proto), users of this library can encapsulate this message in a carrier message, like gossip in TCP.
2. Consensus algorithm is **NOT** thread-safe, it **MUST** be protected by some synchronization mechanism, like `sync.Mutex` or `chan` + `goroutine`.

## Usage

1. A testing IPC peer -- [ipc_peer.go](ipc_peer.go)
2. A testing TCP node -- [TCP based Consensus Emualtor](cmd/emucon)

## Status
 
On-going
