Adding an Org to a Channel
==========================

.. note:: Ensure that you have downloaded the appropriate images and binaries
          as outlined in :doc:`install` and :doc:`prereqs` that conform to the
          version of this documentation (which can be found at the bottom of the
          table of contents to the left).

This tutorial extends the Fabric test network by adding a new organization
-- Org3 -- to an application channel.

While we will focus on adding a new organization to the channel, you can use a
similar process to make other channel configuration updates (updating modification
policies or altering batch size, for example). To learn more about the process
and possibilities of channel config updates in general, check out :doc:`config_update`).
It's also worth noting that channel configuration updates like the one
demonstrated here will usually be the responsibility of an organization admin
(rather than a chaincode or application developer).

Setup the Environment
~~~~~~~~~~~~~~~~~~~~~

We will be operating from the root of the ``test-network`` subdirectory within
your local clone of ``fabric-samples``. Change into that directory now.

.. code:: bash

   cd fabric-samples/test-network

First, use the ``network.sh`` script to tidy up. This command will kill any active
or stale Docker containers and remove previously generated artifacts. It is by no
means **necessary** to bring down a Fabric network in order to perform channel
configuration update tasks. However, for the sake of this tutorial, we want to operate
from a known initial state. Therefore let's run the following command to clean up any
previous environments:

.. code:: bash

  ./network.sh down

You can now use the script to bring up the test network with one channel named
``mychannel``:

.. code:: bash

  ./network.sh up createChannel

If the command was successful, you can see the following message printed in your
logs:

.. code:: bash

  ========= Channel successfully joined ===========


Now that you have a clean version of the test network running on your machine, we
can start the process of adding a new org to the channel we created. First, we are
going use a script to add Org3 to the channel to confirm that the process works.
Then, we will go through the step by step process of adding Org3 by updating the
channel configuration.

Bring Org3 into the Channel with the Script
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

You should be in the ``test-network`` directory. To use the script, simply issue
the following commands:

.. code:: bash

  cd addOrg3
  ./addOrg3.sh up

The output here is well worth reading. You'll see the Org3 crypto material being
generated, the Org3 organization definition being created, and then the channel
configuration being updated, signed, and then submitted to the channel.

If everything goes well, you'll get this message:

.. code:: bash

  ========= Finished adding Org3 to your test network! =========

Now that we have confirmed we can add Org3 to our channel, we can go through the
steps to update the channel configuration that the script completed behind the
scenes.

Bring Org3 into the Channel Manually
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you just used the ``addOrg3.sh`` script, you'll need to bring your network down.
The following command will bring down all running components and remove the crypto
material for all organizations:

.. code:: bash

  ./addOrg3.sh down

After the network is brought down, bring it back up again:

.. code:: bash

  cd ..
  ./network.sh up createChannel

This will bring your network back to the same state it was in before you executed
the ``addOrg3.sh`` script.

Now we're ready to add Org3 to the channel manually. As a first step, we'll need
to generate Org3's crypto material.

Generate the Org3 Crypto Material
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

In another terminal, change into the ``addOrg3`` subdirectory from
``test-network``.

.. code:: bash

  cd addOrg3

First, we are going to create the certificates and keys for the Org3 peer, along
with an application and admin user. Because we are updating an example channel,
we are going to use the cryptogen tool instead of using a Certificate Authority.
The following command uses cryptogen  to read the ``org3-crypto.yaml`` file
and generate the Org3 crypto material in a new ``org3.example.com`` folder:

.. code:: bash

  ../../bin/cryptogen generate --config=org3-crypto.yaml --output="../organizations"

You can find the generated Org3 crypto material alongside the certificates and
keys for Org1 and Org2 in the ``test-network/organizations/peerOrganizations``
directory.

Once we have created the Org3 crypto material, we can use the configtxgen
tool to print out the Org3 organization definition. We will preface the command
by telling the tool to look in the current directory for the ``configtx.yaml``
file that it needs to ingest.

.. code:: bash

    export FABRIC_CFG_PATH=$PWD
    ../../bin/configtxgen -printOrg Org3MSP > ../organizations/peerOrganizations/org3.example.com/org3.json

