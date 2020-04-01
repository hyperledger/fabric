Building Your First Network
===========================

.. note:: The Build your first network (BYFN) tutorial has been deprecated. If
          you are getting started with Hyperledger Fabric and would like to deploy
          a basic network, see :doc:`test_network`. If you are deploying Fabric
          in production, see the guide for :doc:`deployment_guide_overview`.

The build your first network (BYFN) scenario provisions a sample Hyperledger
Fabric network consisting of two organizations, each maintaining two peer
nodes. It also will deploy a Raft ordering service by default.

Install prerequisites
---------------------

Before we begin, if you haven't already done so, you may wish to check that
you have all the :doc:`prereqs` installed on the platform(s) on which you'll be
developing blockchain applications and/or operating Hyperledger Fabric.

You will also need to :doc:`install`. You will notice that there are a number of
samples included in the ``fabric-samples`` repository. We will be using the
``first-network`` sample. Let's open that sub-directory now.

.. code:: bash

  cd fabric-samples/first-network

.. note:: The supplied commands in this documentation **MUST** be run from your
          ``first-network`` sub-directory of the ``fabric-samples`` repository
          clone.  If you elect to run the commands from a different location,
          the various provided scripts will be unable to find the binaries.

Want to run it now?
-------------------

We provide a fully annotated script --- ``byfn.sh`` --- that leverages these Docker
images to quickly bootstrap a Hyperledger Fabric network that by default is
comprised of four peers representing two different organizations, and a Raft ordering
service. It will also launch a container to run a scripted execution that will join
peers to a channel, deploy a chaincode and drive execution of transactions
against the deployed chaincode.

Here's the help text for the ``byfn.sh`` script:

.. code:: bash

  Usage:
  byfn.sh <mode> [-c <channel name>] [-t <timeout>] [-d <delay>] [-f <docker-compose-file>] [-s <dbtype>] [-l <language>] [-o <consensus-type>] [-i <imagetag>] [-v]"
    <mode> - one of 'up', 'down', 'restart', 'generate' or 'upgrade'"
      - 'up' - bring up the network with docker-compose up"
      - 'down' - clear the network with docker-compose down"
      - 'restart' - restart the network"
      - 'generate' - generate required certificates and genesis block"
      - 'upgrade'  - upgrade the network from version 1.3.x to 1.4.0"
    -c <channel name> - channel name to use (defaults to \"mychannel\")"
    -t <timeout> - CLI timeout duration in seconds (defaults to 10)"
    -d <delay> - delay duration in seconds (defaults to 3)"
    -f <docker-compose-file> - specify which docker-compose file use (defaults to docker-compose-cli.yaml)"
    -s <dbtype> - the database backend to use: goleveldb (default) or couchdb"
    -l <language> - the chaincode language: golang (default), javascript, or java"
    -a - launch certificate authorities (no certificate authorities are launched by default)
    -n - do not deploy chaincode (abstore chaincode is deployed by default)
    -i <imagetag> - the tag to be used to launch the network (defaults to \"latest\")"
    -v - verbose mode"
  byfn.sh -h (print this message)"

  Typically, one would first generate the required certificates and
  genesis block, then bring up the network. e.g.:"

    byfn.sh generate -c mychannel"
    byfn.sh up -c mychannel -s couchdb"
    byfn.sh up -c mychannel -s couchdb -i 1.4.0"
    byfn.sh up -l javascript"
    byfn.sh down -c mychannel"
    byfn.sh upgrade -c mychannel"

  Taking all defaults:"
  	byfn.sh generate"
  	byfn.sh up"
  	byfn.sh down"

If you choose not to supply a flag, the script will use default values.

Generate Network Artifacts
^^^^^^^^^^^^^^^^^^^^^^^^^^

Ready to give it a go? Okay then! Execute the following command:

.. code:: bash

  ./byfn.sh generate

You will see a brief description as to what will occur, along with a yes/no command line
prompt. Respond with a ``y`` or hit the return key to execute the described action.

.. code:: bash

  Generating certs and genesis block for channel 'mychannel' with CLI timeout of '10' seconds and CLI delay of '3' seconds
  Continue? [Y/n] y
  proceeding ...
  /Users/xxx/dev/fabric-samples/bin/cryptogen

  ##########################################################
  ##### Generate certificates using cryptogen tool #########
  ##########################################################
  org1.example.com
  2017-06-12 21:01:37.334 EDT [bccsp] GetDefault -> WARN 001 Before using BCCSP, please call InitFactories(). Falling back to bootBCCSP.
  ...

  /Users/xxx/dev/fabric-samples/bin/configtxgen
  ##########################################################
  #########  Generating Orderer Genesis block ##############
  ##########################################################
  2017-06-12 21:01:37.558 EDT [common/configtx/tool] main -> INFO 001 Loading configuration
  2017-06-12 21:01:37.562 EDT [msp] getMspConfig -> INFO 002 intermediate certs folder not found at [/Users/xxx/dev/byfn/crypto-config/ordererOrganizations/example.com/msp/intermediatecerts]. Skipping.: [stat /Users/xxx/dev/byfn/crypto-config/ordererOrganizations/example.com/msp/intermediatecerts: no such file or directory]
  ...
  2017-06-12 21:01:37.588 EDT [common/configtx/tool] doOutputBlock -> INFO 00b Generating genesis block
  2017-06-12 21:01:37.590 EDT [common/configtx/tool] doOutputBlock -> INFO 00c Writing genesis block

  #################################################################
  ### Generating channel configuration transaction 'channel.tx' ###
  #################################################################
  2017-06-12 21:01:37.634 EDT [common/configtx/tool] main -> INFO 001 Loading configuration
  2017-06-12 21:01:37.644 EDT [common/configtx/tool] doOutputChannelCreateTx -> INFO 002 Generating new channel configtx
  2017-06-12 21:01:37.645 EDT [common/configtx/tool] doOutputChannelCreateTx -> INFO 003 Writing new channel tx

  #################################################################
  #######    Generating anchor peer update for Org1MSP   ##########
  #################################################################
  2017-06-12 21:01:37.674 EDT [common/configtx/tool] main -> INFO 001 Loading configuration
  2017-06-12 21:01:37.678 EDT [common/configtx/tool] doOutputAnchorPeersUpdate -> INFO 002 Generating anchor peer update
  2017-06-12 21:01:37.679 EDT [common/configtx/tool] doOutputAnchorPeersUpdate -> INFO 003 Writing anchor peer update

  #################################################################
  #######    Generating anchor peer update for Org2MSP   ##########
  #################################################################
  2017-06-12 21:01:37.700 EDT [common/configtx/tool] main -> INFO 001 Loading configuration
  2017-06-12 21:01:37.704 EDT [common/configtx/tool] doOutputAnchorPeersUpdate -> INFO 002 Generating anchor peer update
  2017-06-12 21:01:37.704 EDT [common/configtx/tool] doOutputAnchorPeersUpdate -> INFO 003 Writing anchor peer update

This first step generates all of the certificates and keys for our various
network entities, the ``genesis block`` used to bootstrap the ordering service,
and a collection of configuration transactions required to configure a
:ref:`Channel`.

Bring Up the Network
^^^^^^^^^^^^^^^^^^^^

Next, you can bring the network up with one of the following commands:

.. code:: bash

  ./byfn.sh up

The above command will compile Go chaincode images and spin up the corresponding
containers. Go is the default chaincode language, however there is also support
for `Node.js <https://hyperledger.github.io/fabric-chaincode-node/>`_ and `Java <https://hyperledger.github.io/fabric-chaincode-java/>`_
chaincode. If you'd like to run through this tutorial with node chaincode, pass
the following command instead:

.. code:: bash

  # we use the -l flag to specify the chaincode language
  # forgoing the -l flag will default to "golang"

  ./byfn.sh up -l javascript

.. note:: For more information on the Node.js shim, please refer to its
          `documentation <https://hyperledger.github.io/fabric-chaincode-node/master/api/fabric-shim.ChaincodeInterface.html>`_.

.. note:: For more information on the Java shim, please refer to its
          `documentation <https://hyperledger.github.io/fabric-chaincode-java/{BRANCH}/api/org/hyperledger/fabric/shim/Chaincode.html>`_.

Тo make the sample run with Java chaincode, you have to specify ``-l java`` as follows:

.. code:: bash

  ./byfn.sh up -l java

.. note:: Do not run both of these commands. Only one language can be tried unless
          you bring down and recreate the network between.

You will be prompted as to whether you wish to continue or abort.
Respond with a ``y`` or hit the return key:

.. code:: bash

  Starting for channel 'mychannel' with CLI timeout of '10' seconds and CLI delay of '3' seconds
  Continue? [Y/n]
  proceeding ...
  Creating network "net_byfn" with the default driver
  Creating peer0.org1.example.com
  Creating peer1.org1.example.com
  Creating peer0.org2.example.com
  Creating orderer.example.com
  Creating peer1.org2.example.com
  Creating cli


   ____    _____      _      ____    _____
  / ___|  |_   _|    / \    |  _ \  |_   _|
  \___ \    | |     / _ \   | |_) |   | |
   ___) |   | |    / ___ \  |  _ <    | |
  |____/    |_|   /_/   \_\ |_| \_\   |_|

  Channel name : mychannel
  Creating channel...

