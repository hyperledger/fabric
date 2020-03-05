Peer channel-based event services
=================================

General overview
----------------

In previous versions of Fabric, the peer event service was known as the event
hub. This service sent events any time a new block was added to the peer's
ledger, regardless of the channel to which that block pertained, and it was only
accessible to members of the organization running the eventing peer (i.e., the
one being connected to for events).

Starting with v1.1, there are new services which provide events. These services use an
entirely different design to provide events on a per-channel basis. This means
that registration for events occurs at the level of the channel instead of the peer,
allowing for fine-grained control over access to the peer's data. Requests to
receive events are accepted from identities outside of the peer's organization (as
defined by the channel configuration). This also provides greater reliability and a
way to receive events that may have been missed (whether due to a connectivity issue
or because the peer is joining a network that has already been running).

Available services
------------------

* ``Deliver``

This service sends entire blocks that have been committed to the ledger. If
any events were set by a chaincode, these can be found within the
``ChaincodeActionPayload`` of the block.

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
``FilteredChaincodeAction`` of the filtered block.

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

SDK event documentation
-----------------------

For further details on using the event services, refer to the `SDK documentation. <https://hyperledger.github.io/fabric-sdk-node/{BRANCH}/tutorial-channel-events.html>`_

.. Licensed under Creative Commons Attribution 4.0 International License
    https://creativecommons.org/licenses/by/4.0/