The above command creates a JSON file -- ``org3.json`` -- and writes it to the
``test-network/organizations/peerOrganizations/org3.example.com`` folder. The
organization definition contains the policy definitions for Org3, as well as three
important certificates encoded in base64 format:

  * a CA root cert, used to establish the organizations root of trust
  * a TLS root cert, used by the gossip protocol to identify Org3 for block dissemination and service discovery
  * The admin user certificate (which will be needed to act as the admin of Org3 later on)

We will add Org3 to the channel by appending this organization definition to
the channel configuration.

Bring up Org3 components
~~~~~~~~~~~~~~~~~~~~~~~~

After we have created the Org3 certificate material, we can now bring up the
Org3 peer. From the ``addOrg3`` directory, issue the following command:

.. code:: bash

  docker-compose -f docker/docker-compose-org3.yaml up -d

If the command is successful, you will see the creation of the Org3 peer and
an instance of the Fabric tools container named Org3CLI:

.. code:: bash

  Creating peer0.org3.example.com ... done
  Creating Org3cli                ... done

This Docker Compose file has been configured to bridge across our initial network,
so that the Org3 peer and Org3CLI resolve with the existing peers and ordering
node of the test network. We will use the Org3CLI container to communicate with
the network and issue the peer commands that will add Org3 to the channel.


Prepare the CLI Environment
~~~~~~~~~~~~~~~~~~~~~~~~~~~

The update process makes use of the configuration translator tool -- configtxlator.
This tool provides a stateless REST API independent of the SDK. Additionally it
provides a CLI tool that can be used to simplify configuration tasks in Fabric
networks. The tool allows for the easy conversion between different equivalent
data representations/formats (in this case, between protobufs and JSON).
Additionally, the tool can compute a configuration update transaction based on
the differences between two channel configurations.

Use the following command to exec into the Org3CLI container:

.. code:: bash

  docker exec -it Org3cli bash

This container has been mounted with the ``organizations`` folder, giving us
access to the crypto material and TLS certificates for all organizations and the
Orderer Org. We can use environment variables to operate the Org3CLI container
as the admin of Org1, Org2, or Org3. First, we need to set the environment
variables for the orderer TLS certificate and the channel name:

.. code:: bash

  export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
  export CHANNEL_NAME=mychannel

Check to make sure the variables have been properly set:

.. code:: bash

  echo $ORDERER_CA && echo $CHANNEL_NAME

.. note:: If for any reason you need to restart the Org3CLI container, you will also need to
          re-export the two environment variables -- ``ORDERER_CA`` and ``CHANNEL_NAME``.

Fetch the Configuration
~~~~~~~~~~~~~~~~~~~~~~~

Now we have the Org3CLI container with our two key environment variables -- ``ORDERER_CA``
and ``CHANNEL_NAME`` exported.  Let's go fetch the most recent config block for the
channel -- ``mychannel``.

The reason why we have to pull the latest version of the config is because channel
config elements are versioned. Versioning is important for several reasons. It prevents
config changes from being repeated or replayed (for instance, reverting to a channel config
with old CRLs would represent a security risk). Also it helps ensure concurrency (if you
want to remove an Org from your channel, for example, after a new Org has been added,
versioning will help prevent you from removing both Orgs, instead of just the Org you want
to remove).

Because Org3 is not yet a member of the channel, we need to operate as the admin
of another organization to fetch the channel config. Because Org1 is a member of the channel, the
Org1 admin has permission to fetch the channel config from the ordering service.
Issue the following commands to operate as the Org1 admin.

.. code:: bash

  # you can issue all of these commands at once

  export CORE_PEER_LOCALMSPID="Org1MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
  export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
  export CORE_PEER_ADDRESS=peer0.org1.example.com:7051

We can now issue the command to fetch the latest config block:

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CHANNEL_NAME --tls --cafile $ORDERER_CA

This command saves the binary protobuf channel configuration block to
``config_block.pb``. Note that the choice of name and file extension is arbitrary.
However, following a convention which identifies both the type of object being
represented and its encoding (protobuf or JSON) is recommended.

When you issued the ``peer channel fetch`` command, the following output is
displayed in your logs:

.. code:: bash

  2017-11-07 17:17:57.383 UTC [channelCmd] readBlock -> DEBU 011 Received block: 2

