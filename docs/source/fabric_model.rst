Hyperledger Fabric Model
========================

This section outlines the key design features woven into Hyperledger Fabric that
fulfill its promise of a comprehensive, yet customizable, enterprise blockchain solution:

* :ref:`Assets` --- Asset definitions enable the exchange of almost anything with
  monetary value over the network, from whole foods to antique cars to currency
  futures.
* :ref:`Chaincode` --- Chaincode execution is partitioned from transaction ordering,
  limiting the required levels of trust and verification across node types, and
  optimizing network scalability and performance.
* :ref:`Ledger-Features` --- The immutable, shared ledger encodes the entire
  transaction history for each channel, and includes SQL-like query capability
  for efficient auditing and dispute resolution.
* :ref:`Privacy` --- Channels and private data collections enable private and
  confidential multi-lateral transactions that are usually required by
  competing businesses and regulated industries that exchange assets on a common
  network.
* :ref:`Security-Membership-Services` --- Permissioned membership provides a
  trusted blockchain network, where participants know that all transactions can
  be detected and traced by authorized regulators and auditors.
* :ref:`Consensus` --- A unique approach to consensus enables the
  flexibility and scalability needed for the enterprise.

.. _Assets:

Assets
------

Assets can range from the tangible (real estate and hardware) to the intangible
(contracts and intellectual property).  Hyperledger Fabric provides the
ability to modify assets using chaincode transactions.

Assets are represented in Hyperledger Fabric as a collection of
key-value pairs, with state changes recorded as transactions on a :ref:`Channel`
ledger.  Assets can be represented in binary and/or JSON form.

.. _Chaincode:

Chaincode
---------

Chaincode is software defining an asset or assets, and the transaction instructions for
modifying the asset(s); in other words, it's the business logic.  Chaincode enforces the rules for reading
or altering key-value pairs or other state database information. Chaincode functions execute against
the ledger's current state database and are initiated through a transaction proposal. Chaincode execution
results in a set of key-value writes (write set) that can be submitted to the network and applied to
the ledger on all peers.

.. _Ledger-Features:

Ledger Features
---------------

The ledger is the sequenced, tamper-resistant record of all state transitions in the fabric.  State
transitions are a result of chaincode invocations ('transactions') submitted by participating
parties.  Each transaction results in a set of asset key-value pairs that are committed to the
ledger as creates, updates, or deletes.

The ledger is comprised of a blockchain ('chain') to store the immutable, sequenced record in
blocks, as well as a state database to maintain current fabric state.  There is one ledger per
channel. Each peer maintains a copy of the ledger for each channel of which they are a member.

Some features of a Fabric ledger:

- Query and update ledger using key-based lookups, range queries, and composite key queries
- Read-only queries using a rich query language (if using CouchDB as state database)
- Read-only history queries --- Query ledger history for a key, enabling data provenance scenarios
- Transactions consist of the versions of keys/values that were read in chaincode (read set) and keys/values that were written in chaincode (write set)
- Transactions contain signatures of every endorsing peer and are submitted to ordering service
- Transactions are ordered into blocks and are "delivered" from an ordering service to peers on a channel
- Peers validate transactions against endorsement policies and enforce the policies
- Prior to appending a block, a versioning check is performed to ensure that states for assets that were read have not changed since chaincode execution time
- There is immutability once a transaction is validated and committed
- A channel's ledger contains a configuration block defining policies, access control lists, and other pertinent information
- Channels contain :ref:`MSP` instances allowing for crypto materials to be derived from different certificate authorities

See the :doc:`ledger` topic for a deeper dive on the databases, storage structure, and "query-ability."

.. _Privacy:

Privacy
-------

Hyperledger Fabric employs an immutable ledger on a per-channel basis, as well as
chaincode that can manipulate and modify the current state of assets (i.e. update
key-value pairs).  A ledger exists in the scope of a channel --- it can be shared
across the entire network (assuming every participant is operating on one common
channel) --- or it can be privatized to include only a specific set of participants.

In the latter scenario, these participants would create a separate channel and
thereby isolate/segregate their transactions and ledger.  In order to solve
scenarios that want to bridge the gap between total transparency and privacy,
chaincode can be installed only on peers that need to access the asset states
to perform reads and writes (in other words, if a chaincode is not installed on
a peer, it will not be able to properly interface with the ledger).

When a subset of organizations on that channel need to keep their transaction
data confidential, a private data collection (collection) is used to segregate
this data in a private database, logically separate from the channel ledger,
accessible only to the authorized subset of organizations.

Thus, channels keep transactions private from the broader network whereas
collections keep data private between subsets of organizations on the channel.

To further obfuscate the data, values within chaincode can be encrypted
(in part or in total) using common cryptographic algorithms such as AES before
sending transactions to the ordering service and appending blocks to the ledger.
Once encrypted data has been written to the ledger, it can be decrypted only by
a user in possession of the corresponding key that was used to generate the cipher
text. For further details on chaincode encryption, see the :doc:`chaincode4ade`
topic.

See the :doc:`private-data-arch` topic for more details on how to achieve
privacy on your blockchain network.

.. _Security-Membership-Services:

Security & Membership Services
------------------------------

Hyperledger Fabric underpins a transactional network where all participants have
known identities.  Public Key Infrastructure is used to generate cryptographic
certificates which are tied to organizations, network components, and end users
or client applications.  As a result, data access control can be manipulated and
governed on the broader network and on channel levels.  This "permissioned" notion
of Hyperledger Fabric, coupled with the existence and capabilities of channels,
helps address scenarios where privacy and confidentiality are paramount concerns.

See the :doc:`msp` topic to better understand cryptographic
implementations, and the sign, verify, authenticate approach used in
Hyperledger Fabric.

.. _Consensus:

Consensus
---------

In distributed ledger technology, consensus has recently become synonymous with
a specific algorithm, within a single function. However, consensus encompasses more
than simply agreeing upon the order of transactions, and this differentiation is
highlighted in Hyperledger Fabric through its fundamental role in the entire
transaction flow, from proposal and endorsement, to ordering, validation and commitment.
In a nutshell, consensus is defined as the full-circle verification of the correctness of
a set of transactions comprising a block.

Consensus is achieved ultimately when the order and results of a block's
transactions have met the explicit policy criteria checks. These checks and balances
take place during the lifecycle of a transaction, and include the usage of
endorsement policies to dictate which specific members must endorse a certain
transaction class, as well as system chaincodes to ensure that these policies
are enforced and upheld.  Prior to commitment, the peers will employ these
system chaincodes to make sure that enough endorsements are present, and that
they were derived from the appropriate entities.  Moreover, a versioning check
will take place during which the current state of the ledger is agreed or
consented upon, before any blocks containing transactions are appended to the ledger.
This final check provides protection against double spend operations and other
threats that might compromise data integrity, and allows for functions to be
executed against non-static variables.

In addition to the multitude of endorsement, validity and versioning checks that
take place, there are also ongoing identity verifications happening in all
directions of the transaction flow.  Access control lists are implemented on
hierarchical layers of the network (ordering service down to channels), and
payloads are repeatedly signed, verified and authenticated as a transaction proposal passes
through the different architectural components.  To conclude, consensus is not
merely limited to the agreed upon order of a batch of transactions; rather,
it is an overarching characterization that is achieved as a byproduct of the ongoing
verifications that take place during a transaction's journey from proposal to
commitment.

Check out the :doc:`txflow` diagram for a visual representation
of consensus.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