The logs will continue from there. This will launch all of the containers, and
then drive a complete end-to-end application scenario. Upon successful
completion, it should report the following in your terminal window:

.. code:: bash

    Query Result: 90
    2017-05-16 17:08:15.158 UTC [main] main -> INFO 008 Exiting.....
    ===================== Query successful on peer1.org2 on channel 'mychannel' =====================

    ===================== All GOOD, BYFN execution completed =====================


     _____   _   _   ____
    | ____| | \ | | |  _ \
    |  _|   |  \| | | | | |
    | |___  | |\  | | |_| |
    |_____| |_| \_| |____/

You can scroll through these logs to see the various transactions. If you don't
get this result, then jump down to the :ref:`Troubleshoot` section and let's see
whether we can help you discover what went wrong.

Bring Down the Network
^^^^^^^^^^^^^^^^^^^^^^

Finally, let's bring it all down so we can explore the network setup one step
at a time. The following will kill your containers, remove the crypto material
and four artifacts, and delete the chaincode images from your Docker Registry:

.. code:: bash

  ./byfn.sh down

Once again, you will be prompted to continue, respond with a ``y`` or hit the return key:

.. code:: bash

  Stopping with channel 'mychannel' and CLI timeout of '10'
  Continue? [Y/n] y
  proceeding ...
  WARNING: The CHANNEL_NAME variable is not set. Defaulting to a blank string.
  WARNING: The TIMEOUT variable is not set. Defaulting to a blank string.
  Removing network net_byfn
  468aaa6201ed
  ...
  Untagged: dev-peer1.org2.example.com-mycc-1.0:latest
  Deleted: sha256:ed3230614e64e1c83e510c0c282e982d2b06d148b1c498bbdcc429e2b2531e91
  ...

If you'd like to learn more about the underlying tooling and bootstrap mechanics,
continue reading.  In these next sections we'll walk through the various steps
and requirements to build a fully-functional Hyperledger Fabric network.

.. note:: The manual steps outlined below assume that the ``FABRIC_LOGGING_SPEC`` in
          the ``cli`` container is set to ``DEBUG``. You can set this by modifying
          the ``docker-compose-cli.yaml`` file in the ``first-network`` directory.
          e.g.

          .. code::

            cli:
              container_name: cli
              image: hyperledger/fabric-tools:$IMAGE_TAG
              tty: true
              stdin_open: true
              environment:
                - GOPATH=/opt/gopath
                - CORE_VM_ENDPOINT=unix:///host/var/run/docker.sock
                - FABRIC_LOGGING_SPEC=DEBUG
                #- FABRIC_LOGGING_SPEC=INFO

Crypto Generator
----------------

We will use the ``cryptogen`` tool to generate the cryptographic material
(x509 certs and signing keys) for our various network entities.  These certificates are
representative of identities, and they allow for sign/verify authentication to
take place as our entities communicate and transact.

How does it work?
^^^^^^^^^^^^^^^^^

Cryptogen consumes a file --- ``crypto-config.yaml`` --- that contains the network
topology and allows us to generate a set of certificates and keys for both the
Organizations and the components that belong to those Organizations.  Each
Organization is provisioned a unique root certificate (``ca-cert``) that binds
specific components (peers and orderers) to that Org.  By assigning each
Organization a unique CA certificate, we are mimicking a typical network where
a participating :ref:`Member` would use its own Certificate Authority.
Transactions and communications within Hyperledger Fabric are signed by an
entity's private key (``keystore``), and then verified by means of a public
key (``signcerts``).

You will notice a ``count`` variable within this file. We use this to specify
the number of peers per Organization; in our case there are two peers per Org.
We won't delve into the minutiae of `x.509 certificates and public key
infrastructure <https://en.wikipedia.org/wiki/Public_key_infrastructure>`_
right now. If you're interested, you can peruse these topics on your own time.

After we run the ``cryptogen`` tool, the generated certificates and keys will be
saved to a folder titled ``crypto-config``. Note that the ``crypto-config.yaml``
file lists five orderers as being tied to the orderer organization. While the
``cryptogen`` tool will create certificates for all five of these orderers. These orderers
will be used in a etcdraft ordering service implementation and be used to create the
system channel and ``mychannel``.

Configuration Transaction Generator
-----------------------------------

The ``configtxgen`` tool is used to create four configuration artifacts:

  * orderer ``genesis block``,
  * channel ``configuration transaction``,
  * and two ``anchor peer transactions`` - one for each Peer Org.

Please see :doc:`commands/configtxgen` for a complete description of this tool's functionality.

The orderer block is the :ref:`Genesis-Block` for the ordering service, and the
channel configuration transaction file is broadcast to the orderer at :ref:`Channel` creation
time.  The anchor peer transactions, as the name might suggest, specify each
Org's :ref:`Anchor-Peer` on this channel.

How does it work?
^^^^^^^^^^^^^^^^^

Configtxgen consumes a file - ``configtx.yaml`` - that contains the definitions
for the sample network. There are three members - one Orderer Org (``OrdererOrg``)
and two Peer Orgs (``Org1`` & ``Org2``) each managing and maintaining two peer nodes.
This file also specifies a consortium - ``SampleConsortium`` - consisting of our
two Peer Orgs.  Pay specific attention to the "Profiles" section at the bottom of
this file. You will notice that we have several unique profiles. A few are worth
noting:

