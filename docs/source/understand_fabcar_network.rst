Understanding the Fabcar Network
================================

Fabcar was designed to leverage a network stripped down to only the components
necessary to run an application. And even with that level of simplification,
the ``./startFabric.sh`` script takes care of the installation and
configuration not baked into the network itself.

Obscuring the underpinnings of the network to that degree is fine for the
majority of application developers. They don't necessarily need to know how
network components actually work in detail in order to create their app.

But for those who do want to know about the fun stuff going on under the covers
(so to speak), let's go through how applications **connect** to the network and
how they propose **queries** and **updates** on a more granular level, as well
as point out the differences between a small scale test network like Fabcar and
how apps will usually end up working in the real world.

We'll also point you to where you can get detailed information about how Fabric
networks are created and how a transaction flow works beyond the scope of the
role an application plays.

Components of the Fabcar Network
--------------------------------

The Fabcar network consists of one peer node, one ordering node (aka, the
"orderer"), a couchDB container, and a CLI container. This represents a
very limited network, without a certificate authority or any other
peers.

For detailed information on these components and what they do, refer to
:doc:`build_network`.

These components are bootstrapped by the ``./startFabric.sh`` script, which
also:
          * creates a channel and joins the peer to the channel
          * installs smart contract onto the peer's file system and instantiates it on the channel (instantiate starts a container)
          * calls the ``initLedger`` function to populate the channel ledger with 10 unique cars

These operations would typically be done by an organizational or peer admin.
The script uses the CLI to execute these commands, however there is support in
the SDK as well. Refer to the `Hyperledger Fabric Node SDK repo
<https://github.com/hyperledger/fabric-sdk-node>`__ for example scripts.

How an Application Interacts with the Network
---------------------------------------------

Applications use **APIs** to invoke smart contracts. These smart contracts are
hosted in the network and identified by name and version. For example, our
chaincode container is titled - ``dev-peer0.org1.example.com-fabcar-1.0`` -
where the name is ``fabcar``, the version is ``1.0``, and the peer it is running
against is ``dev-peer0.org1.example.com``.

APIs are accessible with an SDK. For purposes of this exercise, we're using the
`Hyperledger Fabric Node SDK <https://fabric-sdk-node.github.io/>`__ though
there is also a Java SDK and CLI that can be used to develop applications.
SDKs encapsulate all access to the ledger by allowing an application to
use smart contracts, run queries, or receive ledger updates. These APIs use
several different network addresses and are run with a set of input parameters.

Smart contracts are installed and instantiated on a channel through the
consensus process. The script that launched our simplified Fabcar test network
bypassed this process by installing and instantiating the smart contracts for
us on the lone peer in our network.

One crucial aspect of networks missing from Fabcar is the roll a certificate
authority (CA) plays issuing the certificates that allow users to query,
transact, and govern a network. This simplification was made because Fabcar is
really meant to show how applications connect to the network and issue queries
and updates rather than highlighting the enrollment and governance process.

In future iterations of Fabcar we'll go more into how enrollment works and how
different kinds of certificates are issued.

Query
^^^^^

Queries are the simplest kind of invocation: a call and response. Applications
can query different ledgers at the same time. Those results are returned to
the application **synchronously**. This does not necessarily ensure that each
ledger will return exactly the same information (a peer can go down, for
example, and miss updates). Given that our sample Fabcar network has only one
peer, that's not really an issue here, but it's an important consideration
when developing applications in a real world scenario.

The peers hold the hash chain (the record of updates), while the updates
themselves are stored in a separate couchDB container (which allows for the
storage of rich queries, written in JSON).

Queries are built using a **var request** -- identifying the correct ledger, the
smart contracts it will use, the search parameters etc -- and then invoking the
``chain.queryByChaincode`` API to send the query. An API called
``response_payload`` returns the result to the application.

Updates
^^^^^^^

Ledger updates start with an application generating a transaction proposal. A
request is constructed to identify the channel ID, function, and specific smart
contract to target for the transaction. The program then calls the
``channel.SendTransactionProposal`` API to send the transaction proposal to the
peer(s) for endorsement.

The network (i.e., the endorsing peer) returns a proposal response, which the
application uses to build and sign a transaction request. This request is sent
to the ordering service by calling the ``channel.sendTransaction`` API. The
ordering service bundles the transaction into a block and delivers it to all
peers on a channel for validation (the Fabcar network has only one endorsing
peer and one channel).

Finally the application uses two event handler APIs: ``eh.setPeerAddr`` to
connect to the peer's event listener port and ``eh.registerTxEvent`` to
register events associated with a specific transaction ID. The
``eh.registerTxEvent`` API registers a callback for the transactionID that
checks whether ledger was updated or not.

For More Information
--------------------

To learn more about how a transaction flow works beyond the scope of an
application, check out :doc:`txflow`.

To get started developing chaincode, read :doc:'chaincode4ade'.

For more information on how endorsement policies work, check out
:doc:`endorsement-policies`.

For a deeper dive into the architecture of Hyperledger Fabric, check out
:doc:`arch-deep-dive`.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
