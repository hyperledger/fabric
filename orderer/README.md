# Hyperledger Fabric Ordering Service

The Hyperledger Fabric ordering service provides an atomic broadcast ordering service for consumption by the peers. This means that many clients can submit messages to the ordering service, and the same sequence of ordered batches will be delivered to all clients in response.

## Protocol definition

The atomic broadcast ordering protocol for Hyperledger Fabric is described in `hyperledger/fabric/protos/orderer/ab.proto`. There are two services: the `Broadcast` service for injecting messages into the system and the `Deliver` service for receiving ordered batches from the service.

## Service types

* Solo ordering service (testing): The solo ordering service is intended to be an extremely easy to deploy, non-production ordering service. It consists of a single process which serves all clients, so consensus is not required as there is a single central authority.  There is correspondingly no high availability or scalability. This makes solo ideal for development and testing, but not for deployment.
* Kafka-based ordering service (production): The Kafka-based ordering service leverages the Kafka pub/sub system to perform the ordering, but wraps this in the familiar `ab.proto` definition so that the peer orderer client code does not to be written specifically for Kafka. Kafka is currently the preferred choice for production deployments which demand high throughput and high availability, but do not require byzantine fault tolerance.
* PBFT ordering service (pending): The PBFT ordering service will use the Hyperledger Fabric PBFT implementation (currently under development) to order messages in a byzantine fault tolerant way.

### Choosing a service type

In order to set a service type, the ordering service administrator needs to set the right value in the genesis block that the ordering service nodes will be bootstrapped from.

Specifically, the value corresponding to the `ConsensusType` key of the `Values` map of the `Orderer` config group on the system channel should be set to either `solo`, `kafka` or `etcdraft`.

For details on the configuration structure of channels, refer to the [Channel Configuration](../docs/source/configtx.rst) guide.

`configtxgen` is a tool that allows for the creation of a genesis block using profiles, or grouped configuration parameters — refer to the [Configuring using the connfigtxgen tool](../docs/source/configtxgen.rst) guide for more.

The location of this block can be set using the `ORDERER_GENERAL_GENESISFILE` environment variable. As is the case with all the configuration paths for Fabric binaries, this location is relative to the path set via the `FABRIC_CFG_PATH` environment variable.

## Ledger types

Because the ordering service must allow clients to seek within the ordered batch stream, orderers need a backing ledger, where they maintain a local copy of past batches. Not all ledgers are crash fault tolerant, so care should be used when selecting a ledger for an application. Because the orderer ledger interface is abstracted, the ledger type for a particular orderer may be selected at runtime. The following options are available:

* File ledger (production): The file-based ledger stores blocks directly on the file system. The block locations on disk are 'indexed' in a lightweight LevelDB database by number so that clients can efficiently retrieve a block by number. This is the default, and the suggested option for production deployments.
* RAM ledger (testing): The RAM ledger implementation is a simple development oriented ledger which stores batches purely in memory, with a configurable history size for retention. This ledger is not crash fault tolerant; restarting the process will reset the ledger to the genesis block.
* JSON ledger (testing): The file ledger implementation is a simple development oriented ledger which stores batches as JSON encoded files on the filesystem.  This is intended to make inspecting the ledger easy and to allow for crash fault tolerance. This ledger is not intended to be performant, but is intended to be simple and easy to deploy and understand.

### Choosing a ledger type

This can be set by setting the `ORDERER_GENERAL_LEDGERTYPE` environment variable before executing the `orderer` binary. Acceptable values are `file` (default), `ram`, and `json`.

## Experimenting with the orderer service

To experiment with the orderer service you may build the orderer binary by simply typing `go build` in the `hyperledger/fabric/orderer` directory. You may then invoke the orderer binary with no parameters, or you can override the bind address, port, and backing ledger by setting the environment variables `ORDERER_GENERAL_LISTENADDRESS`, `ORDERER_GENERAL_ LISTENPORT` and `ORDERER_GENERAL_LEDGER_TYPE` respectively.

There are sample clients in the `fabric/orderer/sample_clients` directory.

* The `broadcast_timestamp` client sends a message containing the timestamp to the `Broadcast` service.
* The `deliver_stdout` client prints received batches to stdout from the `Deliver` interface.

These may both be built simply by typing `go build` in their respective directories. Note that neither of these clients supports config (so editing the source manually to adjust address and port is required), or signing (so they can only work against channels where no ACL is enforced).

### Profiling

Profiling the ordering service is possible through a standard HTTP interface documented [here](https://golang.org/pkg/net/http/pprof). The profiling service can be configured using the **orderer.yaml** file, or through environment variables. To enable profiling set `ORDERER_GENERAL_PROFILE_ENABLED=true`, and optionally set `ORDERER_GENERAL_PROFILE_ADDRESS` to the desired network address for the profiling service. The default address is `0.0.0.0:6060` as in the Golang documentation.

Note that failures of the profiling service, either at startup or anytime during the run, will cause the overall orderer service to fail. Therefore it is currently not recommended to enable profiling in production settings.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