* ``SampleMultiNodeEtcdRaft``: generates the genesis block for a Raft ordering
  service. Only used if you issue the ``-o`` flag and specify ``etcdraft``.

* ``TwoOrgsChannel``: generates the genesis block for our channel, ``mychannel``.

These headers are important, as we will pass them in as arguments when we create
our artifacts.

.. note:: Notice that our ``SampleConsortium`` is defined in
          the system-level profile and then referenced by
          our channel-level profile.  Channels exist within
          the purview of a consortium, and all consortia
          must be defined in the scope of the network at
          large.

This file also contains two additional specifications that are worth
noting. Firstly, we specify the anchor peers for each Peer Org
(``peer0.org1.example.com`` & ``peer0.org2.example.com``).  Secondly, we point to
the location of the MSP directory for each member, in turn allowing us to store the
root certificates for each Org in the orderer genesis block.  This is a critical
concept. Now any network entity communicating with the ordering service can have
its digital signature verified.

Run the tools
-------------

You can manually generate the certificates/keys and the various configuration
artifacts using the ``configtxgen`` and ``cryptogen`` commands. Alternately,
you could try to adapt the byfn.sh script to accomplish your objectives.

Manually generate the artifacts
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

You can refer to the ``generateCerts`` function in the byfn.sh script for the
commands necessary to generate the certificates that will be used for your
network configuration as defined in the ``crypto-config.yaml`` file. However,
for the sake of convenience, we will also provide a reference here.

First let's run the ``cryptogen`` tool.  Our binary is in the ``bin``
directory, so we need to provide the relative path to where the tool resides.

.. code:: bash

    ../bin/cryptogen generate --config=./crypto-config.yaml

You should see the following in your terminal:

.. code:: bash

  org1.example.com
  org2.example.com

The certs and keys (i.e. the MSP material) will be output into a directory - ``crypto-config`` -
at the root of the ``first-network`` directory.

Next, we need to tell the ``configtxgen`` tool where to look for the
``configtx.yaml`` file that it needs to ingest.  We will tell it look in our
present working directory:

.. code:: bash

    export FABRIC_CFG_PATH=$PWD

Then, we'll invoke the ``configtxgen`` tool to create the orderer genesis block:

.. code:: bash

  ../bin/configtxgen -profile SampleMultiNodeEtcdRaft -channelID byfn-sys-channel -outputBlock ./channel-artifacts/genesis.block

.. note:: The orderer genesis block and the subsequent artifacts we are about to create
          will be output into the ``channel-artifacts`` directory at the root of the
          ``first-network`` directory. The `channelID` in the above command is the
          name of the system channel.

.. _createchanneltx:

Create a Channel Configuration Transaction
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Next, we need to create the channel transaction artifact. Be sure to replace ``$CHANNEL_NAME`` or
set ``CHANNEL_NAME`` as an environment variable that can be used throughout these instructions:

.. code:: bash

    # The channel.tx artifact contains the definitions for our sample channel

    export CHANNEL_NAME=mychannel  && ../bin/configtxgen -profile TwoOrgsChannel -outputCreateChannelTx ./channel-artifacts/channel.tx -channelID $CHANNEL_NAME

Note that the ``TwoOrgsChannel`` profile will use the ordering service
configuration you specified when creating the genesis block for the network.

You should see an output similar to the following in your terminal:

.. code:: bash

  2017-10-26 19:24:05.324 EDT [common/tools/configtxgen] main -> INFO 001 Loading configuration
  2017-10-26 19:24:05.329 EDT [common/tools/configtxgen] doOutputChannelCreateTx -> INFO 002 Generating new channel configtx
  2017-10-26 19:24:05.329 EDT [common/tools/configtxgen] doOutputChannelCreateTx -> INFO 003 Writing new channel tx

Next, we will define the anchor peer for Org1 on the channel that we are
constructing. Again, be sure to replace ``$CHANNEL_NAME`` or set the environment
variable for the following commands.  The terminal output will mimic that of the
channel transaction artifact:

.. code:: bash

    ../bin/configtxgen -profile TwoOrgsChannel -outputAnchorPeersUpdate ./channel-artifacts/Org1MSPanchors.tx -channelID $CHANNEL_NAME -asOrg Org1MSP

Now, we will define the anchor peer for Org2 on the same channel:

.. code:: bash

    ../bin/configtxgen -profile TwoOrgsChannel -outputAnchorPeersUpdate ./channel-artifacts/Org2MSPanchors.tx -channelID $CHANNEL_NAME -asOrg Org2MSP

Start the network
-----------------

.. note:: If you ran the ``byfn.sh`` example above previously, be sure that you
          have brought down the test network before you proceed (see
          `Bring Down the Network`_).

We will leverage a script to spin up our network. The
docker-compose file references the images that we have previously downloaded,
and bootstraps the orderer with our previously generated ``genesis.block``.

We want to go through the commands manually in order to expose the
syntax and functionality of each call.

First let's start our network:

.. code:: bash

    docker-compose -f docker-compose-cli.yaml -f docker-compose-etcdraft2.yaml up -d

If you want to see the realtime logs for your network, then do not supply the ``-d`` flag.
If you let the logs stream, then you will need to open a second terminal to execute the CLI calls.

.. _peerenvvars:

Create & Join Channel
^^^^^^^^^^^^^^^^^^^^^

Recall that we created the channel configuration transaction using the
``configtxgen`` tool in the :ref:`createchanneltx` section, above. You can
repeat that process to create additional channel configuration transactions,
using the same or different profiles in the ``configtx.yaml`` that you pass
to the ``configtxgen`` tool. Then you can repeat the process defined in this
section to establish those other channels in your network.

We will enter the CLI container using the ``docker exec`` command:

.. code:: bash

        docker exec -it cli bash

If successful you should see the following:

.. code:: bash

        bash-5.0#

For the following CLI commands against ``peer0.org1.example.com`` to work, we need
to preface our commands with the four environment variables given below.  These
variables for ``peer0.org1.example.com`` are baked into the CLI container,
therefore we can operate without passing them. **HOWEVER**, if you want to send
calls to other peers or the orderer, keep the CLI container defaults targeting
``peer0.org1.example.com``, but override the environment variables as seen in the
example below when you make any CLI calls:

.. code:: bash

    # Environment variables for PEER0

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    CORE_PEER_LOCALMSPID="Org1MSP"
    CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

Next, we are going to pass in the generated channel configuration transaction
artifact that we created in the :ref:`createchanneltx` section (we called
it ``channel.tx``) to the orderer as part of the create channel request.

We specify our channel name with the ``-c`` flag and our channel configuration
transaction with the ``-f`` flag. In this case it is ``channel.tx``, however
you can mount your own configuration transaction with a different name.  Once again
we will set the ``CHANNEL_NAME`` environment variable within our CLI container so that
we don't have to explicitly pass this argument. Channel names must be all lower
case, less than 250 characters long and match the regular expression
``[a-z][a-z0-9.-]*``.

.. code:: bash

        export CHANNEL_NAME=mychannel

        # the channel.tx file is mounted in the channel-artifacts directory within your CLI container
        # as a result, we pass the full path for the file
        # we also pass the path for the orderer ca-cert in order to verify the TLS handshake
        # be sure to export or replace the $CHANNEL_NAME variable appropriately

        peer channel create -o orderer.example.com:7050 -c $CHANNEL_NAME -f ./channel-artifacts/channel.tx --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