This is telling us that the most recent configuration block for ``mychannel`` is
actually block 2, **NOT** the genesis block. By default, the ``peer channel fetch config``
command returns the most **recent** configuration block for the targeted channel, which
in this case is the third block. This is because the test network script, ``network.sh``, defined anchor
peers for our two organizations -- ``Org1`` and ``Org2`` -- in two separate channel update
transactions. As a result, we have the following configuration sequence:

  * block 0: genesis block
  * block 1: Org1 anchor peer update
  * block 2: Org2 anchor peer update

Convert the Configuration to JSON and Trim It Down
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Now we will make use of the ``configtxlator`` tool to decode this channel
configuration block into JSON format (which can be read and modified by humans).
We also must strip away all of the headers, metadata, creator signatures, and
so on that are irrelevant to the change we want to make. We accomplish this by
means of the ``jq`` tool:

.. code:: bash

  configtxlator proto_decode --input config_block.pb --type common.Block | jq .data.data[0].payload.data.config > config.json

This command leaves us with a trimmed down JSON object -- ``config.json`` -- which
will serve as the baseline for our config update.

Take a moment to open this file inside your text editor of choice (or in your
browser). Even after you're done with this tutorial, it will be worth studying it
as it reveals the underlying configuration structure and the other kind of channel
updates that can be made. We discuss them in more detail in :doc:`config_update`.

Add the Org3 Crypto Material
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. note:: The steps you've taken up to this point will be nearly identical no matter
          what kind of config update you're trying to make. We've chosen to add an
          org with this tutorial because it's one of the most complex channel
          configuration updates you can attempt.

We'll use the ``jq`` tool once more to append the Org3 configuration definition
-- ``org3.json`` -- to the channel's application groups field, and name the output
-- ``modified_config.json``.

.. code:: bash

  jq -s '.[0] * {"channel_group":{"groups":{"Application":{"groups": {"Org3MSP":.[1]}}}}}' config.json ./organizations/peerOrganizations/org3.example.com/org3.json > modified_config.json

Now, within the Org3CLI container we have two JSON files of interest -- ``config.json``
and ``modified_config.json``. The initial file contains only Org1 and Org2 material,
whereas the "modified" file contains all three Orgs. At this point it's simply
a matter of re-encoding these two JSON files and calculating the delta.

First, translate ``config.json`` back into a protobuf called ``config.pb``:

.. code:: bash

  configtxlator proto_encode --input config.json --type common.Config --output config.pb

Next, encode ``modified_config.json`` to ``modified_config.pb``:

.. code:: bash

  configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb

Now use ``configtxlator`` to calculate the delta between these two config
protobufs. This command will output a new protobuf binary named ``org3_update.pb``:

.. code:: bash

  configtxlator compute_update --channel_id $CHANNEL_NAME --original config.pb --updated modified_config.pb --output org3_update.pb

This new proto -- ``org3_update.pb`` -- contains the Org3 definitions and high
level pointers to the Org1 and Org2 material. We are able to forgo the extensive
MSP material and modification policy information for Org1 and Org2 because this
data is already present within the channel's genesis block. As such, we only need
the delta between the two configurations.

Before submitting the channel update, we need to perform a few final steps. First,
let's decode this object into editable JSON format and call it ``org3_update.json``:

.. code:: bash

  configtxlator proto_decode --input org3_update.pb --type common.ConfigUpdate | jq . > org3_update.json

Now, we have a decoded update file -- ``org3_update.json`` -- that we need to wrap
in an envelope message. This step will give us back the header field that we stripped away
earlier. We'll name this file ``org3_update_in_envelope.json``:

.. code:: bash

  echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CHANNEL_NAME'", "type":2}},"data":{"config_update":'$(cat org3_update.json)'}}}' | jq . > org3_update_in_envelope.json

Using our properly formed JSON -- ``org3_update_in_envelope.json`` -- we will
leverage the ``configtxlator`` tool one last time and convert it into the
fully fledged protobuf format that Fabric requires. We'll name our final update
object ``org3_update_in_envelope.pb``:

.. code:: bash

  configtxlator proto_encode --input org3_update_in_envelope.json --type common.Envelope --output org3_update_in_envelope.pb

Sign and Submit the Config Update
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Almost done!

We now have a protobuf binary -- ``org3_update_in_envelope.pb`` -- within the
Org3CLI container. However, we need signatures from the requisite Admin users
before the config can be written to the ledger. The modification policy (mod_policy)
for our channel Application group is set to the default of "MAJORITY", which means that
we need a majority of existing org admins to sign it. Because we have only two orgs --
Org1 and Org2 -- and the majority of two is two, we need both of them to sign. Without
both signatures, the ordering service will reject the transaction for failing to
fulfill the policy.

