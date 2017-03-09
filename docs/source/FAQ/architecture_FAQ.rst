V1 Architecture
===========================

Endorsement
-----------

**Endorsement architecture**:

Q. How many peers in the network need to endorse a transaction?

A. The number of peers required to endorse a transaction is driven by the endorsement
policy that is specified at chaincode deployment time.

Q. Does an application client need to connect to all peers?

A. Clients only need to connect to as many peers as are required by the
endorsement policy for the chaincode.

Security & Access Control
-------------------------

**Data Privacy and Access Control**:

Q. How do I ensure data privacy?

A. There are various aspects to data privacy.
First, you can segregate your network into channels, where each channel represents a subset of
participants that are authorized to see the data for the chaincodes that are deployed
to that channel.
Second, within a channel you can restrict the input data to chaincode to the set of
endorsers only, by using visibility settings. The visibility setting will determine
whether input and output chaincode data is included in the submitted transaction,  versus just
output data.
Third, you can hash or encrypt the data before calling chaincode. If you hash
the data then you will need a way to share the source data outside of fabric. If you encrypt
the data then you will need a way to share the decryption keys outside of fabric.
Fourth, you can restrict data access to certain roles in your organization, by building
access control into the chaincode logic.
Fifth, ledger data at rest can be encrypted via file system encryption on the peer, and data in
transit is encrypted via TLS.

Q. Do the orderers see the transaction data?

A. No, the orderers only order transactions, they do not open the transactions.
If you do not want the data to go through the orderers at all, and you are only concerned
about the input data, then you can use visibility settings. The visibility setting will determine
whether input and output chaincode data is included in the submitted transaction,  versus just
output data. Therefore the input data can be private to the endorsers only.
If you do not want the orderers to see chaincode output, then you can hash or encrypt the data
before calling chaincode. If you hash the data then you will need a way to share the source data
outside of fabric. If you encrypt the data then you will need a way to share the decryption keys
outside of fabric.

Application-side Programming Model
----------------------------------

**Transaction execution result**:

Q. How do application clients know the outcome of a transaction?

A. The transaction simulation results are returned to the client by the endorser in the proposal
response.  If there are multiple endorsers, the client can check that the responses are all the
same, and submit the results and endorsements for ordering and commitment.
Ultimately the committing peers will validate or invalidate the transaction, and the client becomes
aware of the outcome via an event, that the SDK makes available to the application client.

**Ledger queries**:

Q. How do I query the ledger data?

Within chaincode you can query based on keys. Keys can be queried by range, and composite keys can
be modeled to enable equivalence queries against multiple parameters. For example a composite
key of (owner,asset_id) can be used to query all assets owned by a certain entity. These key-based
queries can be used for read-only queries against the ledger, as well as in transactions that
update the ledger.

If you model asset data as JSON in chaincode and use CouchDB as the state database, you can also
perform complex rich queries against the chaincode data values, using the CouchDB JSON query
language within chaincode. The application client can perform read-only queries, but these
responses are not typically submitted as part of transactions to the ordering service.

Q. How do I query the historical data to understand data provenance?

A. The chaincode API ``GetHistoryForKey()`` will return history of
values for a key.

Q. How to guarantee the query result is correct, especially when the peer being
queried may be recovering and catching up on block processing?

A. The client can query multiple peers, compare their block heights, compare their query results,
and favor the peers at the higher block heights.
