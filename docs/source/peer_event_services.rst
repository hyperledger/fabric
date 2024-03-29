Peer channel-based event services
=================================

General overview
----------------

The peer's event service allows client applications to receive block events as blocks are committed to a peer's ledger.
Client applications can also request block events starting from a certain block number,
allowing client applications to receive prior block events or to resume receiving events
in case the application or the peer has been restarted or lost connection.

Requests to receive events are accepted from client identities of the same organization
and also outside of the peer's organization (as defined by the channel configuration).

Several different types of block events are available from a peer depending on the service requested.
Block events also include any chaincode events from the transactions within the block.
Each transaction may have a single chaincode event, which can be emitted by the called chaincode.

This topic describes the low-level peer event services (gRPC services) for block events.
Fabric Gateway application API (for Fabric v2.4 onwards) internally utilizes the peer event services and make block events and chaincode events
available to client applications through the client APIs for Fabric v2.4 onwards.

Available services
------------------

* ``Deliver``

This service sends entire blocks that have been committed to the ledger. If
any events were set by a chaincode, these can be found within the
``ChaincodeActionPayload`` of each transaction within the block.

* ``DeliverWithPrivateData``

This service sends the same data as the ``Deliver`` service, and additionally
includes any private data from collections that the client's organization is
authorized to access.

* ``DeliverFiltered``

This service sends "filtered" blocks, minimal sets of information about blocks
that have been committed to the ledger. It is intended to be used in a network
where owners of the peers wish for external clients to primarily receive
information about their transactions and the status of those transactions. If
any events were set by a chaincode, these can be found within the
``FilteredChaincodeAction`` of each transaction within the block.

.. note:: The payload of chaincode events will not be included in filtered blocks.

How to register for events
--------------------------

Registration for events is done by sending an envelope
containing a deliver seek info message to the peer that contains the desired start
and stop positions, the seek behavior (block until ready or fail if not ready).
There are helper variables ``SeekOldest`` and ``SeekNewest`` that can be used to
indicate the oldest (i.e. first) block or the newest (i.e. last) block on the ledger.
To have the services send events indefinitely, the ``SeekInfo`` message should
include a stop position of ``MAXINT64``.

.. note:: If mutual TLS is enabled on the peer, the TLS certificate hash must be
          set in the envelope's channel header.

By default, the event services use the Channel Readers policy to determine whether
to authorize requesting clients for events.

Overview of deliver response messages
-------------------------------------

The event services send back ``DeliverResponse`` messages.

Each message contains one of the following:

 * status -- HTTP status code. Each of the services will return the appropriate failure
   code if any failure occurs; otherwise, it will return ``200 - SUCCESS`` once
   the service has completed sending all information requested by the ``SeekInfo``
   message.
 * block -- returned only by the ``Deliver`` service.
 * block and private data -- returned only by the ``DeliverWithPrivateData`` service.
 * filtered block -- returned only by the ``DeliverFiltered`` service.

A filtered block contains:

 * channel ID.
 * number (i.e. the block number).
 * array of filtered transactions.
 * transaction ID.

   * type (e.g. ``ENDORSER_TRANSACTION``, ``CONFIG``).
   * transaction validation code.

 * filtered transaction actions.
     * array of filtered chaincode actions.
        * chaincode event for the transaction (with the payload nilled out).

Application API documentation
-----------------------------

The Fabric Gateway application API allows client applications to receive
block events from all committed blocks and chaincode events emitted by successfully committed transactions. These events
can be used to trigger external business processes in response to ledger
updates. For further details, refer to the API documentation:

* `Go <https://pkg.go.dev/github.com/hyperledger/fabric-gateway/pkg/client#Network.ChaincodeEvents>`_
* `Node.js <https://hyperledger.github.io/fabric-gateway/main/api/node/interfaces/Network.html#getChaincodeEvents>`_
* `Java <https://hyperledger.github.io/fabric-gateway/main/api/java/org/hyperledger/fabric/client/Network.html>`_

Legacy application SDKs (Note: Deprecated in Fabric v2.5) use the deliver service and allow client applications
to receive block events, or only chaincode events emitted by successfully
committed transactions within those blocks.

For further details, refer to the :doc:`SDK documentation <sdk_chaincode>`.

.. Licensed under Creative Commons Attribution 4.0 International License
    https://creativecommons.org/licenses/by/4.0/