First, let's sign this update proto as Org1. Remember that we exported the
necessary environment variables to operate the Org3CLI container as the Org1 admin.
As a result, the following ``peer channel signconfigtx`` command will sign the update as Org1.

.. code:: bash

  peer channel signconfigtx -f org3_update_in_envelope.pb

The final step is to switch the container's identity to reflect the Org2 Admin
user. We do this by exporting four environment variables specific to the Org2 MSP.

.. note:: Switching between organizations to sign a config transaction (or to do anything
          else) is not reflective of a real-world Fabric operation. A single container
          would never be mounted with an entire network's crypto material. Rather, the
          config update would need to be securely passed out-of-band to an Org2
          Admin for inspection and approval.

Export the Org2 environment variables:

.. code:: bash

  # you can issue all of these commands at once

  export CORE_PEER_LOCALMSPID="Org2MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
  export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
  export CORE_PEER_ADDRESS=peer0.org2.example.com:9051

Lastly, we will issue the ``peer channel update`` command. The Org2 Admin signature
will be attached to this call so there is no need to manually sign the protobuf a
second time:

.. note:: The upcoming update call to the ordering service will undergo a series
          of systematic signature and policy checks. As such you may find it
          useful to stream and inspect the ordering node's logs. You can issue a
          ``docker logs -f orderer.example.com`` command from a terminal outside
          the Org3CLI container to display them.

Send the update call:

.. code:: bash

  peer channel update -f org3_update_in_envelope.pb -c $CHANNEL_NAME -o orderer.example.com:7050 --tls --cafile $ORDERER_CA

You should see a message similar to the following if your update has been submitted successfully:

.. code:: bash

  2020-01-09 21:30:45.791 UTC [channelCmd] update -> INFO 002 Successfully submitted channel update

The successful channel update call returns a new block -- block 3 -- to all of the
peers on the channel. If you remember, blocks 0-2 are the initial channel
configurations. Block 3 serves as the most recent channel configuration with
Org3 now defined on the channel.

You can inspect the logs for ``peer0.org1.example.com`` by navigating to a terminal
outside the Org3CLI container and issuing the following command:

.. code:: bash

      docker logs -f peer0.org1.example.com


Join Org3 to the Channel
~~~~~~~~~~~~~~~~~~~~~~~~

At this point, the channel configuration has been updated to include our new
organization -- Org3 -- meaning that peers attached to it can now join ``mychannel``.

Inside the Org3CLI container, export the following environment variables to operate
as the Org3 Admin:

.. code:: bash

  # you can issue all of these commands at once

  export CORE_PEER_LOCALMSPID="Org3MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt
  export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp
  export CORE_PEER_ADDRESS=peer0.org3.example.com:11051

Now let's send a call to the ordering service asking for the genesis block of
``mychannel``. As a result of the successful channel update, the ordering service
will verify that Org3 can pull the genesis block and join the channel. If Org3 had not
been successfully appended to the channel config, the ordering service would
reject this request.

.. note:: Again, you may find it useful to stream the ordering node's logs
          to reveal the sign/verify logic and policy checks.

Use the ``peer channel fetch`` command to retrieve this block:

.. code:: bash

  peer channel fetch 0 mychannel.block -o orderer.example.com:7050 -c $CHANNEL_NAME --tls --cafile $ORDERER_CA

Notice, that we are passing a ``0`` to indicate that we want the first block on
the channel's ledger; the genesis block. If we simply passed the
``peer channel fetch config`` command, then we would have received block 3 -- the
updated config with Org3 defined. However, we can't begin our ledger with a
downstream block -- we must start with block 0.

If successful, the command returned the genesis block to a file named ``mychannel.block``.
We can now use this block to join the peer to the channel. Issue the
``peer channel join`` command and pass in the genesis block to join the Org3
peer to the channel:

.. code:: bash

  peer channel join -b mychannel.block


Configuring Leader Election
~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. note:: This section is included as a general reference for understanding
          the leader election settings when adding organizations to a network
          after the initial channel configuration has completed. This sample
          defaults to dynamic leader election, which is set for all peers in the
          network.

