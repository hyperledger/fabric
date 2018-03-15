Upgrading Your Network Components
=================================

.. note:: When we use the term “upgrade” in this documentation, we’re primarily
          referring to changing the version of a component (for example, going
          from a v1.0.x binary to a v1.1 binary). The term “update,” on the other
          hand, refers not to versions but to configuration changes, such as
          updating a channel configuration or a deployment script.

Overview
--------

Because the :doc:`build_network` (BYFN) tutorial defaults to the “latest” binaries, if
you have run it since the release of v1.1, your machine will have v1.1 binaries
and tools installed on it and you will not be able to upgrade them.

As a result, this tutorial will provide a network based on Hyperledger Fabric
v1.0.6 binaries as well as the v1.1 binaries you will be upgrading to. In addition,
we will show how to update channel configurations to recognize :doc:`capability_requirements`.

However, because BYFN does not support the following components, our script for
upgrading BYFN will not cover them:

* **Fabric-CA**
* **Kafka**
* **SDK**

The process for upgrading these components will be covered in a section following
the tutorial.

At a high level, our upgrade tutorial will perform the following steps:

1. Back up the ledger and MSPs.
2. Upgrade the orderer binaries to Fabric v1.1.
3. Upgrade the peer binaries to Fabric v1.1.
4. Enable v1.1 channel capability requirements.

.. note:: In production environments, the orderers and peers can be upgraded
          on a rolling basis simultaneously (in other words, you don't need to
          upgrade your orderers before upgrading your peers). Where extra care
          must be taken is in enabling capabilities. All of the orderers and peers
          must be upgraded before that step (if only some orderers have been
          upgraded when capabilities have been enabled, a catastrophic state fork
          can be created).

This tutorial will demonstrate how to perform each of these steps individually
with CLI commands.

Prerequisites
~~~~~~~~~~~~~

If you haven’t already done so, ensure you have all of the dependencies on your
machine as described in :doc:`prereqs`.

Launch a v1.0.6 Network
-----------------------

To begin, we will provision a basic network running Fabric v1.0.6 images. This
network will consist of two organizations, each maintaining two peer nodes, and
a “solo” ordering service.

We will be operating from the ``first-network`` subdirectory within your local clone
of ``fabric-samples``. Change into that directory now. You will also want to open a
few extra terminals for ease of use.

Clean up
~~~~~~~~

We want to operate from a known state, so we will use the ``byfn.sh`` script to
initially tidy up. This command will kill any active or stale docker containers
and remove any previously generated artifacts. Run the following command:

.. code:: bash

  ./byfn.sh -m down

Generate the Crypto and Bring Up the Network
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

With a clean environment, launch our v1.0.6 BYFN network using these four commands:

.. code:: bash

  git fetch origin

  git checkout v1.0.6

  ./byfn.sh -m generate

  ./byfn.sh -m up -t 3000 -i 1.0.6

.. note:: If you have locally built v1.0.6 images, then they will be used by the example.
          If you get errors, consider cleaning up v1.0.6 images and running the example
          again. This will download 1.0.6 images from docker hub.

If BYFN has launched properly, you will see:

.. code:: bash

  ===================== All GOOD, BYFN execution completed =====================

We are now ready to upgrade our network to Hyperledger Fabric v1.1.

Get the newest samples
~~~~~~~~~~~~~~~~~~~~~~

.. note:: The instructions below pertain to whatever is the most recently
          published version of v1.1.x, starting with 1.1.0-rc1. Please substitute
          '1.1.x' with the version identifier of the published release that
          you are testing. e.g. replace 'v1.1.x' with 'v1.1.0'.

Before completing the rest of the tutorial, it's important to get the v1.1.x
version of the samples, you can do this by:

.. code:: bash

  git fetch origin

  git checkout v1.1.x

Want to upgrade now?
~~~~~~~~~~~~~~~~~~~~

We have a script that will upgrade all of the components in BYFN as well as
enabling capabilities. Afterwards, we will walk you through the steps
in the script and describe what each piece of code is doing in the upgrade process.

To run the script, issue these commands:

.. code:: bash

  # Note, replace '1.1.x' with a specific version, for example '1.1.0'.
  # Don't pass the image flag '-i 1.1.x' if you prefer to default to 'latest' images.

  ./byfn.sh upgrade -i 1.1.x

If the upgrade is successful, you should see the following:

.. code:: bash

  ===================== All GOOD, End-2-End UPGRADE Scenario execution completed =====================

if you want to upgrade the network manually, simply run ``./byfn.sh -m down`` again
and perform the steps up to -- but not including -- ``./byfn.sh upgrade -i 1.1.x``.
Then proceed to the next section.

.. note:: Many of the commands you'll run in this section will not result in any
          output. In general, assume no output is good output.

Upgrade the Orderer Containers
------------------------------

.. note:: Pay **CLOSE** attention to your orderer upgrades. If they are not done
          correctly -- specifically, if only some orderers are upgraded and not
          others -- a state fork could be created (meaning, ledgers would no
          longer be consistent). This MUST be avoided.

Orderer containers should be upgraded in a rolling fashion (one at a time). At a
high level, the orderer upgrade process goes as follows:

1. Stop the orderer.
2. Back up the orderer’s ledger and MSP.
3. Restart the orderer with the latest images.
4. Verify upgrade completion.

As a consequence of leveraging BYFN, we have a solo orderer setup, therefore, we
will only perform this process once. In a Kafka setup, however, this process will
have to be performed for each orderer.

.. note:: This tutorial uses a docker deployment. For native deployments,
          replace the file ``orderer`` with the one from the release artifacts.
          Backup the ``orderer.yaml`` and replace it with the ``orderer.yaml``
          file from the release artifacts. Then port any modified variables from
          the backed up ``orderer.yaml`` to the new one. Utilizing a utility
          like ``diff`` may be helpful. To decrease confusion, the variable
          ``General.TLS.ClientAuthEnabled`` has been renamed to ``General.TLS.ClientAuthRequired``
          (just as it is specified in the peer configuration.). If the old name
          for this variable is still present in the ``orderer.yaml`` file, the
          new ``orderer`` binary will fail to start.

Let’s begin the upgrade process by **bringing down the orderer**:

.. code:: bash

  docker stop orderer.example.com

  export LEDGERS_BACKUP=./ledgers-backup

  # Note, replace '1.1.x' with a specific version, for example '1.1.0'.
  # Set IMAGE_TAG to 'latest' if you prefer to default to the images tagged 'latest' on your system.

  export IMAGE_TAG=`uname -m`-1.1.x

We have created a variable for a directory to put file backups into, and
exported the ``IMAGE_TAG`` we'd like to move to.

Once the orderer is down, you'll want to **backup its ledger and MSP**:

.. code:: bash

  mkdir -p $LEDGERS_BACKUP

  docker cp orderer.example.com:/var/hyperledger/production/orderer/ ./$LEDGERS_BACKUP/orderer.example.com

In a production network this process would be repeated for each of the Kafka-based
orderers in a rolling fashion.

Now **download and restart the orderer** with our new fabric image:

.. code:: bash

  docker-compose -f docker-compose-cli.yaml up -d --no-deps orderer.example.com

Because our sample uses a "solo" ordering service, there are no other orderers in the
network that the restarted orderer must sync up to. However, in a production network
leveraging Kafka, it will be a best practice to issue ``peer channel fetch <blocknumber>``
after restarting the orderer to verify that it has caught up to the other orderers.

Upgrade the Peer Containers
---------------------------

Next, let's look at how to upgrade peer containers to Fabric v1.1. Peer containers should,
like the orderers, be upgraded in a rolling fashion (one at a time). As mentioned
during the orderer upgrade, orderers and peers may be upgraded in parallel, but for
the purposes of this tutorial we’ve separated the processes out. At a high level,
we will perform the following steps:

1. Stop the peer.
2. Back up the peer’s ledger and MSP.
3. Remove chaincode containers and images.
4. Restart the peer with with latest image.
5. Verify upgrade completion.

We have four peers running in our network. We will perform this process once for
each peer, totaling four upgrades.