.. note:: Notice the ``--cafile`` that we pass as part of this command.  It is
          the local path to the orderer's root cert, allowing us to verify the
          TLS handshake.

This command returns a genesis block - ``<CHANNEL_NAME.block>`` - which we will use to join the channel.
It contains the configuration information specified in ``channel.tx``  If you have not
made any modifications to the default channel name, then the command will return you a
proto titled ``mychannel.block``.

.. note:: You will remain in the CLI container for the remainder of
          these manual commands. You must also remember to preface all commands
          with the corresponding environment variables when targeting a peer other than
          ``peer0.org1.example.com``.

Now let's join ``peer0.org1.example.com`` to the channel.

.. code:: bash

        # By default, this joins ``peer0.org1.example.com`` only
        # the <CHANNEL_NAME.block> was returned by the previous command
        # if you have not modified the channel name, you will join with mychannel.block
        # if you have created a different channel name, then pass in the appropriately named block

         peer channel join -b mychannel.block

You can make other peers join the channel as necessary by making appropriate
changes in the four environment variables we used in the :ref:`peerenvvars`
section, above.

Rather than join every peer, we will simply join ``peer0.org2.example.com`` so that
we can properly update the anchor peer definitions in our channel.  Since we are
overriding the default environment variables baked into the CLI container, this full
command will be the following:

.. code:: bash

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp CORE_PEER_ADDRESS=peer0.org2.example.com:9051 CORE_PEER_LOCALMSPID="Org2MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt peer channel join -b mychannel.block

Alternatively, you could choose to set these environment variables individually
rather than passing in the entire string.  Once they've been set, you simply need
to issue the ``peer channel join`` command again and the CLI container will act
on behalf of ``peer0.org2.example.com``.

Update the anchor peers
^^^^^^^^^^^^^^^^^^^^^^^

The following commands are channel updates and they will propagate to the definition
of the channel.  In essence, we adding additional configuration information on top
of the channel's genesis block.  Note that we are not modifying the genesis block, but
simply adding deltas into the chain that will define the anchor peers.

Update the channel definition to define the anchor peer for Org1 as ``peer0.org1.example.com``:

.. code:: bash

  peer channel update -o orderer.example.com:7050 -c $CHANNEL_NAME -f ./channel-artifacts/Org1MSPanchors.tx --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

Now update the channel definition to define the anchor peer for Org2 as ``peer0.org2.example.com``.
Identically to the ``peer channel join`` command for the Org2 peer, we will need to
preface this call with the appropriate environment variables.

.. code:: bash

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp CORE_PEER_ADDRESS=peer0.org2.example.com:9051 CORE_PEER_LOCALMSPID="Org2MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt peer channel update -o orderer.example.com:7050 -c $CHANNEL_NAME -f ./channel-artifacts/Org2MSPanchors.tx --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

.. _install-define-chaincode:

Install and define a chaincode
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. note:: We will utilize a simple existing chaincode. To learn how to write
          your own chaincode, see the :doc:`chaincode4ade` tutorial.

.. note:: These instructions use the Fabric chaincode lifecycle introduced in
          the v2.0 release. If you would like to use the previous lifecycle to
          install and instantiate a chaincode, visit the v1.4 version of the
          `Building your first network tutorial <https://hyperledger-fabric.readthedocs.io/en/release-1.4/build_network.html>`__.

Applications interact with the blockchain ledger through ``chaincode``.
Therefore we need to install a chaincode on every peer that will execute and
endorse our transactions. However, before we can interact with our chaincode,
the members of the channel need to agree on a chaincode definition that
establishes chaincode governance.

We need to package the chaincode before it can be installed on our peers. For
each package you create, you need to provide a chaincode package label as a
description of the chaincode. Use the following commands to package a sample
Go, Node.js or Java chaincode.

**Go**

.. code:: bash

    # before packaging Go chaincode, vendoring Go dependencies is required like the following commands.
    cd /opt/gopath/src/github.com/hyperledger/fabric-samples/chaincode/abstore/go
    GO111MODULE=on go mod vendor
    cd -

    # this packages a Go chaincode.
    # make note of the --lang flag to indicate "golang" chaincode
    # for Go chaincode --path takes the relative path from $GOPATH/src
    # The --label flag is used to create the package label
    peer lifecycle chaincode package mycc.tar.gz --path github.com/hyperledger/fabric-samples/chaincode/abstore/go/ --lang golang --label mycc_1

**Node.js**

.. code:: bash

    # this packages a Node.js chaincode
    # make note of the --lang flag to indicate "node" chaincode
    # for node chaincode --path takes the absolute path to the Node.js chaincode
    # The --label flag is used to create the package label
    peer lifecycle chaincode package mycc.tar.gz --path /opt/gopath/src/github.com/hyperledger/fabric-samples/chaincode/abstore/javascript/ --lang node --label mycc_1

**Java**

.. code:: bash

    # this packages a java chaincode
    # make note of the --lang flag to indicate "java" chaincode
    # for java chaincode --path takes the absolute path to the Java chaincode
    # The --label flag is used to create the package label
    peer lifecycle chaincode package mycc.tar.gz --path /opt/gopath/src/github.com/hyperledger/fabric-samples/chaincode/abstore/java/ --lang java --label mycc_1

Each of the above commands will create a chaincode package named ``mycc.tar.gz``,
which we can use to install the chaincode on our peers. Issue the following
command to install the package on peer0 of Org1.

.. code:: bash

    # this command installs a chaincode package on your peer
    peer lifecycle chaincode install mycc.tar.gz

A successful install command will return a chaincode package identifier. You
should see output similar to the following:

.. code:: bash

    2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nEmycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173" >
    2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: mycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173

You can also find the chaincode package identifier by querying your peer for
information about the packages you have installed.

.. code:: bash

    # this returns the details of the chaincode packages installed on your peers
    peer lifecycle chaincode queryinstalled

The command above will return the same package identifier as the install command.
You should see output similar to the following:

.. code:: bash

      Get installed chaincodes on peer:
      Package ID: mycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173, Label: mycc_1

We are going to need the package ID for future commands, so let's go ahead and
save it as an environment variable. Paste the package ID returned by the
`peer lifecycle chaincode queryinstalled` command into the command below. The
package ID may not be the same for all users, so you need to complete this step
using the package ID returned from your console.

.. code:: bash

   # Save the package ID as an environment variable.

   CC_PACKAGE_ID=mycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173

The endorsement policy of ``mycc`` will be set to require endorsements from a
peer in both Org1 and Org2. Therefore, we also need to install the chaincode on
a peer in Org2.

Modify the following four environment variables to issue the install command
as Org2:

.. code:: bash

   # Environment variables for PEER0 in Org2

   CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
   CORE_PEER_ADDRESS=peer0.org2.example.com:9051
   CORE_PEER_LOCALMSPID="Org2MSP"
   CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt

Now install the chaincode package onto peer0 of Org2. The following command
will install the chaincode and return same identifier as the install command we
issued as Org1.

.. code:: bash

    # this installs a chaincode package on your peer
    peer lifecycle chaincode install mycc.tar.gz