Newly joining peers are bootstrapped with the genesis block, which does not
contain information about the organization that is being added in the channel
configuration update. Therefore new peers are not able to utilize gossip as
they cannot verify blocks forwarded by other peers from their own organization
until they get the configuration transaction which added the organization to the
channel. Newly added peers must therefore have one of the following
configurations so that they receive blocks from the ordering service:

1. To utilize static leader mode, configure the peer to be an organization
leader:

::

    CORE_PEER_GOSSIP_USELEADERELECTION=false
    CORE_PEER_GOSSIP_ORGLEADER=true


.. note:: This configuration must be the same for all new peers added to the
          channel.

2. To utilize dynamic leader election, configure the peer to use leader
election:

::

    CORE_PEER_GOSSIP_USELEADERELECTION=true
    CORE_PEER_GOSSIP_ORGLEADER=false


.. note:: Because peers of the newly added organization won't be able to form
          membership view, this option will be similar to the static
          configuration, as each peer will start proclaiming itself to be a
          leader. However, once they get updated with the configuration
          transaction that adds the organization to the channel, there will be
          only one active leader for the organization. Therefore, it is
          recommended to leverage this option if you eventually want the
          organization's peers to utilize leader election.


.. _upgrade-and-invoke:

Install, define, and invoke chaincode
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

We can confirm that Org3 is a member of ``mychannel`` by installing and invoking
a chaincode on the channel. If the existing channel members have already committed
a chaincode definition to the channel, a new organization can start using the
chaincode by approving the chaincode definition.

.. note:: These instructions use the Fabric chaincode lifecycle introduced in
          the v2.0 release. If you would like to use the previous lifecycle to
          install and instantiate a chaincode, visit the v1.4 version of the
          `Adding an org to a channel tutorial <https://hyperledger-fabric.readthedocs.io/en/release-1.4/channel_update_tutorial.html>`__.

Before we install a chaincode as Org3, we can use the ``./network.sh`` script to
deploy the Fabcar chaincode on the channel. Open a new terminal outside the
Org3CLI container and navigate to the ``test-network`` directory. You can then use
use the ``test-network`` script to deploy the Fabcar chaincode:

.. code:: bash

  cd fabric-samples/test-network
  ./network.sh deployCC

The script will install the Fabcar chaincode on the Org1 and Org2 peers, approve
the chaincode definition for Org1 and Org2, and then commit the chaincode
definition to the channel. Once the chaincode definition has been committed to
the channel, the Fabcar chaincode is initialized and invoked to put initial data
on the ledger. The commands below assume that we are still using the channel
``mychannel``.

After the chaincode has been to deployed we can use the following steps to use
invoke Fabcar chaincode as Org3. These steps can be completed from the
``test-network`` directory, without having to exec into Org3CLI container. Copy
and paste the following environment variables in your terminal in order to interact
with the network as the Org3 admin:

.. code:: bash

    export PATH=${PWD}/../bin:$PATH
    export FABRIC_CFG_PATH=$PWD/../config/
    export CORE_PEER_TLS_ENABLED=true
    export CORE_PEER_LOCALMSPID="Org3MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp
    export CORE_PEER_ADDRESS=localhost:11051

The first step is to package the Fabcar chaincode:

.. code:: bash

    peer lifecycle chaincode package fabcar.tar.gz --path ../chaincode/fabcar/go/ --lang golang --label fabcar_1

This command will create a chaincode package named ``fabcar.tar.gz``, which we can
install on the Org3 peer. Modify the command accordingly if the channel is running a
chaincode written in Java or Node.js. Issue the following command to install the
chaincode package ``peer0.org3.example.com``:

.. code:: bash

    peer lifecycle chaincode install fabcar.tar.gz


The next step is to approve the chaincode definition of Fabcar as Org3. Org3
needs to approve the same definition that Org1 and Org2 approved and committed
to the channel. In order to invoke the chaincode, Org3 needs to include the
package identifier in the chaincode definition. You can find the package
identifier by querying your peer:

.. code:: bash

    peer lifecycle chaincode queryinstalled

You should see output similar to the following:

.. code:: bash

      Get installed chaincodes on peer:
      Package ID: fabcar_1:25f28c212da84a8eca44d14cf12549d8f7b674a0d8288245561246fa90f7ab03, Label: fabcar_1