.. note:: Again, this tutorial utilizes a docker deployment. For **native**
          deployments, replace the file ``peer`` with the one from the release
          artifacts. Backup your ``core.yaml`` and replace it with the one from
          the release artifacts. Port any modified variables from the backed up
          ``core.yaml`` to the new one. Utilizing a utility like ``diff`` may be
          helpful.

Let’s **bring down the first peer** with the following command:

.. code:: bash

   export PEER=peer0.org1.example.com

   docker stop $PEER

We can then **backup the peer’s ledger and MSP**:

.. code:: bash

  mkdir -p $LEDGERS_BACKUP

  docker cp $PEER:/var/hyperledger/production ./$LEDGERS_BACKUP/$PEER

With the peer stopped and the ledger backed up, **remove the peer chaincode
containers**:

.. code:: bash

  CC_CONTAINERS=$(docker ps | grep dev-$PEER | awk '{print $1}')
  if [ -n "$CC_CONTAINERS" ] ; then docker rm -f $CC_CONTAINERS ; fi

And the peer chaincode images:

.. code:: bash

  CC_IMAGES=$(docker images | grep dev-$PEER | awk '{print $1}')
  if [ -n "$CC_IMAGES" ] ; then docker rmi -f $CC_IMAGES ; fi

Now we'll re-launch the peer using the v1.1 image tag:

.. code:: bash

  docker-compose -f docker-compose-cli.yaml up -d --no-deps $PEER

.. note:: Although, BYFN supports using CouchDB, we opted for a simpler
          implementation in this tutorial. If you are using CouchDB, however,
          follow the instructions in the **Upgrading CouchDB** section below at
          this time and then issue this command instead of the one above:

.. code:: bash

  docker-compose -f docker-compose-cli.yaml -f docker-compose-couch.yaml up -d --no-deps $PEER

We'll talk more generally about how to update CouchDB after the tutorial.

Verify Upgrade Completion
~~~~~~~~~~~~~~~~~~~~~~~~~

We’ve completed the upgrade for our first peer, but before we move on let’s check
to ensure the upgrade has been completed properly with a chaincode invoke. Let’s
move ``10`` from ``a`` to ``b`` using these commands:

.. code:: bash

  docker-compose -f docker-compose-cli.yaml up -d --no-deps cli

  docker exec -it cli bash

  peer chaincode invoke -o orderer.example.com:7050  --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem  -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

Our query earlier revealed a to have a value of ``90`` and we have just removed
``10`` with our invoke. Therefore, a query against ``a`` should reveal ``80``.
Let’s see:

.. code:: bash

  peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

We should see the following:

.. code:: bash

  Query Result: 80

After verifying the peer was upgraded correctly, make sure to issue an ``exit``
to leave the container before continuing to upgrade your peers. You can
do this by repeating the process above with a different peer name exported.

.. code:: bash

  export PEER=peer1.org1.example.com
  export PEER=peer0.org2.example.com
  export PEER=peer1.org2.example.com

.. note:: All peers must be upgraded BEFORE enabling capabilities.

Enable Capabilities for the Channels
------------------------------------

Because v1.0.x Fabric binaries do not understand the concept of channel capabilities,
extra care must be taken when initially enabling capabilities for a channel.

Although Fabric binaries can and should be upgraded in a rolling fashion, **it is
critical that the ordering admins not attempt to enable v1.1 capabilities until all
orderer binaries are at v1.1.x+**. If any orderer is executing v1.0.x code, and
capabilities are enabled for a channel, the blockchain will fork as v1.0.x orderers
invalidate the change and v1.1.x+ orderers accept it. This is an exception for the
v1.0 to v1.1 upgrade. For future upgrades, such as v1.1 to v1.2, the ordering network
will handle the upgrade more gracefully and prevent the state fork.

In order to minimize the chance of a fork, attempts to enable the application or channel
v1.1 capabilities before enabling the orderer v1.1 capability will be rejected. Since the orderer
v1.1 capability can only be enabled by the ordering admins, making it a prerequisite for the
other capabilities prevents application admins from accidentally enabling capabilities before
the orderer is ready to support them.

