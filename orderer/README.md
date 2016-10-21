# Hyperledger Ordering Service
The hyperledger fabric ordering service is intended to provide an atomic broadcast ordering service for consumption by the peers.  This means that many clients may submit messages for ordering, and all clients are delivered the same series of ordered batches in response.

## Protocol definition
The atomic broadcast ordering protocol for hyperledger fabric is described in `hyperledger/fabric/orderer/atomicbroadcast/ab.proto`.  There are two services, the `Broadcast` service for injecting messages into the system, and the `Deliver` service for receiving ordered batches from the service.  Sometimes, the service will reside over the network, while othertimes, the service may be bound locally into a peer process.  The service may be bound locally for single process development deployments, or when the underlying ordering service has its own backing network protocol and the proto serves only as a wrapper.

## Service types
* Solo Orderer:
The solo orderer is intended to be an extremely easy to deploy, non-production orderer.  It consists of a single process which serves all clients, so no `consensus' is required as there is a single central authority.  There is correspondingly no high availability or scalability.  This makes solo ideal for development and testing, but not deployment.  The Solo orderer depends on a backing raw ledger.

* Kafka Orderer (pending):
The Kafka orderer leverages the Kafka pubsub system to perform the ordering, but wraps this in the familiar `ab.proto` definition so that the peer orderer client code does not to be written specifically for Kafka.  In real world deployments, it would be expected that the Kafka proto service would bound locally in process, as Kafka has its own robust wire protocol.  However, for testing or novel deployment scenarios, the Kafka orderer may be deployed as a network service.  Kafka is anticipated to be the preferred choice production deployments which demand high throughput and high availability but do not require byzantine fault tolerance.  The Kafka orderer does not utilize a backing raw ledger because this is handled by the Kafka brokers.

* PBFT Orderer (pending):
The PBFT orderer uses the hyperledger fabric PBFT implementation to order messages in a byzantine fault tolerant way.  Because the implementation is being developed expressly for the hyperledger fabric, the `ab.proto` is used for wireline communication to the PBFT orderer.  Therefore it is unusual to bind the PBFT orderer into the peer process, though might be desirable for some deployments.  The PBFT orderer depends on a backing raw ledger.

## Raw Ledger Types
Because the ordering service must allow clients to seek within the ordered batch stream, orderers must maintain a local copy of past batches.  The length of time batches are retained may be configurable (or all batches may be retained indefinitely). Not all ledgers are crash fault tolerant, so care should be used when selecting a ledger for an application.  Because the raw leger interface is abstracted, the ledger type for a particular orderer may be selected at runtime.  Not all orderers require (or can utilize) a backing raw ledger (for instance Kafka, does not).

* RAM Ledger
The RAM ledger implementation is a simple development oriented ledger which stores batches purely in RAM, with a configurable history size for retention.  This ledger is not crash fault tolerant, restarting the process will reset the ledger to the genesis block.  This is the default ledger.
* File Ledger
The file ledger implementation is a simple development oriented ledger which stores batches as JSON encoded files on the filesystem.  This is intended to make inspecting the ledger easy and to allow for crash fault tolerance.  This ledger is not intended to be performant, but is intended to be simple and easy to deploy and understand.  This ledger may be enabled before executing the `orderer` binary by setting `ORDERER_LEDGER_TYPE=file` (note: this is a temporary hack and may not persist into the future).
* Other Ledgers
There are currently no other raw ledgers available, although it is anticipated that some high performance database or other log based storage system will eventually be adapter for production deployments.

## Experimenting with the orderer service

To experiment with the orderer service you may build the orderer binary by simply typing `go build` in the `hyperledger/fabric/orderer` directory.  You may then invoke the orderer binary with no parameters, or you can override the bind address, port, and backing ledger by setting the environment variables `ORDERER_LISTEN_ADDRESS`, `ORDERER_LISTEN_PORT` and `ORDERER_LEDGER_TYPE` respectively.  Presently, only the solo orderer is supported.  The deployment and configuration is very stopgap at this point, so expect for this to change noticably in the future.

There are sample clients in the `fabric/orderer/sample_clients` directory.  The `broadcast_timestamp` client sends a message containing the timestamp to the `Broadcast` service.  The `deliver_stdout` client prints received batches to stdout from the `Deliver` interface.  These may both be build simply by typing `go build` in their respective directories.  Neither presently supports config, so editing the source manually to adjust address and port is required.
