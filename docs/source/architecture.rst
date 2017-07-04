Hyperledger Fabric is a unique implementation of distributed ledger
technology (DLT) that ensures data integrity and consistency while
delivering accountability, transparency, and efficiencies unmatched by
other blockchain or DLT technology.

Hyperledger Fabric implements a specific type of
:doc:`permissioned <glossary#permissioned-network>` :doc:`blockchain
network <glossary#blockchain-network>` on which members can track,
exchange and interact with digitized assets using
:doc:`transactions <glossary#transactions>` that are governed by smart
contracts - what we call :doc:`chaincode <glossary#chaincode>` - in a
secure and robust manner while enabling
:doc:`participants <glossary#participants>` in the network to interact
in a manner that ensures that their transactions and data can be
restricted to an identified subset of network participants - something
we call a :doc:`channel <glossary#channel>`.

The blockchain network supports the ability for members to establish
shared ledgers that contain the source of truth about those digitized
assets, and recorded transactions, that is replicated in a secure manner
only to the set of nodes participating in that channel.

The Hyperledger Fabric architecture is comprised of the following
components: peer nodes, ordering nodes and the clients applications that
are likely leveraging one of the language-specific Hyperledger Fabric SDKs.
These components have identities derived from certificate authorities.
Hyperledger Fabric also offers a certificate authority service,
*fabric-ca* but, you may substitute that with your own.

All peer nodes maintain the ledger/state by committing transactions. In
that role, the peer is called a :doc:`committer <glossary#commitment>`.
Some peers are also responsible for simulating transactions by executing
chaincodes (smart contracts) and endorsing the result. In that role the
peer is called an :doc:`endorser <glossary#endorsement>`. A peer may be an
endorser for certain types of transactions and just a ledger maintainer
(committer) for others.

The :doc:`orderers <glossary#ordering-service>` consent on the order of
transactions in a block to be committed to the ledger. In common
blockchain architectures (including earlier versions of the Hyperledger
Fabric) the roles played by the peer and orderer nodes were unified (cf.
validating peer in Hyperledger Fabric v0.6). The orderers also play a
fundamental role in the creation and management of channels.

Two or more :doc:`participants <glossary#participant>` may create and
join a channel, and begin to interact. Among other things, the policies
governing the channel membership and chaincode lifecycle are specified
at the time of channel creation. Initially, the members in a channel
agree on the terms of the chaincode that will govern the transactions.
When consensus is reached on the :doc:`proposal <glossary#proposal>` to
deploy a given chaincode (as governed by the life cycle policy for the
channel), it is committed to the ledger.

Once the chaincode is deployed to the peer nodes in the channel, :doc:`end
users <glossary#end-users>` with the right privileges can propose
transactions on the channel by using one of the language-specific client
SDKs to invoke functions on the deployed chaincode.

The proposed transactions are sent to endorsers that execute the
chaincode (also called "simulated the transaction"). On successful
execution, endorse the result using the peer's identity and return the
result to the client that initiated the proposal.

The client application ensures that the results from the endorsers are
consistent and signed by the appropriate endorsers, according to the
endorsement policy for that chaincode and, if so, the application then
sends the transaction, comprised of the result and endorsements, to the
ordering service.

Ordering nodes order the transactions - the result and endorsements
received from the clients - into a block which is then sent to the peer
nodes to be committed to the ledger. The peers then validate the
transaction using the endorsement policy for the transaction's chaincode
and against the ledger for consistency of result.

Some key capabilities of Hyperledger Fabric include:

-  Allows for complex query for applications that need ability to handle
   complex data structures.

-  Implements a permissioned network, also known as a consortia network,
   where all members are known to each other.

-  Incorporates a modular approach to various capabilities, enabling
   network designers to plug in their preferred implementations for
   various capabilities such as consensus (ordering), identity
   management, and encryption.

-  Provides a flexible approach for specifying policies and pluggable
   mechanisms to enforce them.

-  Ability to have multiple channels, isolated from one another, that
   allows for multi-lateral transactions amongst select peer nodes,
   thereby ensuring high degrees of privacy and confidentiality required
   by competing businesses and highly regulated industries on a common
   network.

-  Network scalability and performance are achieved through separation
   of chaincode execution from transaction ordering, which limits the
   required levels of trust and verification across nodes for
   optimization.

For a deeper dive into the details, please visit :doc:`this
document <arch-deep-dive>`.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