.. note:: Once a capability has been enabled, disabling it is not recommended or
          supported.

Once a capability has been enabled, it becomes part of the permanent record
for that channel. This means that even after disabling the capability, old binaries will
not be able to participate in the channel because they cannot process beyond the block
which enabled the capability to get to the block which disables it.

For this reason, think of enabling channel capabilities as a point of no return. Please
experiment with the new capabilities in a test setting and be confident before proceeding
to enable them in production.

Note that enabling capability requirements on a channel which a v1.0.0 peer is joined to
will result in a crash of the peer. This crashing behavior is deliberate because it
indicates a misconfiguration which might result in a state fork.

The error message displayed by failing v1.0.x peers will say:

.. code:: bash

  Cannot commit block to the ledger due to Error validating config which passed
  initial validity checks: ConfigEnvelope LastUpdate did not produce the supplied
  config result

We will enable capabilities in the following order:

1. Orderer System Channel

  a. Orderer Group
  b. Channel Group

2. Individual Channels

  a. Orderer Group
  b. Channel Group
  c. Application Group

.. note:: In order to minimize the chance of a fork a best practice is to enable
          the orderer system capability first and then enable individual channel
          capabilities.

For each group, we will enable the capabilities in the following order:

1. Get the latest channel config
2. Create a modified channel config
3. Create a config update transaction

.. note:: This process will be accomplished through a series of config update
          transactions, one for each channel group. In a real world production
          network, these channel config updates would be handled by the admins
          for each channel. Because BYFN all exists on a single machine, it is
          possible for us to update each of these channels.

For more information on updating channel configs, click on :doc:`channel_update_tutorial`
or the doc on :doc:`config_update`.

Get back into the  ``cli`` container by reissuing ``docker exec -it cli bash``.

Now let’s check the set environment variables with:

.. code:: bash

  env|grep PEER

You'll also need to install ``jq``:

.. code:: bash

  apt-get update

  apt-get install -y jq

Orderer System Channel Capabilities
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Let’s set our environment variables for the orderer system channel. Issue each of
these commands:

.. code:: bash

  CORE_PEER_LOCALMSPID="OrdererMSP"

  CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/users/Admin@example.com/msp

  ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

And let’s set our channel name to ``testchainid``:

.. code:: bash

  CH_NAME=testchainid

Orderer Group
^^^^^^^^^^^^^

The first step in updating a channel configuration is getting the latest config
block:

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CH_NAME  --tls --cafile $ORDERER_CA

.. note:: We require configtxlator v1.0.0 or higher for this next step.

To make our config easy to edit, let’s convert the config block to JSON using
configtxlator:

.. code:: bash

  configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json

This command uses ``jq`` to remove the headers, metadata, and signatures
from the config:

.. code:: bash

  jq .data.data[0].payload.data.config config_block.json > config.json

Next, add capabilities to the orderer group. The following command will create a
copy of the config file and add our new capabilities to it:

.. code:: bash

  jq -s '.[0] * {"channel_group":{"groups":{"Orderer": {"values": {"Capabilities": .[1]}}}}}' config.json ./scripts/capabilities.json > modified_config.json

Note what we’re changing here: ``Capabilities`` are being added as a ``value``
of the ``orderer`` group under ``channel_group``. The specific channel we’re
working in is not noted in this command, but recall that it’s the orderer system
channel ``testchainid``. It should be updated first because it is **this**
channel’s configuration that will be copied by default during the creation of
any new channel.

Now we can create the config update:

.. code:: bash

  configtxlator proto_encode --input config.json --type common.Config --output config.pb

  configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

  configtxlator compute_update --channel_id $CH_NAME --original config.pb --updated modified_config.pb --output config_update.pb

Package the config update into a transaction:

.. code:: bash

  configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json

  echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CH_NAME'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json

  configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb

Submit the config update transaction:

.. note:: The command below both signs and submits the transaction to the ordering
          service.

.. code:: bash

  peer channel update -f config_update_in_envelope.pb -c $CH_NAME -o orderer.example.com:7050 --tls true --cafile $ORDERER_CA

