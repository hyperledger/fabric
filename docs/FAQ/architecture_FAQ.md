# V1 Network and Architecture

## Network

__Network architecture__:

Q. In the design document, clients are supposed to be able to connect to all peers (endorsers and consenters). But in reality, the blockchain peers may be hosted in private, restricted and secure network, for example, the participants are likely to host their peer in each internal network. In this case, how can clients connect to all peers? 

A. Clients only need to connect to as many peers as are required for endorsement. We expect 'clients' will more likely be application servers running close to a peer, which has connection to the peer network, rather than end user clients.  There has been discussion of a 'submitting peer' pattern where your peer could act on behalf of your client to connect to other peers to gather endorsements and submit to ordering.  This is not in plan for v1 but may get re-visited in future releases.

## Security & Access Control

__Reliance on External Application Server__:

Q. Security (data integrity) — Reliance on an external application server is a single-point of control over data that could be compromised easily. Write operations flow through through the application server and/or directly to the primary data store. These systems have no concept of consensus and thus represent a single-point of control and therefore vulnerability.

A. More question context is needed - which external application server is in question?

Q. I hear that within a channel "everyone sees all the data." Does this mean "every endorser sees all the data" or do all users of the chaincode (non-endorsers) also see all the data? My understanding was that every endorser will see all the data, but ACLs in chaincode can ensure that users get selective visibility. Has there been a change to this? If so, where can I find documentation? I can't find any documentation on access control.

A. A channel is comprised of a subset of the committing peers in a network that are authorized to maintain the ledger (and see the data) for a certain set of chaincodes. Endorsers are a subset of Committer peers that simulate and endorse a given transaction on the channel, but the transaction results will ultimately get sent to all committing peers on the channel.  Access control can still be enforced within chaincode running on the channel, using similar ACL control mechanisms that were in v0.5/0.6 chaincode.  The ACL control options within chaincode do need to be better documented.


__Application-level Data access control__:

Q. In the business process, fine grained access control is required, not only by use or role, but also the state, or content of the data (e.g., only the owner of the document can update it). Currently it is all responsibility of the chaincode to implement such ALC. Is there any design abstract for this?

A. This aspect will remain the same as 0.5/0.6, see above question.

Q. How is access control enforced at the direct query to CouchDB? What is the granularity?

A. Docker configuration enforces that only the peer container can connect to CouchDB.  All queries should go through peer chaincode, where chaincode enforces the access control.

Q. Even if there are multiple private chains, the Orderers can see all the transactions, which means they are the single point of trust. This is not acceptable in some use cases. Are there any plans to make orderers decentralized?

A. This is generally true. The orderers do receive a higher degree of trust, and they must necessarily see all channels and their membership. I would point out, however, that the orderers only see the information which passes through them. Certain pieces of the transaction, such as the proposal, can be configured not to go through ordering. Similarly, any data which is referenced by hash or encrypted would be opaque to the orderers. Clients can hash/encrypt the data that they submit to ledger.

Q. This is not acceptable in some use cases. Is there any plans to make orderers decentralized?

A. I'm not sure I understand this portion of the question. There are other ordering protocols in the works, like SBFT, which allow further decentralization by allowing for more parties to take part because of the Byzantine fault tolerance. But, this does not really solve the confidentiality problem, in fact, distributing it makes it worse as there are more potential information leakage points.

A. I would point out that ordering does not necessarily need to be centralized. A single peer could participate with many different ordering networks. (This is not necessarily targeted for v1, but the architecture does explicitly intend this).

Q. Is encryption of transaction and ledger removed from V1?

A. V1 does not have encryption at fabric level. The data at rest can be encrypted via file system encryption, and the data in transit is encrypted via TLS. In v1 it is possible to set up private channels so that you can share a ledger (and corresponding transactions) with the subset of network participants that are allowed to see the data.   The submitting party now has full control - they can encrypt the complete set of data before submitting to ledger, encrypt sensitive portions only, or not encrypt if complete transparency/queryability is the goal.

## Application-side Programming Model

__Processing capacity of client__:

Q. In the current design, client needs to send tx, collect endorsement result, check endorsement policy and broadcast endowment. These tasks are acceptable for the powerful clients such as server, but may be beyond the processing capacity of weaker ones, such as mobile app, IOT edge device. Is there any consideration for this scenario? 

A. We expect 'clients' will more likely be application servers that are serving the actual end user base.  That being said, there has been discussion of a 'submitting peer' pattern where your peer could act on behalf of your client to connect to other peers to gather endorsements and submit to ordering.  This is not in plan for v1 but may get re-visited in future releases.


