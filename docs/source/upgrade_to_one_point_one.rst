Procedure for Upgrading from v1.0.x
===================================

At a high level, upgrading a Fabric network can be performed with the following sequence.

 * Update orderers, peers, and fabric-ca.  These updates may be done in parallel.
 * Update client SDKs.
 * Enable v1.1 channel capability requirements.
 * (Optional) Update the Kafka cluster.

While the above represents a best practice of the order in which to perform an update
from version 1.0.x to version 1.1, it’s worth first understanding the concept of
“Capability Requirements” to know how and why it’s important to update to new versions
and/or incorporate new components into your network (i.e., the orderer system channel),
or individual channels.

Fabric Capability Requirements
------------------------------

Since Fabric is a distributed system that will often involve multiple organizations
(sometimes in different countries or even continents), it is possible (and typical)
that many different versions of Fabric code will exist in the network. Nevertheless,
this code -- and the machines it lives on -- must process transactions in the same
way *across the network* so that everyone has the same view of the current network
state. This means that every network -- and every channel within that network -- must
have within itself a common set of features to be able to participate in processing
transactions on a channel. We call this common set of features “Capability Requirements.”

For example, Fabric v1.1 introduces new MSP role types of “Peer”, “Orderer”, and
“Client”. However, if a v1.0 peer does not understand these new role types, it will
not be able to appropriately evaluate an endorsement policy that references them.
This means that before the new role types may be used, the network must agree to
enable the v1.1 channel capability requirement. This ensures that all peers come
to the same decision.

Where are Capability Requirements Defined?
------------------------------------------

Capability requirements are defined per channel in the channel configuration (found
in the channel’s most recent configuration block). The channel configuration contains
three locations, each of which defines a capability of a different type.

* Channel-wide capabilities -- across both peers and orderers -- are located in the
  root ‘Channel’ group.

* Application specific (peer related) capabilities are located in the "Application" group.

* Orderer-specific capabilities are located in the "Orderer" group.

The capabilities are broken into these groups because of their existent administrative
structure. Updating orderer capabilities is something the ordering orgs would manage
autonomously from the application orgs. Similarly, updating application capabilities
is something only the application admins would manage. By splitting the capabilities
between "Orderer" and "Application", a hypothetical network could run a v1.6 ordering
service while supporting a v1.3 peer application network.

However, some capabilities cross both the ‘Application’ and ‘Orderer’ groups. As we
saw earlier, adding a new MSP role type is something both the orderer and application
admins agree to. The orderer must understand the meaning of MSP roles in order to
allow the transactions to pass through ordering, while the peers must understand the
roles in order to validate the transaction. These kinds of capabilities -- which span
both the application and orderer components -- are defined in the top level "Channel"
group.

.. note:: It is possible that the channel capabilities are defined to be at version
          v1.3, while the orderer and application capabilities are defined to be at
          version 1.1 and v1.2 respectively. Enabling a capability at the "Channel"
          group level does not imply that this same capability is available at the
          more specific "Orderer" and "Application" group levels.

Now that we’ve shown why capability requirements are important, let’s move on to how
you actually update your components. We’ll discuss how you update the capabilities a
little later.

First, let’s update your orderer.

Update Orderer Binaries
-----------------------

Orderer binaries should be upgraded in a rolling fashion (one at a time). For each
orderer process:

1. Stop the orderer.
2. Backup the orderer's ledger and MSP.
3. Update the orderer binary to v1.1.x.
4. For native deployments, replace the file ‘orderer’ with the one from the release
   artifacts.

.. note:: For Docker deployments, you must set the environment variable
          ``ORDERER_KAFKA_VERSION`` to match your Kafka cluster version, even if it
          was not set before.

5. Update the image version.
6. Start the orderer.
7. Verify that the new orderer starts up and synchronizes with the rest of the network.
8. First, using the peer CLI, use the peer channel fetch newest command to verify that
   the orderer has started.
9. Next, send some transactions to the new orderer, either using the SDK or the CLI.
   Verify that these transactions successfully commit.

Repeat this process for each orderer.

Update Chaincodes With Vendored Shim
------------------------------------

1. For any chaincodes which used Go vendoring to include the chaincode shim, the source
   code must be updated in one of two ways:

   * Remove the vendoring of the shim.
   * Update the vendored version of the shim to use the v1.1.0 Fabric source.

2. Re-package the updated chaincode.
3. Install the chaincode on all peers which have the original version of the chaincode
   installed. Install with the same name, but specify a new version.

Update Peer Binaries
--------------------

Peer binaries should be upgraded in a rolling fashion (one at a time). For each peer
process:

1. Stop the peer.
2. Backup the peer’s ledger and local MSP directories.

If using CouchDB as state database:

1. Stop CouchDB.
2. Backup CouchDB data directory.
3. Delete CouchDB data directory.
4. Install CouchDB 2.1.1 binaries or update the Docker image (CouchDB 2.1.1 pre-configured
   Docker image is provided alongside Hyperledger Fabric 1.1).
5. Restart CouchDB.