Our config update transaction represents the difference between the original
config and the modified one, but the orderer will translate this into a full
channel config.

Channel Group
^^^^^^^^^^^^^

Now let’s move on to enabling capabilities for the channel group at the orderer
system level.

The first step, as before, is to get the latest channel configuration.

.. note:: This set of commands is exactly the same as the steps from the orderer
          group.

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CH_NAME --tls --cafile $ORDERER_CA

  configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json

  jq .data.data[0].payload.data.config config_block.json > config.json

Next, create a modified channel config:

.. code:: bash

  jq -s '.[0] * {"channel_group":{"values": {"Capabilities": .[1]}}}' config.json ./scripts/capabilities.json > modified_config.json

Note what we’re changing here: ``Capabilities`` are being added as a ``value`` of
the top level ``channel_group`` (in the ``testchainid`` channel, as before).

Create the config update transaction:

.. note:: This set of commands is exactly the same as the third step from the
          orderer group.

.. code:: bash

  configtxlator proto_encode --input config.json --type common.Config --output config.pb

  configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

  configtxlator compute_update --channel_id $CH_NAME --original config.pb --updated modified_config.pb --output config_update.pb

Package the config update into a transaction:

.. code:: bash

  configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json

  echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CH_NAME'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json

  configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb

Submit the config update transaction:

.. code:: bash

  peer channel update -f config_update_in_envelope.pb -c $CH_NAME -o orderer.example.com:7050 --tls true --cafile $ORDERER_CA

Enabling Capabilities on Existing Channels
------------------------------------------

Set the channel name to ``mychannel``:

.. code:: bash

  CH_NAME=mychannel

Orderer Group
~~~~~~~~~~~~~

Get the channel config:

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CH_NAME  --tls --cafile $ORDERER_CA

  configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json

  jq .data.data[0].payload.data.config config_block.json > config.json

Let’s add capabilities to the orderer group. The following command will create a
copy of the config file and add our new capabilities to it:

.. code:: bash

  jq -s '.[0] * {"channel_group":{"groups":{"Orderer": {"values": {"Capabilities": .[1]}}}}}' config.json ./scripts/capabilities.json > modified_config.json

Note what we’re changing here: ``Capabilities`` are being added as a ``value``
of the ``orderer`` group under ``channel_group``. This is exactly what we changed
before, only now we’re working with the config to the channel ``mychannel``
instead of ``testchainid``.

Create the config update:

.. code:: bash

  configtxlator proto_encode --input config.json --type common.Config --output config.pb

  configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

  configtxlator compute_update --channel_id $CH_NAME --original config.pb --updated modified_config.pb --output config_update.pb

Package the config update into a transaction:

.. code:: bash

  configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json

  echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CH_NAME'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json

  configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb

Submit the config update transaction:

.. code:: bash

  peer channel update -f config_update_in_envelope.pb -c $CH_NAME -o orderer.example.com:7050 --tls true --cafile $ORDERER_CA

Channel Group
~~~~~~~~~~~~~

.. note:: While this may seem repetitive, remember that we're performing the same
          process on different groups. In a production network, as we've said,
          this process would likely be split up among the various channel admins.

Fetch, decode, and scope the config:

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CH_NAME --tls --cafile $ORDERER_CA

  configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json

  jq .data.data[0].payload.data.config config_block.json > config.json

Create a modified config:

.. code:: bash

  jq -s '.[0] * {"channel_group":{"values": {"Capabilities": .[1]}}}' config.json ./scripts/capabilities.json > modified_config.json

Note what we’re changing here: ``Capabilities`` are being added as a ``value``
of the top level ``channel_group`` (in ``mychannel``, as before).

Create the config update:

.. code:: bash

  configtxlator proto_encode --input config.json --type common.Config --output config.pb

  configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

  configtxlator compute_update --channel_id $CH_NAME --original config.pb --updated modified_config.pb --output config_update.pb

Package the config update into a transaction:

.. code:: bash

  configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json

  echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CH_NAME'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json

  configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb

Because we're updating the config of the ``channel`` group, the relevant orgs --
Org1, Org2, and the OrdererOrg -- need to sign it. This task would usually
be performed by the individual org admins, but in BYFN, as we've said, this task
falls to us.

