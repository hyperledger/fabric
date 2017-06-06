Hyperledger Fabric FAQs
=======================

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

Chaincode (Smart Contracts and Digital Assets)
----------------------------------------------

* Does the fabric implementation support smart contract logic?

Yes. Chaincode is the fabric’s interpretation of the smart contract
method/algorithm, with additional features.

A chaincode is programmatic code deployed on the network, where it is
executed and validated by chain validators together during the consensus
process. Developers can use chaincodes to develop business contracts,
asset definitions, and collectively-managed decentralized applications.

* How do I create a business contract using the fabric?

There are generally two ways to develop business contracts: the first way is to
code individual contracts into standalone instances of chaincode; the
second way, and probably the more efficient way, is to use chaincode to
create decentralized applications that manage the life cycle of one or
multiple types of business contracts, and let end users instantiate
instances of contracts within these applications.

* How do I create assets using the fabric?

Users can use chaincode (for business rules) and membership service (for digital tokens) to
design assets, as well as the logic that manages them.

There are two popular approaches to defining assets in most blockchain
solutions: the stateless UTXO model, where account balances are encoded
into past transaction records; and the account model, where account
balances are kept in state storage space on the ledger.

Each approach carries its own benefits and drawbacks. This blockchain
fabric does not advocate either one over the other. Instead, one of our
first requirements was to ensure that both approaches can be easily
implemented with tools available in the fabric.

* Which languages are supported for writing chaincode?

Chaincode can be written in any programming language and executed in containers
inside the fabric context layer. We are also looking into developing a
templating language (such as Apache Velocity) that can either get
compiled into chaincode or have its interpreter embedded into a
chaincode container.

The fabric's first fully supported chaincode language is Golang, and
support for JavaScript and Java is planned for 2016. Support for
additional languages and the development of a fabric-specific templating
language have been discussed, and more details will be released in the
near future.

* Does the fabric have native currency?

No. However, if you really need a native currency for your chain network, you can develop your own
native currency with chaincode. One common attribute of native currency
is that some amount will get transacted (the chaincode defining that
currency will get called) every time a transaction is processed on its
chain.

Identity Management (Membership Service)
----------------------------------------

* What is unique about the fabric's Membership Service module?

One of the things that makes the Membership Service module stand out from
the pack is our implementation of the latest advances in cryptography.

In addition to ensuring private, auditable transactions, our Membership
Service module introduces the concept of enrollment and transaction
certificates. This innovation ensures that only verified owners can
create asset tokens, allowing an infinite number of transaction
certificates to be issued through parent enrollment certificates while
guaranteeing the private keys of asset tokens can be regenerated if
lost.

Issuers also have the ability revoke transaction certificates or
designate them to expire within a certain timeframe, allowing greater
control over the asset tokens they have issued.

Like most other modules on Fabric, you can always replace the
default module with another membership service option should the need
arise.

* Does its Membership Service make Fabric a centralized solution?

No. The only role of the Membership Service module is to issue digital
certificates to validated entities that want to participate in the
network. It does not execute transactions nor is it aware of how or when
these certificates are used in any particular network.

However, because certificates are the way networks regulate and manage
their users, the module serves a central regulatory and organizational
role.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