__Transaction execution result__:

Q. Transaction execution is asynchronous in blockchain. We cannot get the tx result for now, which is a big problem in the solution development. Any plan to fetch the output result? 

A. The output of the transaction execution is provided to the client by the endorser.  Ultimately the transaction output will get validated or invalidated by the committing peers, and the client becomes aware of the outcome via an event.

## Monitoring

__More Monitoring Metrics are expected from the Docker Image__:

Q. (phrased as a requirement) Provide basic information about deployed chaincode on the chain, such as chaincode ID, its deployed time, etc. For each chain code, with proper permission, provide invoke/query metrics data such as TPS, consensus latency (average, 50%, 85%), broken down into different chaincode API (function). Provide storage usage information on peer node. 

A. Agreed, much work remains around serviceability in general, monitoring is one piece of that.  The specific feedback here has been added to:
https://jira.hyperledger.org/browse/FAB-2174 - As an administrator, I need better runtime monitoring capabilities for the fabric

## Backward Compatibility

__Chaincode Stub Interface__:

Q. (phrased as a requirement) It's preferred to minimize changes to chaincode stub API
1. Removal of Table API causes backward compatibility issues to existing chaincode
2. Need RangeQuery for the world state to maintain backward compatibility of chaincode

A. There is general agreement that chaincode API should be as stable as possible.  That being said, 0.5/0.6 was a developer preview to gather feedback, and community feedback around API experiences must be taken into consideration for v1 release.  There were many complaints and frustrations around the table API limitations, since it added overhead but didn't enable typical table queries. v1 provides a broader and richer set of APIs that solutions have called for, including `GetStateByRange()`, `GetStateByPartialCompositeKey()`, `GetHistoryForKey()` against the default/embedded Key/Value data store. Also there is a beta option to use CouchDB JSON document store that enables rich query via a new `GetQueryResult()` API. The new rich query is intended to be used for read-only queries, but it can also be used in chaincode read-write transactions if the application layer can guarantee the stability of the query result set between transaction simulation and validation/commit phases (no phantoms).

__REST API__:

Q. (phrased as a requirement) It's preferred to maintain REST API and hopefully make it more secure, in order to use existing tools via REST API (e.g., JMeter, Postman, etc.)

A. SDK resolves security concerns of 0.5/0.6 REST API.  SDK itself will likely provide the REST API interface going forward (rather than the peer).  This is not yet available but planned for v1 or shortly thereafter.


__Database / Data Store__

Q. How ACL is enforced at block data?

A. In v1 a member of the channel has access to the data on that channel's ledger.  Access can be controlled within chaincode, and data can be encrypted before submitting to ledger. See questions in Security section above.


__ACID-Compliant Transactions__:

Q. (phrased as a requirement) ACID-compliant transactions. While the overall Hyperledger system is eventually consistent, having a local datastore that is ACID compliant will simplify integration/implementation.

A. v1 architecture separates execution (transaction simulation) and validation/commit phases.  Upon commit, the transaction is indeed atomic, consistent, isolated (serializable), and durable.  But clients working with endorsers and the chaincode needs to be designed with the understanding that the transaction is a proposal only, until such time that it is sent to peers for commitment.  See NCAP document for more details: https://github.com/hyperledger/fabric/blob/master/proposals/r1/Next-Consensus-Architecture-Proposal.md


Q. (phrased as a requirement) Query the historical data

A. New chaincode API in v1 `GetHistoryForKey()` will return history of values for a key.

Q. (phrased as a requirement) Too much burden on chaincode to implement data access layer (such as DAO) to wrap KVS.

A. v1 provides a broader and richer set of APIs that solutions have called for, including `GetStateByRange()`, `GetStateByPartialCompositeKey()`, `GetHistoryForKey()` against the default/embedded Key/Value data store. Also there is a beta option to use CouchDB JSON document store that enables rich query via a new GetQueryResult() API. The new rich query is intended to be used for read-only queries, but it can also be used in chaincode read-write transactions if the application layer can guarantee the stability of the query result set between transaction simulation and validation/commit phases (no phantoms). Research is investigating even richer data experience for future releases.  Additionally, the new Composer offering is intended to make it simpler for developers to work with the fabric.

__Data integrity constraint__:

Q. Data integrity constraint is an important requirement from customer. For example, the balance of user bank account should always be non-negative. Is there any plan for data integrity constraint?

A. Chaincode is the appropriate place to enforce data constraints.

Q. How to guarantee the query result is correct, when the peer being queried is just recovering from failure and in the middle of the state transfer?