First, switch into Org1 and sign the update:

.. code:: bash

  CORE_PEER_LOCALMSPID="Org1MSP"

  CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp

  CORE_PEER_ADDRESS=peer0.org1.example.com:7051

  peer channel signconfigtx -f config_update_in_envelope.pb

And do the same as Org2:

.. code:: bash

  CORE_PEER_LOCALMSPID="Org2MSP"

  CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp

  CORE_PEER_ADDRESS=peer0.org1.example.com:7051

  peer channel signconfigtx -f config_update_in_envelope.pb

And as the OrdererOrg:

.. code:: bash

  CORE_PEER_LOCALMSPID="OrdererMSP"

  CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/users/Admin@example.com/msp

  peer channel update -f config_update_in_envelope.pb -c $CH_NAME -o orderer.example.com:7050 --tls true --cafile $ORDERER_CA

Application Group
~~~~~~~~~~~~~~~~~

For the application group, we will need to reset the environment variables as
one organization:

.. code:: bash

  CORE_PEER_LOCALMSPID="Org1MSP"

  CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp

  CORE_PEER_ADDRESS=peer0.org1.example.com:7051

Now, get the latest channel config (this process should be very familiar by now):

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CH_NAME --tls --cafile $ORDERER_CA

  configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json

  jq .data.data[0].payload.data.config config_block.json > config.json

Create a modified channel config:

.. code:: bash

  jq -s '.[0] * {"channel_group":{"groups":{"Application": {"values": {"Capabilities": .[1]}}}}}' config.json ./scripts/capabilities.json > modified_config.json

Note what we’re changing here: ``Capabilities`` are being added as a ``value``
of the ``Application`` group under ``channel_group`` (in ``mychannel``).

Create a config update transaction:

.. code:: bash

  configtxlator proto_encode --input config.json --type common.Config --output config.pb

  configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

  configtxlator compute_update --channel_id $CH_NAME --original config.pb --updated modified_config.pb --output config_update.pb

Package the config update into a transaction:

.. code:: bash

  configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json

  echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CH_NAME'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json

  configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb

Org1 signs the transaction:

.. code:: bash

  peer channel signconfigtx -f config_update_in_envelope.pb

Set the environment variables as Org2:

.. code:: bash

  export CORE_PEER_LOCALMSPID="Org2MSP"

  export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt

  export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp

  export CORE_PEER_ADDRESS=peer0.org2.example.com:7051

Org2 submits the config update transaction with its signature:

.. code:: bash

  peer channel update -f config_update_in_envelope.pb -c $CH_NAME -o orderer.example.com:7050 --tls true --cafile $ORDERER_CA

Congratulations! You have now enabled capabilities on all of your channels.

Verify that Capabilities are Enabled
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

But let's test just to make sure by moving ``10`` from ``a`` to ``b``, as before:

.. code:: bash

  peer chaincode invoke -o orderer.example.com:7050  --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem  -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

And then querying the value of ``a``, which should reveal a value of ``70``.
Let’s see:

.. code:: bash

  peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

We should see the following:

.. code:: bash

  Query Result: 70

In which case we have successfully added capabilities to all of our channels.

.. note:: Although all peer binaries in the network should have been upgraded
          prior to this point, enabling capability requirements on a channel
          which a v1.0.0 peer is joined to will result in a crash of the peer.
          This crashing behavior is deliberate because it indicates a
          misconfiguration which might result in a state fork.

Upgrading Components BYFN Does Not Support
------------------------------------------

Although this is the end of our update tutorial, there are other components that
exist in production networks that are not supported by the BYFN sample. In this
section, we’ll talk through the process of updating them.

Fabric CA Container
~~~~~~~~~~~~~~~~~~~

To learn how to upgrade your Fabric CA server, click over to the `CA documentation. <http://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html#upgrading-the-server>`_

Upgrade Node SDK Clients
~~~~~~~~~~~~~~~~~~~~~~~~

.. note:: Upgrade Fabric CA before upgrading Node SDK Clients.

