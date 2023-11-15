Service Discovery
=================

Why do we need service discovery?
---------------------------------

Client applications interact with the :doc:`Fabric Gateway <gateway>` to
execute chaincode on peers, submit transactions to orderers, and learn
about the status of transactions. The peer gateway service must therefore
know the relevant endorsement policies as well as which peers
have the chaincode installed.

The **discovery service** is responsible for keeping this information synchronized
across peers and then the :doc:`Fabric Gateway <gateway>` interacts with service discovery in order to determine
which peers are required to endorse a transaction, and which orderer nodes to send
the transaction to.

How service discovery works in Fabric
-------------------------------------

The application is bootstrapped knowing about one or more Gateway peers which are
trusted by the application developer/administrator to gather endorsements
from other peers. A good candidate peer to be used by the client application
is one that is in the same organization. Note that in order for peers to be known
to the discovery service, they must have a ``peer.gossip.externalEndpoint`` defined. To see
how to do this, check out our :doc:`discovery-cli` documentation.

A service discovery client such as the :doc:`discovery-cli` or another Gateway peer
issues a configuration query to the discovery service to obtain information about
peers in the channel. This information can be refreshed at any point
by sending a subsequent query to the discovery service of a peer.

The discovery service runs on peers and uses the network metadata
information maintained by the gossip communication layer to find out which peers
are online. It also fetches information, such as any relevant endorsement policies,
from the peer's state database.

With service discovery, the peer Gateway acting on behalf of a client application
simply sends a query to the discovery service
asking which peers are needed given a channel and a chaincode ID. The discovery
service will then compute a descriptor comprised of two objects:

1. **Layouts**: a list of groups of peers and a corresponding amount of peers from
   each group which should be selected.
2. **Group to peer mapping**: from the groups in the layouts to the peers of the
   channel. In practice, each group would most likely be peers that represent
   individual organizations, but because the service API is generic and ignorant of
   organizations this is just a "group".

The following is an example of a descriptor from the evaluation of a policy of
``AND(Org1, Org2)`` where there are two peers in each of the organizations.

.. code-block:: none

   Layouts: [
        QuantitiesByGroup: {
          “Org1”: 1,
          “Org2”: 1,
        }
   ],
   EndorsersByGroups: {
     “Org1”: [peer0.org1, peer1.org1],
     “Org2”: [peer0.org2, peer1.org2]
   }

In other words, the endorsement policy requires a signature from one peer in Org1
and one peer in Org2. And it provides the names of available peers in those orgs who
can endorse (``peer0`` and ``peer1`` in both Org1 and in Org2).

When coordinating a transaction, the peer Gateway selects an endorsement layout.
In the example above, the endorsement policy is Org1 ``AND`` Org2. It then selects
which endorsing peers to target from the layout using information such as which peers are
currently available and their ledger heights.

Capabilities of the discovery service
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The discovery service can respond to the following queries:

* **Configuration query**: Returns the ``MSPConfig`` of all organizations in the channel
  along with the orderer endpoints of the channel.
* **Peer membership query**: Returns the peers that have joined the channel.
* **Endorsement query**: Returns an endorsement descriptor for given chaincode(s) in
  a channel.
* **Local peer membership query**: Returns the local membership information of the
  peer that responds to the query. By default the client needs to be an administrator
  for the peer to respond to this query.

Special requirements
~~~~~~~~~~~~~~~~~~~~~~
When the peer is running with TLS enabled the client must provide a TLS certificate when connecting
to the peer. If the peer isn't configured to verify client certificates (clientAuthRequired is false), this TLS certificate
can be self-signed.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
