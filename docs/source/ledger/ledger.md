# Ledger

The ledger is the sequenced, tamper-resistant record of all state transitions.
State transitions are a result of chaincode invocations ("transactions")
submitted by participating parties.  Each transaction results in a set of asset
key-value pairs that are committed to the ledger as creates, updates, or
deletes.

The ledger is comprised of a blockchain ('chain') to store the immutable,
sequenced record in blocks, as well as a state database to maintain current
state.  There is one ledger per channel. Each peer maintains a copy of the
ledger for each channel of which they are a member.

In the diagram below one can see that the ledger consists of two major items,
the Blockchain and the World State. The state transitions have as contents facts
asserted by principals outside the Hyperledger Fabric Network, but inside one of
the organizations that make up the consortium associated with the network. The
contents of the state transitions are also referred to as user transactions. All
these transactions are immutably stored in blocks that are linked by hashes of
the payload of the block. Each block contains a hash and a block number, and
each block except the first one contains the hash of the previous block.

![ledger.ledger](./ledger.diagram.1.png)

*The Visual Vocabulary expressed in facts is as follows: The block with number 0 contains the hash 6, and contains the transactions T1 and T2; the block with number 1 contains the hash 8, contains the transaction T3, and has as hash of the previous block 6.*

## Chain

The chain is a transaction log, structured as hash-linked blocks, where each
block contains a sequence of N transactions. The block header includes a hash of
the block's transactions, as well as a hash of the prior block's header. In this
way, all transactions on the ledger are sequenced and cryptographically linked
together. In other words, it is not possible to tamper with the ledger data,
without breaking the hash links. The hash of the latest block represents every
transaction that has come before, making it possible to ensure that all peers
are in a consistent and trusted state.

The chain is stored on the peer file system (either local or attached storage),
efficiently supporting the append-only nature of the blockchain workload.

![ledger.blockchain](./ledger.diagram.2.png)

## Blocks

Need a subsection on blocks

![ledger.blocks](./ledger.diagram.4.png)

## World State

The ledger's current state data represents the latest values for all keys ever
included in the chain transaction log. Since current state represents all latest
key values known to the channel, it is sometimes referred to as World State.

The World State contains facts that are derived by the peers taking as input the previous World State and the Write sets of the valid transactions of the last block.

Here comes a diagram with the contents of the first 4 transactions of fabcar, in the format of the diagram below

![ledger.worldstate](./ledger.diagram.3.png)

Why is made by Ford not expressed with currently?  Because the manufacturer of a car can not change in time, nor can the model.
Please note that the World State after the first transaction would be the same as the first transaction. Hence conceptually there is this point in time when the chain and the World State can be the same as far as the relevant facts of the business are concerned. They differ substantially in other facts. One may ask: I learned that a block usually has many transactions in it and the World State is after, say 10 transactions, different from the chain. That is true. However it could have happened that the Ordering System reached the time out after the first transaction and then the above mentioned situation would be the case. In all other cases one could say that the World State consists of the derived facts from all the previous transactions and describe the current state of each relevant fact.


Chaincode invocations execute transactions against the current state data. To
make these chaincode interactions extremely efficient, the latest values of all
keys are stored in a state database. The state database is simply an indexed
view into the chain's transaction log, it can therefore be regenerated from the
chain at any time. The state database will automatically get recovered (or
generated if needed) upon peer startup, before transactions are accepted.

State database options include LevelDB and CouchDB. LevelDB is the default state
database embedded in the peer process and stores chaincode data as key/value
pairs. CouchDB is an optional alternative external state database that provides
addition query support when your chaincode data is modeled as JSON, permitting
rich queries of the JSON content. See
[CouchDB as the State Database](./couchdb_as_state_database.html) for more
information on CouchDB.

## Transactions

Need a section on transaction structure

![ledger.transation](./ledger.diagram.5.png)

## Transaction Flow

At a high level, the transaction flow consists of a transaction proposal sent by
an application client to specific endorsing peers.  The endorsing peers verify
the client signature, and execute a chaincode function to simulate the
transaction. The output is the chaincode results, a set of key/value versions
that were read in the chaincode (read set), and the set of keys/values that were
written in chaincode (write set). The proposal response gets sent back to the
client along with an endorsement signature.

The client assembles the endorsements into a transaction payload and broadcasts
it to an ordering service. The ordering service delivers ordered transactions as
blocks to all peers on a channel.

Before committal, peers will validate the transactions. First, they will check
the endorsement policy to ensure that the correct allotment of the specified
peers have signed the results, and they will authenticate the signatures against
the transaction payload.

Secondly, peers will perform a versioning check against the transaction read
set, to ensure data integrity and protect against threats such as
double-spending. Hyperledger Fabric has concurrency control whereby transactions
execute in parallel (by endorsers) to increase throughput, and upon commit (by
all peers) each transaction is verified to ensure that no other transaction has
modified data it has read. In other words, it ensures that the world state data
that was read during chaincode execution has not changed since execution
(endorsement) time, and therefore the execution results are still valid and can
be committed to the world state database. If the data that was read has been
changed by another transaction, then the transaction in the block is marked as
invalid and is not applied to the world state database. The client application
is alerted, and can handle the error or retry as appropriate.

## Example Ledger: fabcar

This walks through the fabcar example, and how the ledger evolves over time as transactions are executed.

Here's an example of the kind of thing we mean:

![ledger.transation](./ledger.diagram.6.png)

## More information

See the [Transaction Flow](./txflow.html),
[Read-Write set semantics](./readwrite.html) and
[CouchDB as the State Database](./couchdb_as_state_database.html) topics for
a deeper dive on transaction structure, concurrency control, and the state DB.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