We are going to need the package ID in a future command, so lets go ahead and
save it as an environment variable. Paste the package ID returned by the
``peer lifecycle chaincode queryinstalled`` command into the command below. The
package ID may not be the same for all users, so you need to complete this step
using the package ID returned from your console.

.. code:: bash

   export CC_PACKAGE_ID=fabcar_1:25f28c212da84a8eca44d14cf12549d8f7b674a0d8288245561246fa90f7ab03

Use the following command to approve a definition of the Fabcar chaincode
for Org3:

.. code:: bash

    # use the --package-id flag to provide the package identifier
    # use the --init-required flag to request the ``Init`` function be invoked to initialize the chaincode
    peer lifecycle chaincode approveformyorg -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name fabcar --version 1 --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem


You can use the ``peer lifecycle chaincode querycommitted`` command to check if
the chaincode definition you have approved has already been committed to the
channel.

.. code:: bash

    # use the --name flag to select the chaincode whose definition you want to query
    peer lifecycle chaincode querycommitted --channelID mychannel --name fabcar --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

A successful command will return information about the committed definition:

.. code:: bash

    Committed chaincode definition for chaincode 'fabcar' on channel 'mychannel':
    Version: 1, Sequence: 1, Endorsement Plugin: escc, Validation Plugin: vscc, Approvals: [Org1MSP: true, Org2MSP: true, Org3MSP: true]

Org3 can use the Fabcar chaincode after it approves the chaincode definition
that was committed to the channel. The chaincode definition uses the default endorsement
policy, which requires a majority of organizations on the channel endorse a transaction.
This implies that if an organization is added to or removed from the channel, the
endorsement policy will be updated automatically. We previously needed endorsements
from Org1 and Org2 (2 out of 2). Now we need endorsements from two organizations
out of Org1, Org2, and Org3 (2 out of 3).

You can query the chaincode to ensure that it has started on the Org3 peer. Note
that you may need to wait for the chaincode container to start.

.. code:: bash

    peer chaincode query -C mychannel -n fabcar -c '{"Args":["queryAllCars"]}'

You should see the initial list of cars that were added to the ledger as a
response.

Now, invoke the chaincode to add a new car to the ledger. In the command below,
we target a peer in Org1 and Org3 to collect a sufficient number of endorsements.

.. code:: bash

    peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n fabcar --peerAddresses localhost:7051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses localhost:11051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt -c '{"function":"createCar","Args":["CAR11","Honda","Accord","Black","Tom"]}'

We can query again to see the new car, "CAR11" on the our the ledger:

.. code:: bash

    peer chaincode query -C mychannel -n fabcar -c '{"Args":["queryCar","CAR11"]}'


Conclusion
~~~~~~~~~~

The channel configuration update process is indeed quite involved, but there is a
logical method to the various steps. The endgame is to form a delta transaction object
represented in protobuf binary format and then acquire the requisite number of admin
signatures such that the channel configuration update transaction fulfills the channel's
modification policy.

The ``configtxlator`` and ``jq`` tools, along with the ``peer channel``
commands, provide us with the functionality to accomplish this task.

Updating the Channel Config to include an Org3 Anchor Peer (Optional)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The Org3 peers were able to establish gossip connection to the Org1 and Org2
peers since Org1 and Org2 had anchor peers defined in the channel configuration.
Likewise newly added organizations like Org3 should also define their anchor peers
in the channel configuration so that any new peers from other organizations can
directly discover an Org3 peer. In this section, we will make a channel
configuration update to define an Org3 anchor peer. The process will be similar
to the previous configuration update, therefore we'll go faster this time.

If you don't have it open, exec back into the Org3CLI container:

.. code:: bash

  docker exec -it Org3cli bash

Export the $ORDERER_CA and $CHANNEL_NAME variables if they are not already set:

.. code:: bash

  export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
  export CHANNEL_NAME=mychannel

As before, we will fetch the latest channel configuration to get started.
Inside the Org3CLI container, fetch the most recent config block for the channel,
using the ``peer channel fetch`` command.

.. code:: bash

  peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CHANNEL_NAME --tls --cafile $ORDERER_CA

After fetching the config block we will want to convert it into JSON format. To do
this we will use the configtxlator tool, as done previously when adding Org3 to the
channel. When converting it we need to remove all the headers, metadata, and signatures
that are not required to update Org3 to include an anchor peer by using the jq
tool. This information will be reincorporated later before we proceed to update the
channel configuration.