After you install the package, you need to approve a chaincode definition
for your organization. The chaincode definition includes the important
parameters of chaincode governance, including the chaincode name and version.
The definition also includes the package identifier used to associate the
chaincode package installed on your peers with a chaincode definition approved
by your organization.

Because we set the environment variables to operate as Org2, we can use the
following command to approve a definition of the ``mycc`` chaincode for
Org2. The approval is distributed to peers within each organization, so
the command does not need to target every peer within an organization.

.. code:: bash

    # this approves a chaincode definition for your org
    # make note of the --package-id flag that provides the package ID
    # use the --init-required flag to request the ``Init`` function be invoked to initialize the chaincode
    peer lifecycle chaincode approveformyorg --channelID $CHANNEL_NAME --name mycc --version 1.0 --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

We could have provided a ``--signature-policy`` or ``--channel-config-policy``
argument to the command above to set the chaincode endorsement policy. The
endorsement policy specifies how many peers belonging to different channel
members need to validate a transaction against a given chaincode. Because we did
not set a policy, the definition of ``mycc`` will use the default endorsement
policy, which requires that a transaction be endorsed by a majority of channel
members present when the transaction is submitted. This implies that if new
organizations are added to or removed from the channel, the endorsement policy
is updated automatically to require more or fewer endorsements. In this tutorial,
the default policy will require an endorsement from a peer belonging to Org1
**AND** Org2 (i.e. two endorsements). See the :doc:`endorsement-policies`
documentation for more details on policy implementation.

All organizations need to agree on the definition before they can use the
chaincode. Modify the following four environment variables to operate as Org1:

.. code:: bash

    # Environment variables for PEER0

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    CORE_PEER_LOCALMSPID="Org1MSP"
    CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

You can now approve a definition for the ``mycc`` chaincode as Org1. Chaincode is
approved at the organization level. You can issue the command once even if you
have multiple peers.

.. code:: bash

    # this defines a chaincode for your org
    # make note of the --package-id flag that provides the package ID
    # use the --init-required flag to request the Init function be invoked to initialize the chaincode
    peer lifecycle chaincode approveformyorg --channelID $CHANNEL_NAME --name mycc --version 1.0 --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

Once a sufficient number of channel members have approved a chaincode definition,
one member can commit the definition to the channel. By default a majority of
channel members need to approve a definition before it can be committed. It is
possible to check whether the chaincode definition is ready to be committed and
view the current approvals by organization by issuing the following query:

.. code:: bash

    # the flags used for this command are identical to those used for approveformyorg
    # except for --package-id which is not required since it is not stored as part of
    # the definition
    peer lifecycle chaincode checkcommitreadiness --channelID $CHANNEL_NAME --name mycc --version 1.0 --init-required --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem --output json

The command will produce as output a JSON map showing if the organizations in the
channel have approved the chaincode definition provided in the checkcommitreadiness
command. In this case, given that both organizations have approved, we obtain:

.. code:: bash

    {
            "Approvals": {
                    "Org1MSP": true,
                    "Org2MSP": true
            }
    }

Since both channel members have approved the definition, we can now commit it to
the channel using the following command. You can issue this command as either
Org1 or Org2. Note that the transaction targets peers in Org1 and Org2 to
collect endorsements.

.. code:: bash

    # this commits the chaincode definition to the channel
    peer lifecycle chaincode commit -o orderer.example.com:7050 --channelID $CHANNEL_NAME --name mycc --version 1.0 --sequence 1 --init-required --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt

Invoking the chaincode
^^^^^^^^^^^^^^^^^^^^^^

After a chaincode definition has been committed to a channel, we are ready to
invoke the chaincode and start interacting with the ledger. We requested the
execution of the ``Init`` function in the chaincode definition using the
``--init-required`` flag. As a result, we need to pass the ``--isInit`` flag to
its first invocation and supply the arguments to the ``Init`` function. Issue the
following command to initialize the chaincode and put the initial data on the
ledger.

.. code:: bash

    # be sure to set the -C and -n flags appropriately
    # use the --isInit flag if you are invoking an Init function
    peer chaincode invoke -o orderer.example.com:7050 --isInit --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n mycc --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["Init","a","100","b","100"]}' --waitForEvent

The first invoke will start the chaincode container. We may need to wait for the
container to start. Node.js images will take longer.

Query
^^^^^

Let's query the chaincode to make sure that the container was properly started
and the state DB was populated. The syntax for query is as follows:

.. code:: bash

  # be sure to set the -C and -n flags appropriately

  peer chaincode query -C $CHANNEL_NAME -n mycc -c '{"Args":["query","a"]}'

Invoke
^^^^^^

Now let’s move ``10`` from ``a`` to ``b``. This transaction will cut a new block
and update the state DB. The syntax for invoke is as follows:

.. code:: bash

  # be sure to set the -C and -n flags appropriately
  peer chaincode invoke -o orderer.example.com:7050 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n mycc --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["invoke","a","b","10"]}' --waitForEvent

Query
^^^^^

Let's confirm that our previous invocation executed properly. We initialized the
key ``a`` with a value of ``100`` and just removed ``10`` with our previous
invocation. Therefore, a query against ``a`` should return ``90``. The syntax
for query is as follows.

.. code:: bash

  # be sure to set the -C and -n flags appropriately

  peer chaincode query -C $CHANNEL_NAME -n mycc -c '{"Args":["query","a"]}'

We should see the following:

.. code:: bash

   Query Result: 90

Install the chaincode on an additional peer
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

If you want additional peers to interact with the ledger, then you will need to
join them to the channel and install the same chaincode package on the peers.
You only need to approve the chaincode definition once from your organization.
A chaincode container will be launched for each peer as soon as they try to
interact with that specific chaincode. Again, be cognizant of the fact that the
Node.js images will be slower to build and start upon the first invoke.

We will install the chaincode on a third peer, peer1 in Org2. Modify the
following four environment variables to issue the install command against peer1
in Org2:

.. code:: bash

   # Environment variables for PEER1 in Org2

   CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
   CORE_PEER_ADDRESS=peer1.org2.example.com:10051
   CORE_PEER_LOCALMSPID="Org2MSP"
   CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer1.org2.example.com/tls/ca.crt

Now install the ``mycc`` package on peer1 of Org2:

.. code:: bash

    # this command installs a chaincode package on your peer
    peer lifecycle chaincode install mycc.tar.gz

Query
^^^^^

Let's confirm that we can issue the query to Peer1 in Org2. We initialized the
key ``a`` with a value of ``100`` and just removed ``10`` with our previous
invocation. Therefore, a query against ``a`` should still return ``90``.

Peer1 in Org2 must first join the channel before it can respond to queries. The
channel can be joined by issuing the following command:

.. code:: bash

  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp CORE_PEER_ADDRESS=peer1.org2.example.com:10051 CORE_PEER_LOCALMSPID="Org2MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer1.org2.example.com/tls/ca.crt peer channel join -b mychannel.block

After the join command returns, the query can be issued. The syntax for query is
as follows.

.. code:: bash

  # be sure to set the -C and -n flags appropriately

  peer chaincode query -C $CHANNEL_NAME -n mycc -c '{"Args":["query","a"]}'

We should see the following:

.. code:: bash

   Query Result: 90