A. New blocks are always being added to the blockchain, whether the peer is catching up or doing normal processing.  The client can query multiple peers, compare their block heights, compare their query results, and favor the peers at the higher block heights.

__The Missing/Altered Data Problem__:

Q. (phrased as a requirement) Describe the minimal query results vs full set, how we can only do error detection. If we search for Shanghai, and get five results, and can verify the hashcode, that means that at a minimum there are five assets from Shanghai. However, there may in fact be more, but they may have been edited to say the location is Beijing. For those we can only detect that they are wrong, but we cannot correct them back to Shanghai.

A. Is this a question about past state, or malicious tampering of data?  If the former, there has been talk of solutions to sync data to a data warehouse for historical queries and analysis.  If the latter, there is future work planned to audit the validity of the blockchain data, for example recalculate and compare hashes to prove/audit the data integrity of the blockchain.

Q. (phrased as a requirement) No support for tables and range queries. Only simple GetState and PutState.

A. There were many complaints and frustrations around the table API limitations in 0.5/0.6, since it added overhead but didn't enable typical table queries. v1 provides a broader and richer set of APIs that solutions have called for, including `GetStateByRange()`, `GetStateByPartialCompositeKey()`, `GetHistoryForKey()` against the default/embedded Key/Value data store. Also there is a beta option to use CouchDB JSON document store that enables rich query via a new GetQueryResult() API. The new rich query is intended to be used for read-only queries, but it can also be used in chaincode read-write transactions if the application layer can guarantee the stability of the query result set between transaction simulation and validation/commit phases (no phantoms).

Q. (phrased as a requirement) Request for a Trusted Storage Service to store large files that are not appropriate to store in the world state and to replicate across all the peers.

A. In traditional systems, it is very common to have a separate object storage to manage large object files aside from the DBMS. A common approach today is to only store the hash values of the files in the ledger and have the files managed somewhere else. There are three main problems for this approach. 1. the developer has to handle the interaction between the ledger and the object storage. 2. there is no good way to guarantee the consistency of the content between the ledger and the object storage. 3. the object storage has to be well secured, and protected by the access control in the same manner as the data on blockchain. Therefore, trusted storage service, that is integrated with blockchain is desired.

Additional issues when most of the data is stored in an external data store, and the hash-codes used for verification and the linked-list data structures are stored in in the blockchain. This leads to the following problems:
   
   2.1. Primary Data Store Schema/Behavior Changes — In order to track an asset and recall historical values, the whole point of provenance, the primary data store, which is external to blockchain, will need to store all historical versions of the documents. Without this, historical hash values stored on the blockchain are meaningless for navigating provenance. 
   
   2.2. Query-ability —The blockchain serves two purposes in the provenance solution as it stands today: data integrity and storing the graph of historical hash values of an asset. This means that for many types of queries, such as tell me all locations an asset has been at, the application server must perform data fusion between the lineage data, which is a sequence of document hashes, and the primary data store, which has been supplemented to store the document hashes necessary for quickly retrieving the documents referenced by the provenance graph’s hashes stored in the blockchain.
   
   2.3. Data duplication — In part to overcome the query-ability challenges, some additional data should be duplicated in the primary data store. For example, to be able to easily traverse the history of an asset, it is helpful to store the hash of the previous document in the primary data store so that the application server is not required to perform data fusion between the data stored in the blockchain and the primary data store to show the provenance of an asset, such as all locations it has been at. 

It sounds like new v1 rich query and history APIs will help, but will not entirely address the requirement here.  Please net out the specific remaining requirements for consideration post-v1.



__Data Integration with Graph DB__:

Q. Blockchain Integration Challenges for Provenance MVP.

1. Manipulating/querying the opaque linked-list data structure stored in the values of the Hyperledger key-value store is not possible in a generic way. Custom chaincode that is able to interpret the opaque data structure is required.
   1.1. Thoughts: In graph databases, such as Neo4J, there are typically three data stores used to represent the various parts of a graph: 1) Nodes: a key-value store with the node id as the key, 2) Properties, and 3) Relationships. The most basic solution would be to allow us to use multiple key-value stores as part of the chaincode, but of course if we do that we might as well just focus on integrating an existing graph database. One good example of a graph database that in fact already uses rocksdb and is written in go is Dgraph. However, this data structure is not well-optimized for forward edge-chaining traversals, such as we want to use for traversing history.
2. Related to the thoughts mentioned in point 1, we really need a good way to do efficient graph edge/version traversal. Typical graph systems are optimized for answering questions such as "tell me the set of all friends of my friends" This only chains two edge traversals. We are mostly interested in much longer edge traversals.