The reason to delete the CouchDB data directory is that upon startup the 1.1 peer
will rebuild the CouchDB state databases from the blockchain transactions. Starting
in 1.1, there will be an internal CouchDB database for each channel_chaincode combination
(for each chaincode instantiated on each channel that the peer has joined).

Next, remove all Docker chaincode images.

These can be recognized by the pattern:

``${CORE_PEER_NETWORKID}-${CORE_PEER_ID}-${CC_NAME}-${CC_VERSION}-${CC_HASH}``

for instance:

``dev-peer1.org2.example.com-mycc-1.0-26c2ef32838554aac4f7ad6f100aca865e87959c9a126e86d764c8d01f8346ab``

Then, update the peer binary to v1.1.x. For native deployments, replace the file ``peer``
with the one from the release artifacts. For Docker deployments, update the image version.

Now, start the peer, making sure to verify that the peer blockchain syncs with the
rest of the network and can endorse transactions.

Once peer binaries have been updated, send an upgrade transaction on each channel for
any chaincodes that were rebuilt to remove the v1.0.x chaincode shim. This upgrade
transaction should specify the new chaincode version which was selected during Update
Chaincodes With Vendored Shim.

Update fabric-ca binary
-----------------------

The fabric-ca-server must be upgraded before upgrading the fabric-ca-client.

To upgrade a single instance of fabric-ca-server which uses the sqlite3 database:

1. Stop the fabric-ca-server process.
2. Backup the sqlite3 database file (which is named fabric-ca-server.db by default).
3. Upgrade to the fabric-ca-server v1.1 binary.
4. Launch the fabric-ca-server process.
5. Verify the fabric-ca-server process is available with the following command where
   ``<host>`` is the hostname on which the server was started:

.. code:: bash

  fabric-ca-client getcacert -u https://<host>:7054 --tls.certfiles tls-cert.pem

.. note:: This step assumes that the server was launched with TLS enabled; otherwise,
          use “http” instead of “https”. It also assumes that the server is listening
          on the default port (7054). The “tls-cert.pem” is the TLS certificate file
          used by the fabric-ca-server.

To upgrade a cluster of fabric-ca-server instances, do the following one cluster member
at a time. We assume the cluster members are using either a MySQL or Postgres database.

1. Stop the fabric-ca-server process.
2. Upgrade to the fabric-ca-server v1.1 binary.
3. Launch the fabric-ca-server process.
4. Verify the fabric-ca-server process is available as shown above in step 5.

To upgrade the fabric-ca-client, simply replace the fabric-ca-client v1.0 binary with
the v1.1 binary.

Update Node SDK Clients
-----------------------

**Warning: Upgrade fabric-ca before upgrading Node SDK clients.**

Use NPM to upgrade any Node.js client by executing in the root dir of your application,
the following commands:

.. code:: bash

  npm install fabric-client@1.1
  npm install fabric-ca-client@1.1

These commands install the new version of both the Fabric client and fabric-ca client
and updates “package.json”.

Setting Capabilities
--------------------

Capabilities are set as part of the channel configuration (either as part of the **initial
configuration** or as part of a **reconfiguration**, also known as an **update configuration**).

Capabilities in an Initial Configuration
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

In the ``configtx.yaml`` file there is a ``Capabilities`` section which enumerates the
possible capabilities for each capability type (Channel, Orderer, and Application).

The simplest way to enable capabilities is to pick a v1.1 sample profile and customize
it for your network, for example:

.. code:: bash

    SampleSingleMSPSoloV1_1:
        Capabilities:
            <<: *GlobalCapabilities
        Orderer:
            <<: *OrdererDefaults
            Organizations:
                - *SampleOrg
            Capabilities:
                <<: *OrdererCapabilities
        Consortiums:
            SampleConsortium:
                Organizations:
                    - *SampleOrg


Note that there is a ``Capabilities`` section defined at the root level (for the channel
capabilities), and at the Orderer level (for orderer capabilities). The sample above uses
a YAML reference to include the capabilities as defined at the bottom of the YAML.

When defining the orderer system channel there is usually no Application section, as those
capabilities are defined during the creation of an application channel. To do this,
application admins should create their channel modeling after the
``SampleSingleMSPChannelV1_1`` profile.

.. code:: bash

   SampleSingleMSPChannelV1_1:
        Consortium: SampleConsortium
        Application:
            Organizations:
                - *SampleOrg
            Capabilities:
                <<: *ApplicationCapabilities

Here, the Application section has a new element ``Capabilities`` which references the
``ApplicationCapabilities`` section defined at the end of the YAML.

.. note:: The capabilities for the Channel and Orderer sections are inherited from
          the definition in the ordering system channel and are automatically included
          by the orderer during the process of channel creation.

Capabilities in a Configuration Update
--------------------------------------

For networks which have already been bootstrapped, setting capability requirements
are done as a channel reconfiguration.

Capabilities are found in the channel configuration according to the following table:

+------------------+-----------------------------------+----------------------------------------------------+
| Capability Type  | Canonical Path                    | JSON Path                                          |
+==================+===================================+====================================================+
| Channel          | /Channel/Capabilities             | .channel_group.values.Capabilities                 |
+------------------+-----------------------------------+----------------------------------------------------+
| Orderer          | /Channel/Orderer/Capabilities     | .channel_group.groups.Orderer.values.Capabilities  |
+------------------+-----------------------------------+----------------------------------------------------+
| Application      | /Channel/Application/Capabilities | .channel_group.groups.Application.values.          |
|                  |                                   | Capabilities                                       |
+------------------+-----------------------------------+----------------------------------------------------+

The schema for the Capabilities value is defined in protobuf as:

.. code:: bash

  message Capabilities {
        map<string, Capability> capabilities = 1;
  }

  message Capability { }

As an example, rendered in JSON:

.. code:: bash

  {
      "capabilities": {
          "V1_1": {}
      }
  }

To update a configuration, simply pull the current configuration, update the desired
``Capabilities`` value to include the new capability, compute the config update, collect
signatures, and submit.

Enable Channel Capability Requirements
--------------------------------------

For background, please refer to the "Fabric Capability Requirements" section above before
proceeding.

.. note:: Ensure all orderer binaries are upgraded to v1.1.0+ before enabling any
          capabilities.

Because the v1.0.x Fabric binaries do not understand the concept of channel capabilities,
extra care must be taken when initially enabling capabilities for a channel.

Although Fabric binaries can and should be upgraded in a rolling fashion, **it is
critical that the ordering admins not attempt to enable v1.1 capabilities until all
orderer binaries are at v1.1.0+**. If any orderer is executing v1.0.x code, and
capabilities are enabled for a channel, the blockchain will fork as v1.0.0 orderers
invalidate the change and v1.1.0+ orderers accept it.  This is an exception for the
v1.0 to v1.1 upgrade. For future upgrades, such as v1.1 to v1.2, the ordering network
will handle the upgrade more gracefully and prevent the state fork.

In order to minimize the chance of a fork, the orderer v1.1 capability must be enabled
first in a transition from v1.0.x to v1.1. Since this upgrade may only be enabled by the
ordering admins, it prevents application admins from accidentally enabling capabilities
before the orderer is ready to support them.

.. note:: Once a capability has been enabled, disabling it is not recommended or supported.

Because Fabric is blockchain technology, all of the peers and orderers on a channel
process the entirety of the blockchain to arrive at the current state of that channel.
As a result, once a capability has been enabled, it becomes part of the permanent record
for that channel. This means that even after disabling the capability, old binaries will
not be able to participate in the channel, because they cannot process beyond the block
which enabled the capability.

For this reason, think of enabling channel capabilities as a ‘point of no return’. Please
experiment with the new capabilities in a test setting and be confident before proceeding
to enable them in production.

.. note:: Although all peer binaries in the network should have been upgraded prior
          to this point, enabling capability requirements on a channel which a v1.0.0
          peer is joined to will result in a crash of the peer.  This crashing behavior
          is deliberate because it indicates a misconfiguration which might result in a
          state fork.

To upgrade the orderer system channel, first enable the orderer group v1.1 capability.
When bootstrapping the orderer, a channel ID should have been specified. If no channel
ID was specified, then most likely the ID of the orderer system channel is ``testchainid``.

Enabling a capability is done like all other channel configuration, you may see instructions
for this in the “Capabilities as Updated Configuration” section.

Next, enable the channel group v1.1 capability. Once the orderer system channel has been
upgraded, any newly created channels will include the orderer and channel group capabilities
as specified in the orderer system channel. To create new channels with v1.1 application
capabilities, include the capability definition in the channel creation transaction.

Then, for each each channel (other than the orderer system channel):

 * Enable the orderer group v1.1 capability.
 * Enable the application group v1.1 capability.
 * Enable the channel group v1.1 capability.

At this point, the entire network should be upgraded with v1.1 capabilities and the upgrade
is complete.

Upgrading the Kafka Cluster
---------------------------

It is not required, but it is recommended that the Kafka cluster be upgraded and kept
up to date along with the rest of Fabric. Newer versions of Kafka support older protocol
versions, so you may update Kafka before or after the result of Fabric.

If your Kafka cluster is older than Kafka v0.11.0, this upgrade is especially recommended
as it  hardens replication in order to better handle crash faults which can exhibit
problems such as seen in FAB-7330.

No configuration changes to the orderer are required when the Kafka brokers are upgraded
to a newer version. Refer to the official Apache Kafka documentation on upgrading Kafka
from previous versions on how to upgrade the Kafka brokers.

Please note that the Kafka cluster might experience a negative performance impact if
the value of Kafka.Version in the orderer.yaml is older than the Kafka broker version.

Upgrading CouchDB
-----------------

If using CouchDB as your state database, upgrade CouchDB binaries or Docker images to
2.1.1 when upgrading each peer to Hyperledger Fabric 1.1, as described in the peer
upgrade instructions. The CouchDB 2.1.1 Docker images provided alongside Hyperledger
Fabric 1.1 have a configuration that has been verified to work with v1.1 peers.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