If you received an error, it may be because it takes a few seconds for the
peer to join and catch up to the current blockchain height. You may
re-query as needed. Feel free to perform additional invokes as well.

.. _behind-scenes:

What's happening behind the scenes?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. note:: These steps describe the scenario in which
          ``script.sh`` is run by './byfn.sh up'.  Clean your network
          with ``./byfn.sh down`` and ensure
          this command is active.  Then use the same
          docker-compose prompt to launch your network again

-  A script - ``script.sh`` - is baked inside the CLI container. The
   script drives the ``createChannel`` command against the supplied channel name
   and uses the channel.tx file for channel configuration.

-  The output of ``createChannel`` is a genesis block -
   ``<your_channel_name>.block`` - which gets stored on the peers' file systems and contains
   the channel configuration specified from channel.tx.

-  The ``joinChannel`` command is exercised for all four peers, which takes as
   input the previously generated genesis block.  This command instructs the
   peers to join ``<your_channel_name>`` and create a chain starting with ``<your_channel_name>.block``.

-  Now we have a channel consisting of four peers, and two
   organizations.  This is our ``TwoOrgsChannel`` profile.

-  ``peer0.org1.example.com`` and ``peer1.org1.example.com`` belong to Org1;
   ``peer0.org2.example.com`` and ``peer1.org2.example.com`` belong to Org2

-  These relationships are defined through the ``crypto-config.yaml`` and
   the MSP path is specified in our docker compose.

-  The anchor peers for Org1MSP (``peer0.org1.example.com``) and
   Org2MSP (``peer0.org2.example.com``) are then updated.  We do this by passing
   the ``Org1MSPanchors.tx`` and ``Org2MSPanchors.tx`` artifacts to the ordering
   service along with the name of our channel.

-  A chaincode - **abstore** - is packaged and installed on ``peer0.org1.example.com``
   and ``peer0.org2.example.com``

-  The chaincode is then separately approved by Org1 and Org2, and then committed
   on the channel. Since an endorsement policy was not specified, the channel's
   default endorsement policy of a majority of organizations will get utilized,
   meaning that any transaction must be endorsed by a peer tied to Org1 and Org2.

-  The chaincode Init is then called which starts the container for the target peer,
   and initializes the key value pairs associated with the chaincode.  The initial
   values for this example are ["a","100" "b","200"]. This first invoke results
   in a container by the name of ``dev-peer0.org2.example.com-mycc-1.0`` starting.

-  A query against the value of "a" is issued to ``peer0.org2.example.com``.
   A container for Org2 peer0 by the name of ``dev-peer0.org2.example.com-mycc-1.0``
   was started when the chaincode was initialized. The result of the query is
   returned. No write operations have occurred, so a query against "a" will
   still return a value of "100".

-  An invoke is sent to ``peer0.org1.example.com`` and ``peer0.org2.example.com``
   to move "10" from "a" to "b"

-  A query is sent to ``peer0.org2.example.com`` for the value of "a". A
   value of 90 is returned, correctly reflecting the previous
   transaction during which the value for key "a" was modified by 10.

-  The chaincode - **abstore** - is installed on ``peer1.org2.example.com``

-  A query is sent to ``peer1.org2.example.com`` for the value of "a". This starts a
   third chaincode container by the name of ``dev-peer1.org2.example.com-mycc-1.0``. A
   value of 90 is returned, correctly reflecting the previous
   transaction during which the value for key "a" was modified by 10.

What does this demonstrate?
^^^^^^^^^^^^^^^^^^^^^^^^^^^

Chaincode **MUST** be installed on a peer in order for it to
successfully perform read/write operations against the ledger.
Furthermore, a chaincode container is not started for a peer until an ``init`` or
traditional transaction - read/write - is performed against that chaincode (e.g. query for
the value of "a"). The transaction causes the container to start. Also,
all peers in a channel maintain an exact copy of the ledger which
comprises the blockchain to store the immutable, sequenced record in
blocks, as well as a state database to maintain a snapshot of the current state.
This includes those peers that do not have chaincode installed on them
(like ``peer1.org1.example.com`` in the above example) . Finally, the chaincode is accessible
after it is installed (like ``peer1.org2.example.com`` in the above example) because its
definition has already been committed on the channel.

How do I see these transactions?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Check the logs for the CLI Docker container.

.. code:: bash

        docker logs -f cli

You should see the following output:

.. code:: bash

      2017-05-16 17:08:01.366 UTC [msp] GetLocalMSP -> DEBU 004 Returning existing local MSP
      2017-05-16 17:08:01.366 UTC [msp] GetDefaultSigningIdentity -> DEBU 005 Obtaining default signing identity
      2017-05-16 17:08:01.366 UTC [msp/identity] Sign -> DEBU 006 Sign: plaintext: 0AB1070A6708031A0C08F1E3ECC80510...6D7963631A0A0A0571756572790A0161
      2017-05-16 17:08:01.367 UTC [msp/identity] Sign -> DEBU 007 Sign: digest: E61DB37F4E8B0D32C9FE10E3936BA9B8CD278FAA1F3320B08712164248285C54
      Query Result: 90
      2017-05-16 17:08:15.158 UTC [main] main -> INFO 008 Exiting.....
      ===================== Query successful on peer1.org2 on channel 'mychannel' =====================

      ===================== All GOOD, BYFN execution completed =====================


       _____   _   _   ____
      | ____| | \ | | |  _ \
      |  _|   |  \| | | | | |
      | |___  | |\  | | |_| |
      |_____| |_| \_| |____/

You can scroll through these logs to see the various transactions.

How can I see the chaincode logs?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

You can inspect the individual chaincode containers to see the separate
transactions executed against each container. Use the following command to find
the list of running containers to find your chaincode containers:

.. code:: bash

    $ docker ps -a
    CONTAINER ID        IMAGE                                                                                                                                                                 COMMAND                  CREATED              STATUS              PORTS                                NAMES
    7aa7d9e199f5        dev-peer1.org2.example.com-mycc_1-27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8-2eba360c66609a3ba78327c2c86bc3abf041c78f5a35553191a1acf1efdd5a0d   "chaincode -peer.add…"   About a minute ago   Up About a minute                                        dev-peer1.org2.example.com-mycc_1-27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8
    82ce129c0fe6        dev-peer0.org2.example.com-mycc_1-27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8-1297906045aa77086daba21aba47e8eef359f9498b7cb2b010dff3e2a354565a   "chaincode -peer.add…"   About a minute ago   Up About a minute                                        dev-peer0.org2.example.com-mycc_1-27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8
    eaef1a8f7acf        dev-peer0.org1.example.com-mycc_1-27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8-00d8dbefd85a4aeb9428b7df95df9744be1325b2a60900ac7a81796e67e4280a   "chaincode -peer.add…"   2 minutes ago        Up 2 minutes                                             dev-peer0.org1.example.com-mycc_1-27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8
    da403175b785        hyperledger/fabric-tools:latest                                                                                                                                       "/bin/bash"              4 minutes ago        Up 4 minutes                                             cli
    c62a8d03818f        hyperledger/fabric-peer:latest                                                                                                                                        "peer node start"        4 minutes ago        Up 4 minutes        7051/tcp, 0.0.0.0:9051->9051/tcp     peer0.org2.example.com
    06593c4f3e53        hyperledger/fabric-peer:latest                                                                                                                                        "peer node start"        4 minutes ago        Up 4 minutes        0.0.0.0:7051->7051/tcp               peer0.org1.example.com
    4ddc928ebffe        hyperledger/fabric-orderer:latest                                                                                                                                     "orderer"                4 minutes ago        Up 4 minutes        0.0.0.0:7050->7050/tcp               orderer.example.com
    6d79e95ec059        hyperledger/fabric-peer:latest                                                                                                                                        "peer node start"        4 minutes ago        Up 4 minutes        7051/tcp, 0.0.0.0:10051->10051/tcp   peer1.org2.example.com
    6aad6b40fd30        hyperledger/fabric-peer:latest                                                                                                                                        "peer node start"        4 minutes ago        Up 4 minutes        7051/tcp, 0.0.0.0:8051->8051/tcp     peer1.org1.example.com

