Hyperledger Fabric Functionalities
==================================

Hyperledger Fabric is an implementation of distributed ledger technology
(DLT) that delivers enterprise-ready network security, scalability,
confidentiality and performance, in a modular blockchain architecture.
Hyperledger Fabric delivers the following blockchain network functionalities:

Identity management
-------------------

To enable permissioned networks, Hyperledger Fabric provides a membership
identity service that manages user IDs and authenticates all participants on
the network. Access control lists can be used to provide additional layers of
permission through authorization of specific network operations. For example, a
specific user ID could be permitted to invoke a chaincode application, but
be blocked from deploying new chaincode.

Privacy and confidentiality
---------------------------

Hyperledger Fabric enables competing business interests, and any groups that
require private, confidential transactions, to coexist on the same permissioned
network. Private **channels** are restricted messaging paths that can be used
to provide transaction privacy and confidentiality for specific subsets of
network members. All data, including transaction, member and channel
information, on a channel are invisible and inaccessible to any network members
not explicitly granted access to that channel.

Efficient processing
--------------------

Hyperledger Fabric assigns network roles by node type. To provide concurrency
and parallelism to the network, transaction execution is separated from
transaction ordering and commitment. Executing transactions prior to
ordering them enables each peer node to process multiple transactions
simultaneously. This concurrent execution increases processing efficiency on
each peer and accelerates delivery of transactions to the ordering service.

In addition to enabling parallel processing, the division of labor unburdens
ordering nodes from the demands of transaction execution and ledger
maintenance, while peer nodes are freed from ordering (consensus) workloads.
This bifurcation of roles also limits the processing required for authorization
and authentication; all peer nodes do not have to trust all ordering nodes, and
vice versa, so processes on one can run independently of verification by the
other.

Chaincode functionality
-----------------------

Chaincode applications encode logic that is
invoked by specific types of transactions on the channel. Chaincode that
defines parameters for a change of asset ownership, for example, ensures that
all transactions that transfer ownership are subject to the same rules and
requirements. **System chaincode** is distinguished as chaincode that defines
operating parameters for the entire channel. Lifecycle and configuration system
chaincode defines the rules for the channel; endorsement and validation system
chaincode defines the requirements for endorsing and validating transactions.

Modular design
--------------

Hyperledger Fabric implements a modular architecture to
provide functional choice to network designers. Specific algorithms for
identity, ordering (consensus) and encryption, for example, can be plugged in
to any Hyperledger Fabric network. The result is a universal blockchain
architecture that any industry or public domain can adopt, with the assurance
that its networks will be interoperable across market, regulatory and
geographic boundaries.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
