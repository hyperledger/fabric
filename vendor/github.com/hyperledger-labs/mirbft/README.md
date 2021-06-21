# MirBFT Library

This open-source project is part of [Hyperledger Labs](https://labs.hyperledger.org/labs/mir-bft.html).
It aims at developing a production-quality implementation of the
[Mir Byzantine fault tolerant consensus protocol](https://arxiv.org/abs/1906.05552)
and integrating it with Hyperledger Fabric.
Being framed as a library, however, MirBFT's goal is also to serve as a general-purpose high-performance BFT component
of other projects as well.

The [research branch](https://github.com/hyperledger-labs/mirbft/tree/research) contains code developed independently
as a research prototype and was used to produce experimental results
for the [research paper](https://arxiv.org/abs/1906.05552).

## Current status

This library is in development and not usable yet.
This document describes what the library _should become_ rather than what it _currently is_.
This document itself is more than likely to still change.
You are more than welcome to contribute to accelerating the development of the MirBFT library.
Have a look at the [Contributions section](#contributing) if you want to help out!

[![Build Status](https://github.com/hyperledger-labs/mirbft/actions/workflows/test.yml/badge.svg)](https://github.com/hyperledger-labs/mirbft/actions)
[![GoDoc](https://godoc.org/github.com/hyperledger-labs/mirbft?status.svg)](https://godoc.org/github.com/hyperledger-labs/mirbft)

## Overview

MirBFT is a library implementing the [Mir byzantine fault tolerant consensus protocol](https://arxiv.org/abs/1906.05552)
in a network transport, storage, and cryptographic algorithm agnostic way.
MirBFT hopes to be a building block of a next generation of distributed systems,
providing an implementation of [atomic broadcast](https://en.wikipedia.org/wiki/Atomic_broadcast)
which can be utilized by any distributed system.

MirBFT improves on traditional atomic broadcast protocols
like [PBFT](https://www.microsoft.com/en-us/research/wp-content/uploads/2017/01/p398-castro-bft-tocs.pdf) and Raft
which always have a single active leader by allowing concurrent leaders
and reconciling total order in a deterministic way.

The multi-leader nature of Mir should lead to exceptional performance
especially on wide area networks,
but should be suitable for LAN deployments as well.

MirBFT, decouples request payload dissemination from ordering,
outputting totally ordered request digests rather than all request data.
This allows for novel methods of request dissemination.
The provided request dissemination method, however, does guarantee the availability of all request payload data.

## Design

The high level structure of the MirBFT library
is inspired by [etcdraft](https://github.com/etcd-io/etcd/tree/master/raft).
The protocol logic is implemented as a state machine that is mutated by short-lived, non-blocking operations.
Operations which might block or which have any degree of computational complexity (like hashing, I/O, etc.)
are delegated to the caller.

TODO: Link the "state machine" to the documentation of the state machine interface when it exists.

Additional components required to use the library are:

1. A **write-ahead-log (WAL)** that the protocol uses for persisting its state.
   This is crucial for crash-recovery - a recovering node will use the entries in this log when re-initializing.
   External actions (like sending messages or interacting with the application) are always reflected in the WAL
   before they occur.
   This way a crash of a node and a subsequent recovery are completely transparent to the rest of the system,
   except a potential delay incurred by the recovery.
   The users of this library can use their own WAL implementation
   or use [the library-provided one](https://github.com/hyperledger-labs/mirbft/tree/main/pkg/simplewal)
2. A **request store** for persisting application requests.
   This is a simple persistent key-value store where requests are indexed by their hashes.
   The users of this library can use their own request store implementation
   or use [the library-provided one](https://github.com/hyperledger-labs/mirbft/tree/main/pkg/reqstore).
3. A hashing implementation.
   Any hash implementation can be provided that satisfies Go's standard library interface,
   e.g., [sha256](https://golang.org/pkg/crypto/sha256/).
4. An application that will consume the agreed-upon requests and provide state snapshots on library request.
   Looking at MirBFT as a state machine replication (SMR) library,
   this application is the "state machine" in terms of SMR.
   It is not to be confused with the library-internal state machine that implements the protocol.
5. A networking component that implements sending and receiving messages over the network.
   It is the networking component's task to establish physical connections to other nodes
   and abstract away addresses, ports, authentication, etc.
   The library relies on nodes being addressable by their numerical IDs and the messages received being authenticated.
   TODO: Provide a simple implementation and link it here.

TODO: Link the component names to the documentation of the respective interfaces when this documentation is ready.

For basic applications, the library-provided components should be sufficient.
To increase performance, the user of the library may provide their own optimized implementations.

## Using Mir
 
TODO: Refer to the sample application when it is integrated in this repository.

## Contributing

**Contributions are more than welcome!**

If you want to contribute, have a look at Contributor's guide
(TODO: create a contributor's guide link when there is a contributor's guide.)
and at the open [issues](https://github.com/hyperledger-labs/mirbft/issues).
If you have any questions (specific or general),
do not hesitate to post them on [MirBFT's Rocket.Chat channel](https://chat.hyperledger.org/channel/mirbft).
You can also drop an email to the active maintainer(s).

## Summary of References

- Public Rocket.Chat channel: https://chat.hyperledger.org/channel/mirbft
- Hyperledger Labs page: https://labs.hyperledger.org/labs/mir-bft.html
- Paper describing the algorithm: https://arxiv.org/abs/1906.05552
- Original PBFT paper: https://www.microsoft.com/en-us/research/wp-content/uploads/2017/01/p398-castro-bft-tocs.pdf

## Active maintainer(s)

- [Matej Pavlovic](https://github.com/matejpavlovic) (mpa@zurich.ibm.com)

## Initial Committers

- [Jason Yellick](https://github.com/jyellick)
- [Matej Pavlovic](https://github.com/matejpavlovic)
- [Chrysoula Stathakopoulou](https://github.com/stchrysa)
- [Marko Vukolic](https://github.com/vukolic)

## Sponsor

[Angelo de Caro](https://github.com/adecaro) (adc@zurich.ibm.com).

## License

The MirBFT library source code is made available under the Apache License, version 2.0 (Apache-2.0).

TODO: Add a license file and link to it.