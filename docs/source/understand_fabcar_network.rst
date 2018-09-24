Understanding the Fabcar Network
================================

Fabcar was designed to leverage a network stripped down to only the components
necessary to run an application. And even with that level of simplification,
the ``./startFabric.sh`` script takes care of the installation and
configuration not baked into the network itself.

Obscuring the underpinnings of the network to that degree is fine for the
majority of application developers. They don't necessarily need to know how
network components actually work in detail in order to create their app.

But for those who do want to know about the fun stuff going on under the covers,
let's go through how applications **connect** to the network and
how they propose **queries** and **updates** on a more granular level, as well
as point out the differences between a small scale test network like Fabcar and
how apps will usually end up working in the real world.

We'll also point you to where you can get detailed information about how Fabric
networks are created and how a transaction flow works beyond the scope of the
role an application plays.

Components of the Fabcar Network
--------------------------------

Fabcar uses the "basic-network" sample as its limited development network. It
consists of a single peer node configured to use CouchDB as the state database,
a single "solo" ordering node, a certificate authority (CA) and a CLI container
for executing commands.

For detailed information on these components and what they do, refer to
:doc:`build_network`.

These components are bootstrapped by the ``./startFabric.sh`` script, which
also:

* creates a channel and joins the peer to the channel
* installs the ``fabcar`` smart contract onto the peer's file system and instantiates it on the channel (instantiate starts a container)
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
there is also a Java SDK and CLI that can be used to drive transactions.
SDKs encapsulate all access to the ledger by allowing an application to
communicate with smart contracts, run queries, or receive ledger updates. These APIs use
several different network addresses and are run with a set of input parameters.

Smart contracts are installed by a peer administrator and then instantiated on a
channel by an identity fulfilling the chaincode's instantiation policy, which by
default is comprised of channel administrators.  The instantiation of
the smart contract follows the same transaction flow as a normal invocation - endorse,
order, validate, commit - and is a prerequisite to interacting with a chaincode
container. The script that launched our simplified Fabcar test network took care
of the installation and instantiation for us.

Query
^^^^^

Queries are the simplest kind of invocation: a call and response.  The most common query
will interrogate the state database for the current value associated
with a key (``GetState``).  However, the `chaincode shim interface <https://godoc.org/github.com/hyperledger/fabric/core/chaincode/shim#ChaincodeStub>`__
also allows for different types of ``Get`` calls (e.g. ``GetHistoryForKey`` or ``GetCreator``).

In our example, the peer holds a hash chain of all transactions and maintains
chaincode state through use of a state database, which in our case is a CouchDB container.  CouchDB
provides the added functionality of rich queries, contingent upon the chaincode data (key/val pairs)
being modeled as JSON.  When we call the ``GetState`` API in our smart contract, we
are retrieving the JSON value associated with a car from the CouchDB state database.

Queries are constructed by identifying a peer, a chaincode, a channel and a set of
inputs (e.g. the key) for an available chaincode function and then utilizing the
``chain.queryByChaincode`` API to send the query to the peer.  The corresponding
value to the supplied inputs is returned to the application client as a response.

Updates
^^^^^^^

Ledger updates start with an application generating a transaction proposal. As with
query, a request is constructed to identify a peer, chaincode, channel, function, and
set of inputs for the transaction. The program then calls the
``channel.SendTransactionProposal`` API to send the transaction proposal to the
peer(s) for endorsement.

The network (i.e. the endorsing peer(s)) returns a proposal response, which the
application uses to build and sign a transaction request. This request is sent
to the ordering service by calling the ``channel.sendTransaction`` API. The
ordering service bundles the transaction into a block and delivers it to all
peers on a channel for validation (the Fabcar network has only one peer and one channel).

Finally the application uses the :doc:`peer_event_services` to register for events
associated with a specific transaction ID so that the application can be notified
about the fate of a transaction (i.e. valid or invalid).

For More Information
--------------------

To learn more about how a transaction flow works beyond the scope of an
application, check out :doc:`txflow`.

To get started developing chaincode, read :doc:`chaincode4ade`.

For more information on how endorsement policies work, check out
:doc:`endorsement-policies`.

For a deeper dive into the architecture of Hyperledger Fabric, check out
:doc:`arch-deep-dive`.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