The chaincode containers are the images starting with `dev-peer`. You can then
use the container ID to find the logs from each chaincode container.

.. code:: bash

        $ docker logs 7aa7d9e199f5
        ABstore Init
        Aval = 100, Bval = 100
        ABstore Invoke
        Aval = 90, Bval = 110

        $ docker logs eaef1a8f7acf
        ABstore Init
        Aval = 100, Bval = 100
        ABstore Invoke
        Query Response:{"Name":"a","Amount":"100"}
        ABstore Invoke
        Aval = 90, Bval = 110
        ABstore Invoke
        Query Response:{"Name":"a","Amount":"90"}

You can also see the peer logs to view chaincode invoke messages
and block commit messages:

.. code:: bash

          $ docker logs peer0.org1.example.com

Understanding the Docker Compose topology
-----------------------------------------

The BYFN sample offers us two flavors of Docker Compose files, both of which
are extended from the ``docker-compose-base.yaml`` (located in the ``base``
folder).  Our first flavor, ``docker-compose-cli.yaml``, provides us with a
CLI container, along with an orderer, four peers.  We use this file
for the entirety of the instructions on this page.

.. note:: the remainder of this section covers a docker-compose file designed for the
          SDK.  Refer to the `Node SDK <https://github.com/hyperledger/fabric-sdk-node>`__
          repo for details on running these tests.

The second flavor, ``docker-compose-e2e.yaml``, is constructed to run end-to-end tests
using the Node.js SDK.  Aside from functioning with the SDK, its primary differentiation
is that there are containers for the fabric-ca servers.  As a result, we are able
to send REST calls to the organizational CAs for user registration and enrollment.

If you want to use the ``docker-compose-e2e.yaml`` without first running the
byfn.sh script, then we will need to make four slight modifications.
We need to point to the private keys for our Organization's CA's.  You can locate
these values in your crypto-config folder.  For example, to locate the private
key for Org1 we would follow this path - ``crypto-config/peerOrganizations/org1.example.com/ca/``.
The private key is a long hash value followed by ``_sk``.  The path for Org2
would be - ``crypto-config/peerOrganizations/org2.example.com/ca/``.

In the ``docker-compose-e2e.yaml`` update the FABRIC_CA_SERVER_TLS_KEYFILE variable
for ca0 and ca1.  You also need to edit the path that is provided in the command
to start the ca server.  You are providing the same private key twice for each
CA container.

Using CouchDB
-------------

The state database can be switched from the default (goleveldb) to CouchDB.
The same chaincode functions are available with CouchDB, however, there is the
added ability to perform rich and complex queries against the state database
data content contingent upon the chaincode data being modeled as JSON.

To use CouchDB instead of the default database (goleveldb), follow the same
procedures outlined earlier for generating the artifacts, except when starting
the network pass ``docker-compose-couch.yaml`` as well:

.. code:: bash

    docker-compose -f docker-compose-cli.yaml -f docker-compose-couch.yaml -f docker-compose-etcdraft2.yaml up -d

**abstore** should now work using CouchDB underneath.

.. note::  If you choose to implement mapping of the fabric-couchdb container
           port to a host port, please make sure you are aware of the security
           implications. Mapping of the port in a development environment makes the
           CouchDB REST API available, and allows the
           visualization of the database via the CouchDB web interface (Fauxton).
           Production environments would likely refrain from implementing port mapping in
           order to restrict outside access to the CouchDB containers.

You can use **abstore** chaincode against the CouchDB state database
using the steps outlined above, however in order to exercise the CouchDB query
capabilities you will need to use a chaincode that has data modeled as JSON.
The sample chaincode **marbles02** has been written to demostrate the queries
you can issue from your chaincode if you are using a CouchDB database. You can
locate the **marbles02** chaincode in the ``fabric/examples/chaincode/go``
directory.

We will follow the same process to create and join the channel as outlined in the
:ref:`peerenvvars` section above.  Once you have joined your peer(s) to the
channel, use the following steps to interact with the **marbles02** chaincode:


- Package and install the chaincode on ``peer0.org1.example.com``:

.. code:: bash

       # before packaging Go chaincode, vendoring dependencies is required.
       cd /opt/gopath/src/github.com/hyperledger/fabric-samples/chaincode/marbles02/go
       GO111MODULE=on go mod vendor
       cd -

       # package and install the Go chaincode
       peer lifecycle chaincode package marbles.tar.gz --path github.com/hyperledger/fabric-samples/chaincode/marbles02/go/ --lang golang --label marbles_1
       peer lifecycle chaincode install marbles.tar.gz

The install command will return a chaincode packageID that you will use to
approve a chaincode definition.

.. code:: bash

      2019-04-08 20:10:32.568 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nJmarbles_1:cfb623954827aef3f35868764991cc7571b445a45cfd3325f7002f14156d61ae\022\tmarbles_1" >
      2019-04-08 20:10:32.568 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: marbles_1:cfb623954827aef3f35868764991cc7571b445a45cfd3325f7002f14156d61ae

- Save the packageID as an environment variable so you can pass it to future
  commands:

  .. code:: bash

      CC_PACKAGE_ID=marbles_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173

- Approve a chaincode definition as Org1:

.. code:: bash

       # be sure to modify the $CHANNEL_NAME variable accordingly for the command

       peer lifecycle chaincode approveformyorg --channelID $CHANNEL_NAME --name marbles --version 1.0 --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

- Install the chaincode on ``peer0.org2.example.com``:

.. code:: bash

      CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
      CORE_PEER_ADDRESS=peer0.org2.example.com:9051
      CORE_PEER_LOCALMSPID="Org2MSP"
      CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
      peer lifecycle chaincode install marbles.tar.gz

- Approve a chaincode definition as Org2, and then commit the definition to the
  channel:

.. code:: bash

       # be sure to modify the $CHANNEL_NAME variable accordingly for the command

       peer lifecycle chaincode approveformyorg --channelID $CHANNEL_NAME --name marbles --version 1.0 --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
       peer lifecycle chaincode commit -o orderer.example.com:7050 --channelID $CHANNEL_NAME --name marbles --version 1.0 --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt

- We can now create some marbles. The first invoke of the chaincode will start
  the chaincode container. You may need to wait for the container to start.

.. code:: bash

       # be sure to modify the $CHANNEL_NAME variable accordingly

       peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n marbles --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["initMarble","marble1","blue","35","tom"]}'