Use NPM to upgrade any ``Node.js`` client by executing these commands in the
root directory of your application:

..  code:: bash

  npm install fabric-client@1.1

  npm install fabric-ca-client@1.1

These commands install the new version of both the Fabric client and Fabric-CA
client and write the new versions ``package.json``.

Upgrading the Kafka Cluster
~~~~~~~~~~~~~~~~~~~~~~~~~~~

It is not required, but it is recommended that the Kafka cluster be upgraded and
kept up to date along with the rest of Fabric. Newer versions of Kafka support
older protocol versions, so you may upgrade Kafka before or after the rest of
Fabric.

If your Kafka cluster is older than Kafka v0.11.0, this upgrade is especially
recommended as it hardens replication in order to better handle crash faults.

Refer to the official Apache Kafka documentation on `upgrading Kafka from previous
versions`__ to upgrade the Kafka cluster brokers.

.. __: https://kafka.apache.org/documentation/#upgrade

Please note that the Kafka cluster might experience a negative performance impact
if the orderer is configured to use a Kafka protocol version that is older than
the Kafka broker version. The Kafka protocol version is set using either the
``Kafka.Version`` key in the ``orderer.yaml`` file or via the ``ORDERER_KAFKA_VERSION``
environment variable in a Docker deployment. Fabric v1.0 provided sample Kafka
docker images containing Kafka version 0.9.0.1. Fabric v1.1 provides
sample Kafka docker images containing Kafka version v1.0.0.

.. note:: You must configure the Kafka protocol version used by the orderer to
          match your Kafka cluster version, even if it was not set before. For
          example, if you are using the sample Kafka images provided with
          Fabric v1.0.x, either set the ``ORDERER_KAFKA_VERSION`` environment
          variable, or the ``Kafka.Version`` key in the ``orderer.yaml`` to
          ``0.9.0.1``. If you are unsure about your Kafka cluster version, you
          can configure the orderer's Kafka protocol version to ``0.9.0.1`` for
          maximum compatibility and update the setting afterwards when you have
          determined your Kafka cluster version.

Upgrading Zookeeper
^^^^^^^^^^^^^^^^^^^
An Apache Kafka cluster requires an Apache Zookeeper cluster. The Zookeeper API
has been stable for a long time and, as such, almost any version of Zookeeper is
tolerated by Kafka. Refer to the `Apache Kafka upgrade`_ documentation in case
there is a specific requirement to upgrade to a specific version of Zookeeper.
If you would like to upgrade your Zookeeper cluster, some information on
upgrading Zookeeper cluster can be found in the `Zookeeper FAQ`_.

.. _Apache Kafka upgrade: https://kafka.apache.org/documentation/#upgrade
.. _Zookeeper FAQ: https://cwiki.apache.org/confluence/display/ZOOKEEPER/FAQ

Upgrading CouchDB
~~~~~~~~~~~~~~~~~

If you are using CouchDB as state database, upgrade the peer's CouchDB at the same
time the peer is being upgraded. To upgrade CouchDB:

1. Stop CouchDB.
2. Backup CouchDB data directory.
3. Delete CouchDB data directory.
4. Install CouchDB v2.1.1 binaries or update deployment scripts to use a new Docker image
   (CouchDB v2.1.1 pre-configured Docker image is provided alongside Fabric v1.1).
5. Restart CouchDB.

The reason to delete the CouchDB data directory is that upon startup the v1.1 peer
will rebuild the CouchDB state databases from the blockchain transactions. Starting
in v1.1, there will be an internal CouchDB database for each ``channel_chaincode``
combination (for each chaincode instantiated on each channel that the peer has joined).

Upgrade Chaincodes With Vendored Shim
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

A number of third party tools exist that will allow you to vendor a chaincode
shim. If you used one of these tools, use the same one to update your vendoring
and re-package your chaincode.

If your chaincode vendors the shim, after updating the shim version, you must install
it to all peers which already have the chaincode. Install it with the same name, but
a newer version. Then you should execute a chaincode upgrade on each channel where
this chaincode has been deployed to move to the new version.

If you did not vendor your chaincode, you can skip this step entirely.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
