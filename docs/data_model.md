# Transaction Model

[WIP]

The Fabric transaction model is a complete, self-encapsulated immutable record
of the state transition on the ledger of the participating parties. It contains
a chaincode function call, versioned inputs (referred to as the read set), and
outputs (referred to as the write set). The read-write-sets are the actual states
making up the multi-version concurrency control, which serves to grant an
appropriate version to each read request. Transactions issuing write requests
which might destroy database integrity are aborted at the "validation" phase to
counter against malicious threats such as double-spending.

Transactions are appended in order of processing to an append-only log with
cryptographic links to prevent any modification to the records. There is no
limit on the log size except available physical storage space.

States are data variables of any type represented by binary or text (JSON) format;
they are scoped and manipulated by chaincodes running on a ledger. A chaincode
function reads and writes states (read-write set) in the context of a transaction
proposal, which is endorsed by the counter-parties specified for the chaincode.
The endorsed proposal is submitted to the network as a transaction for ordering,
validation, and then committal to the ledger within a block.

Before committal, peers will make sure that the transaction has been both adequately,
and properly endorsed (i.e. the correct allotment of the specified peers have
signed the proposal, and these signatures have been authenticated) - this is
referred to as validation.  Secondly, a versioning check will occur to ensure
that the key value pairs affected by this transaction have not been altered
since simulation of the proposal.  Or put another way, the world state for this
chaincode still mirrors the world state used for the inputs.  If these conditions
are met, then this transaction is written to the ledger and the versions of the
keys, along with their new values, are updated in the world state.  

See the [Read Write Set](readwrite.md) topic for a deeper dive on transaction
structure, MVCC and the state DB.  