.. code:: bash

    configtxlator proto_decode --input config_block.pb --type common.Block | jq .data.data[0].payload.data.config > config.json

The ``config.json`` is the now trimmed JSON representing the latest channel configuration
that we will update.

Using the jq tool again, we will update the configuration JSON with the Org3 anchor peer we
want to add.

.. code:: bash

    jq '.channel_group.groups.Application.groups.Org3MSP.values += {"AnchorPeers":{"mod_policy": "Admins","value":{"anchor_peers": [{"host": "peer0.org3.example.com","port": 11051}]},"version": "0"}}' config.json > modified_anchor_config.json

We now have two JSON files, one for the current channel configuration,
``config.json``, and one for the desired channel configuration ``modified_anchor_config.json``.
Next we convert each of these back into protobuf format and calculate the delta between the two.

Translate ``config.json`` back into protobuf format as ``config.pb``

.. code:: bash

    configtxlator proto_encode --input config.json --type common.Config --output config.pb

Translate the ``modified_anchor_config.json`` into protobuf format as ``modified_anchor_config.pb``

.. code:: bash

    configtxlator proto_encode --input modified_anchor_config.json --type common.Config --output modified_anchor_config.pb

Calculate the delta between the two protobuf formatted configurations.

.. code:: bash

    configtxlator compute_update --channel_id $CHANNEL_NAME --original config.pb --updated modified_anchor_config.pb --output anchor_update.pb

Now that we have the desired update to the channel we must wrap it in an envelope
message so that it can be properly read. To do this we must first convert the protobuf
back into a JSON that can be wrapped.

We will use the configtxlator command again to convert ``anchor_update.pb`` into ``anchor_update.json``

.. code:: bash

    configtxlator proto_decode --input anchor_update.pb --type common.ConfigUpdate | jq . > anchor_update.json

Next we will wrap the update in an envelope message, restoring the previously
stripped away header, outputting it to ``anchor_update_in_envelope.json``

.. code:: bash

    echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CHANNEL_NAME'", "type":2}},"data":{"config_update":'$(cat anchor_update.json)'}}}' | jq . > anchor_update_in_envelope.json

Now that we have reincorporated the envelope we need to convert it
to a protobuf so it can be properly signed and submitted to the orderer for the update.

.. code:: bash

    configtxlator proto_encode --input anchor_update_in_envelope.json --type common.Envelope --output anchor_update_in_envelope.pb

Now that the update has been properly formatted it is time to sign off and submit it. Since this
is only an update to Org3 we only need to have Org3 sign off on the update. Run the following
commands to make sure that we are operating as the Org3 admin:

.. code:: bash

  # you can issue all of these commands at once

  export CORE_PEER_LOCALMSPID="Org3MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt
  export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp
  export CORE_PEER_ADDRESS=peer0.org3.example.com:11051

We can now just use the ``peer channel update`` command to sign the update as the
Org3 admin before submitting it to the orderer.

.. code:: bash

    peer channel update -f anchor_update_in_envelope.pb -c $CHANNEL_NAME -o orderer.example.com:7050 --tls --cafile $ORDERER_CA

The orderer receives the config update request and cuts a block with the updated configuration.
As peers receive the block, they will process the configuration updates.

Inspect the logs for one of the peers. While processing the configuration transaction from the new block,
you will see gossip re-establish connections using the new anchor peer for Org3. This is proof
that the configuration update has been successfully applied!

.. code:: bash

    docker logs -f peer0.org1.example.com

.. code:: bash

    2019-06-12 17:08:57.924 UTC [gossip.gossip] learnAnchorPeers -> INFO 89a Learning about the configured anchor peers of Org1MSP for channel mychannel : [{peer0.org1.example.com 7051}]
    2019-06-12 17:08:57.926 UTC [gossip.gossip] learnAnchorPeers -> INFO 89b Learning about the configured anchor peers of Org2MSP for channel mychannel : [{peer0.org2.example.com 9051}]
    2019-06-12 17:08:57.926 UTC [gossip.gossip] learnAnchorPeers -> INFO 89c Learning about the configured anchor peers of Org3MSP for channel mychannel : [{peer0.org3.example.com 11051}]

Congratulations, you have now made two configuration updates --- one to add Org3 to the channel,
and a second to define an anchor peer for Org3.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
