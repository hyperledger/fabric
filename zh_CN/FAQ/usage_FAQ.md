## Usage

&nbsp;
#####What are the expected performance figures for the fabric?
The performance of any chain network depends on several factors: proximity of the validating nodes, number of validators, encryption method, transaction message size, security level set, business logic running, and the consensus algorithm deployed, among others.

The current performance goal for the fabric is to achieve 100,000 transactions per second in a standard production environment of about 15 validating nodes running in close proximity. The team is committed to continuously improving the performance and the scalability of the system.

&nbsp;
##### Do I have to own a validating node to transact on a chain network?
No. You can still transact on a chain network by owning a non-validating node (NV-node).

Although transactions initiated by NV-nodes will eventually be forwarded to their validating peers for consensus processing, NV-nodes establish their own connections to the membership service module and can therefore package transactions independently. This allows NV-node owners to independently register and manage certificates, a powerful feature that empowers NV-node owners to create custom-built applications for their clients while managing their client certificates.

In addition, NV-nodes retain full copies of the ledger, enabling local queries of the ledger data.

&nbsp;
##### What does the error string "state may be inconsistent, cannot query" as a query result mean?
Sometimes, a validating peer will be out of sync with the rest of the network. Although determining this condition is not always possible, validating peers make a best effort determination to detect it, and internally mark themselves as out of date.

When under this condition, rather than reply with out of date or potentially incorrect data, the peer will reply to chaincode queries with the error string "state may be inconsistent, cannot query".

In the future, more sophisticated reporting mechanisms may be introduced such as returning the stale value and a flag that the value is stale.