Once the container has started, you can issue additional commands to create
some marbles and move them around:

.. code:: bash

        # be sure to modify the $CHANNEL_NAME variable accordingly

        peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n marbles --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["initMarble","marble2","red","50","tom"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n marbles --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["initMarble","marble3","blue","70","tom"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n marbles --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["transferMarble","marble2","jerry"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n marbles --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["transferMarblesBasedOnColor","blue","jerry"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n marbles --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["delete","marble1"]}'

-  If you chose to map the CouchDB ports in docker-compose, you can now view
   the state database through the CouchDB web interface (Fauxton) by opening
   a browser and navigating to the following URL:

   ``http://localhost:5984/_utils``

You should see a database named ``mychannel`` (or your unique channel name) and
the documents inside it.

.. note:: For the below commands, be sure to update the $CHANNEL_NAME variable appropriately.

You can run regular queries from the CLI (e.g. reading ``marble2``):

.. code:: bash

      peer chaincode query -C $CHANNEL_NAME -n marbles -c '{"Args":["readMarble","marble2"]}'

The output should display the details of ``marble2``:

.. code:: bash

       Query Result: {"color":"red","docType":"marble","name":"marble2","owner":"jerry","size":50}

You can retrieve the history of a specific marble - e.g. ``marble1``:

.. code:: bash

      peer chaincode query -C $CHANNEL_NAME -n marbles -c '{"Args":["getHistoryForMarble","marble1"]}'

The output should display the transactions on ``marble1``:

.. code:: bash

      Query Result: [{"TxId":"1c3d3caf124c89f91a4c0f353723ac736c58155325f02890adebaa15e16e6464", "Value":{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}},{"TxId":"755d55c281889eaeebf405586f9e25d71d36eb3d35420af833a20a2f53a3eefd", "Value":{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"jerry"}},{"TxId":"819451032d813dde6247f85e56a89262555e04f14788ee33e28b232eef36d98f", "Value":}]

You can also perform rich queries on the data content, such as querying marble fields by owner ``jerry``:

.. code:: bash

      peer chaincode query -C $CHANNEL_NAME -n marbles -c '{"Args":["queryMarblesByOwner","jerry"]}'

The output should display the two marbles owned by ``jerry``:

.. code:: bash

       Query Result: [{"Key":"marble2", "Record":{"color":"red","docType":"marble","name":"marble2","owner":"jerry","size":50}},{"Key":"marble3", "Record":{"color":"blue","docType":"marble","name":"marble3","owner":"jerry","size":70}}]


Why CouchDB
-------------
CouchDB is a kind of NoSQL solution. It is a document-oriented database where document fields are stored as key-value maps. Fields can be either a simple key-value pair, list, or map.
In addition to keyed/composite-key/key-range queries which are supported by LevelDB, CouchDB also supports full data rich queries capability, such as non-key queries against the whole blockchain data,
since its data content is stored in JSON format and fully queryable. Therefore, CouchDB can meet chaincode, auditing, reporting requirements for many use cases that not supported by LevelDB.

CouchDB can also enhance the security for compliance and data protection in the blockchain. As it is able to implement field-level security through the filtering and masking of individual attributes within a transaction, and only authorizing the read-only permission if needed.

A Note on Data Persistence
--------------------------

If data persistence is desired on the peer container or the CouchDB container,
one option is to mount a directory in the docker-host into a relevant directory
in the container. For example, you may add the following two lines in
the peer container specification in the ``docker-compose-base.yaml`` file:

.. code:: bash

       volumes:
        - /var/hyperledger/peer0:/var/hyperledger/production

For the CouchDB container, you may add the following two lines in the CouchDB
container specification:

.. code:: bash

       volumes:
        - /var/hyperledger/couchdb0:/opt/couchdb/data

.. _Troubleshoot:

Troubleshooting
---------------

-  Always start your network fresh.  Use the following command
   to remove artifacts, crypto, containers and chaincode images:

   .. code:: bash

      ./byfn.sh down

   .. note:: You **will** see errors if you do not remove old containers
             and images.

-  If you see Docker errors, first check your docker version (:doc:`prereqs`),
   and then try restarting your Docker process.  Problems with Docker are
   oftentimes not immediately recognizable.  For example, you may see errors
   resulting from an inability to access crypto material mounted within a
   container.

   If they persist remove your images and start from scratch:

   .. code:: bash

       docker rm -f $(docker ps -aq)
       docker rmi -f $(docker images -q)

-  If you see errors on your create, approve, commit, invoke or query commands,
   make sure you have properly updated the channel name and chaincode name.
   There are placeholder values in the supplied sample commands.

-  If you see the below error:

   .. code:: bash

       Error: Error endorsing chaincode: rpc error: code = 2 desc = Error installing chaincode code mycc:1.0(chaincode /var/hyperledger/production/chaincodes/mycc.1.0 exits)

   You likely have chaincode images (e.g. ``dev-peer1.org2.example.com-mycc-1.0`` or
   ``dev-peer0.org1.example.com-mycc-1.0``) from prior runs. Remove them and try
   again.

   .. code:: bash

       docker rmi -f $(docker images | grep dev-peer[0-9] | awk '{print $3}')

-  If you see something similar to the following:

   .. code:: bash

      Error connecting: rpc error: code = 14 desc = grpc: RPC failed fast due to transport failure
      Error: rpc error: code = 14 desc = grpc: RPC failed fast due to transport failure

   Make sure you are running your network against the "1.0.0" images that have
   been retagged as "latest".

-  If you see the below error:

   .. code:: bash

     [configtx/tool/localconfig] Load -> CRIT 002 Error reading configuration: Unsupported Config Type ""
     panic: Error reading configuration: Unsupported Config Type ""

   Then you did not set the ``FABRIC_CFG_PATH`` environment variable properly.  The
   configtxgen tool needs this variable in order to locate the configtx.yaml.  Go
   back and execute an ``export FABRIC_CFG_PATH=$PWD``, then recreate your
   channel artifacts.

-  To cleanup the network, use the ``down`` option:

   .. code:: bash

       ./byfn.sh down

-  If you see an error stating that you still have "active endpoints", then prune
   your Docker networks.  This will wipe your previous networks and start you with a
   fresh environment:

   .. code:: bash

        docker network prune

   You will see the following message:

   .. code:: bash

      WARNING! This will remove all networks not used by at least one container.
      Are you sure you want to continue? [y/N]

   Select ``y``.

-  If you see an error similar to the following:

   .. code:: bash

      /bin/bash: ./scripts/script.sh: /bin/bash^M: bad interpreter: No such file or directory

   Ensure that the file in question (**script.sh** in this example) is encoded
   in the Unix format. This was most likely caused by not setting
   ``core.autocrlf`` to ``false`` in your Git configuration (see
   :ref:`windows-extras`). There are several ways of fixing this. If you have
   access to the vim editor for instance, open the file:

   .. code:: bash

      vim ./fabric-samples/first-network/scripts/script.sh

   Then change its format by executing the following vim command:

   .. code:: bash

      :set ff=unix

.. note:: If you continue to see errors, share your logs on the
          **fabric-questions** channel on
          `Hyperledger Rocket Chat <https://chat.hyperledger.org/home>`__
          or on `StackOverflow <https://stackoverflow.com/questions/tagged/hyperledger-fabric>`__.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
