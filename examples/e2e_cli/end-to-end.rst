End-to-End Flow
===============

The end-to-end verification provisions a sample Fabric network consisting of
two organizations, each maintaining two peers, and a Kafka-based ordering service.

This verification makes use of two fundamental tools, which are necessary to
create a functioning transactional network with digital signature validation
and access control:

* cryptogen - generates the x509 certificates used to identify and authenticate the various components in the network.
* configtxgen - generates the requisite configuration artifacts for orderer bootstrap and channel creation.

Each tool consumes a configuration yaml file, within which we specify the topology
of our network (cryptogen) and the location of our certificates for various
configuration operations (configtxgen).  Once the tools have been successfully run,
we are able to launch our network.  More detail on the tools and the structure of
the network will be provided later in this document.  For now, let's get going...

Prerequisites
-------------

- `Git client <https://git-scm.com/downloads>`__
- `Docker <https://www.docker.com/products/overview>`__ - v1.12 or higher
- `Docker Compose <https://docs.docker.com/compose/overview/>`__ - v1.8 or higher
- `Homebrew <https://brew.sh/>`__ - OSX only
- `Xcode <https://itunes.apple.com/us/app/xcode/id497799835?mt=12>`__ - OSX only (this can take upwards of an hour)
- `Docker Toolbox <https://docs.docker.com/toolbox/toolbox_install_windows/>`__ - Windows users only
- `Go <https://golang.org/>`__ - 1.7 or higher

On Windows machines you will also need the following which provides a better alternative to the Windows command prompt:

- `Git Bash <https://git-scm.com/downloads>`__

.. note:: On older versions of Windows, such as Windows 7, you
          typically get this as part of installing Docker
          Toolbox. However experience has shown this to be a poor
          development environment with limited functionality. It is
          suitable to run docker based scenarios, such as
          :doc:`getting_started`, but you may not be able to find a
          suitable ``make`` command to successfuly go through the
          scenario described here.

Setting the $GOPATH
^^^^^^^^^^^^^^^^^^^
Make sure you have properly setup your Host's `GOPATH environment
variable <https://github.com/golang/go/wiki/GOPATH>`__. This is necessary for the
code to compile properly.

Now create the following directory structure and ``cd`` into it:

.. code:: bash

      mkdir -p $GOPATH/src/github.com/hyperledger
      cd $GOPATH/src/github.com/hyperledger

-  Clone the Fabric code base into this path.

.. code:: bash

       git clone http://gerrit.hyperledger.org/r/fabric

or though a mirrored repository in github:

.. code:: bash

       git clone https://github.com/hyperledger/fabric.git

- If you are running OSX, perform the following:

.. code:: bash

        brew install gnu-tar --with-default-names
        brew install libtool

Build the binaries
^^^^^^^^^^^^^^^^^^

-  Now make the platform-specific binaries for ``cryptogen`` and ``configtxgen``.

.. code:: bash

       cd $GOPATH/src/github.com/hyperledger/fabric
       # ensure sure you are in the /fabric directory where the Makefile resides
       make release

This will output platform-specific binaries into the ``fabric/release`` folder.

-  Next, make the Fabric images.  This typically takes between five to ten minutes, so
   be patient:

.. code:: bash

       # make sure you are in the /fabric directory
       make docker

Execute a ``docker images`` command in your terminal. If the images compiled
successfully, you should see an output similar to the following:

.. code:: bash

  REPOSITORY                     TAG                   IMAGE ID            CREATED             SIZE
  hyperledger/fabric-couchdb     latest                e2df4dd39ca9        38 minutes ago      1.51 GB
  hyperledger/fabric-couchdb     x86_64-1.0.0-beta     e2df4dd39ca9        38 minutes ago      1.51 GB
  hyperledger/fabric-kafka       latest                08af4d797266        40 minutes ago      1.3 GB
  hyperledger/fabric-kafka       x86_64-1.0.0-beta     08af4d797266        40 minutes ago      1.3 GB
  hyperledger/fabric-zookeeper   latest                444e9e695367        40 minutes ago      1.31 GB
  hyperledger/fabric-zookeeper   x86_64-1.0.0-beta     444e9e695367        40 minutes ago      1.31 GB
  hyperledger/fabric-testenv     latest                8678d3101930        41 minutes ago      1.41 GB
  hyperledger/fabric-testenv     x86_64-1.0.0-beta     8678d3101930        41 minutes ago      1.41 GB
  hyperledger/fabric-buildenv    latest                60911392c82e        41 minutes ago      1.33 GB
  hyperledger/fabric-buildenv    x86_64-1.0.0-beta     60911392c82e        41 minutes ago      1.33 GB
  hyperledger/fabric-orderer     latest                2afab937b9cc        41 minutes ago      182 MB
  hyperledger/fabric-orderer     x86_64-1.0.0-beta     2afab937b9cc        41 minutes ago      182 MB
  hyperledger/fabric-peer        latest                9560e58e8089        41 minutes ago      185 MB
  hyperledger/fabric-peer        x86_64-1.0.0-beta     9560e58e8089        41 minutes ago      185 MB
  hyperledger/fabric-javaenv     latest                881ca5219fad        42 minutes ago      1.43 GB
  hyperledger/fabric-javaenv     x86_64-1.0.0-beta     881ca5219fad        42 minutes ago      1.43 GB
  hyperledger/fabric-ccenv       latest                28af77ffe9e9        43 minutes ago      1.29 GB
  hyperledger/fabric-ccenv       x86_64-1.0.0-beta     28af77ffe9e9        43 minutes ago      1.29 GB
  hyperledger/fabric-baseimage   x86_64-0.3.0          f4751a503f02        3 months ago        1.27 GB
  hyperledger/fabric-baseos      x86_64-0.3.0          c3a4cf3b3350        3 months ago        161 MB

If you failed to compile the ``fabric-testenv`` image, then you can
perform a ``make clean`` followed by another ``make docker``.

Cryptogen Tool
--------------
We will use the cryptogen tool to generate the cryptographic material (x509 certs)
for our various network entities.  The certificates are based on a standard PKI
implementation where validation is achieved by reaching a common trust anchor.

How does it work?
^^^^^^^^^^^^^^^^^

Cryptogen consumes a file - ``crypto-config.yaml`` - that contains the network
topology and allows us to generate a library of certificates for both the
Organizations and the components that belong to those Organizations.  Each
Organization is provisioned a unique root certificate (``ca-cert``), that binds
specific components (peers and orderers) to that Org.  Transactions and communications
within Fabric are signed by an entity's private key (``keystore``), and then verified
by means of a public key (``signcerts``).  You will notice a "count" variable within
this file.  We use this to specify the number of peers per Organization; in our
case it's two peers per Org.  The rest of this template is extremely
self-explanatory.

After we run the tool, the certs will be parked in a folder titled ``crypto-config``.

Configuration Transaction Generator
-----------------------------------

The `configtxgen
tool <https://github.com/hyperledger/fabric/blob/master/docs/source/configtxgen.rst>`__
is used to create four artifacts: orderer **bootstrap block**, fabric
**channel configuration transaction**, and two **anchor peer transactions** - one
for each Peer Org.

The orderer block is the genesis block for the ordering service, and the
channel transaction file is broadcast to the orderer at channel creation
time.  The anchor peer transactions, as the name might suggest, specify each
Org's anchor peer on this channel.

How does it work?
^^^^^^^^^^^^^^^^^

Configtxgen consumes a file - ``configtx.yaml`` - that contains the definitions
for the sample network. There are three members - one Orderer Org (``OrdererOrg``)
and two Peer Orgs (``Org1`` & ``Org2``) each managing and maintaining two peer nodes.
This file also specifies a consortium - ``SampleConsortium`` - consisting of our
two Peer Orgs.  Pay specific attention to the "Profiles" section at the top of
this file.  You will notice that we have two unique headers. One for the orderer genesis
block - ``TwoOrgsOrdererGenesis`` - and one for our channel - ``TwoOrgsChannel``.
These headers are important, as we will pass them in as arguments when we create
our artifacts.  This file also contains two additional specifications that are worth
noting.  Firstly, we specify the anchor peers for each Peer Org
(``peer0.org1.example.com`` & ``peer0.org2.example.com``).  Secondly, we point to
the location of the MSP directory for each member, in turn allowing us to store the
root certificates for each Org in the orderer genesis block.  This is a critical
concept. Now any network entity communicating with the ordering service can have
its digital signature verified.

For ease of use, a script - ``generateArtifacts.sh`` - is provided. The
script will generate the crypto material and our four configuration artifacts, and
subsequently output these files into the ``channel-artifacts`` folder.

Run the shell script
^^^^^^^^^^^^^^^^^^^^

Make sure you are in the ``examples/e2e_cli`` directory where the script resides.
Decide upon a unique name for your channel and replace the <channel-ID> parm
with a name of your choice.  The script will fail if you do not supply a name.

.. code:: bash

    cd examples/e2e_cli
    ./generateArtifacts.sh <channel-ID>

The output of the script is somewhat verbose, as it generates the crypto
libraries and multiple artifacts.  However, you will notice five distinct
and self-explanatory messages in your terminal.  They are as follows:

.. code:: bash

  ##########################################################
  ##### Generate certificates using cryptogen tool #########
  ##########################################################

  ##########################################################
  #########  Generating Orderer Genesis block ##############
  ##########################################################

  #################################################################
  ### Generating channel configuration transaction 'channel.tx' ###
  #################################################################

  #################################################################
  #######    Generating anchor peer update for Org0MSP   ##########
  #################################################################

  #################################################################
  #######    Generating anchor peer update for Org1MSP   ##########
  #################################################################


These configuration transactions will bundle the crypto material for the
participating members and their network components and output an orderer
genesis block and three channel transaction artifacts. These artifacts are
required to successfully bootstrap a Fabric network and create a channel to
transact upon.

Manually generate the artifacts (optional)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

You can refer to the ``generateArtifacts.sh`` script for the commands, however
for the sake of convenience we will also provide them here.

First, let's set the environment variable for our platform architecture.
This command will detect your OS and use the appropriate binaries for the subsequent steps:

.. code:: bash

  # for power or z
  os_arch=$(echo "$(uname -s)-$(uname -m)" | awk '{print tolower($0)}')

  # for linux, osx or windows
  os_arch=$(echo "$(uname -s)-amd64" | awk '{print tolower($0)}')

Check to make sure it is set properly:

.. code:: bash

  echo $os_arch

Now let's run the tool.  Our platform specific binary is in the ``release``
directory, so we need to provide the relative path to where the tool resides.
Make sure you are in ``examples/e2e_cli``:

.. code:: bash

    ./../../release/$os_arch/bin/cryptogen generate --config=./crypto-config.yaml

You will likely see the following warning.  It's innocuous, ignore it:

.. code:: bash

    [bccsp] GetDefault -> WARN 001 Before using BCCSP, please call InitFactories(). Falling back to bootBCCSP.

Next, we need to tell the ``configtxgen`` tool where to look for the
``configtx.yaml`` file that it needs to ingest.  We will tell it look in our
present working directory:

.. code:: bash

    FABRIC_CFG_PATH=$PWD

Create the orderer genesis block:

.. code:: bash

    ./../../release/$os_arch/bin/configtxgen -profile TwoOrgsOrdererGenesis -outputBlock ./channel-artifacts/genesis.block

You can ignore the logs regarding intermediate certs, we are not using them in
this crypto implementation.

Create the channel transaction artifact:

.. code:: bash

    # make sure to set the <channel-ID> parm
    ./../../release/$os_arch/bin/configtxgen -profile TwoOrgsChannel -outputCreateChannelTx ./channel-artifacts/channel.tx -channelID <channel-ID>

Define the anchor peer for Org1 on the channel:

.. code:: bash

    # make sure to set the <channel-ID> parm
    ./../../release/$os_arch/bin/configtxgen -profile TwoOrgsChannel -outputAnchorPeersUpdate ./channel-artifacts/Org1MSPanchors.tx -channelID <channel-ID> -asOrg Org1MSP

Define the anchor peer for Org2 on the channel:

.. code:: bash

    # make sure to set the <channel-ID> parm
    ./../../release/$os_arch/bin/configtxgen -profile TwoOrgsChannel -outputAnchorPeersUpdate ./channel-artifacts/Org2MSPanchors.tx -channelID <channel-ID> -asOrg Org2MSP

Run the end-to-end test with Docker
-----------------------------------

Make sure you are in the ``/e2e_cli`` directory. Then use docker-compose
to spawn the network entities and drive the tests.  Notice that you can set a
``TIMEOUT`` variable (specified in seconds) so that your cli container does not
exit after the script completes.  You can choose any value:

.. code:: bash

    # the TIMEOUT variable is optional
    CHANNEL_NAME=<channel-id> TIMEOUT=<pick_a_value> docker-compose -f docker-compose-cli.yaml up -d

If you created a unique channel name, be sure to pass in that parameter.
For example,

.. code:: bash

    CHANNEL_NAME=abc TIMEOUT=1000 docker-compose -f docker-compose-cli.yaml up -d

Wait, 60 seconds or so. Behind the scenes, there are transactions being sent
to the peers. Execute a ``docker ps`` to view your active containers.
You should see an output identical to the following:

.. code:: bash

  CONTAINER ID        IMAGE                                 COMMAND                  CREATED             STATUS              PORTS                                              NAMES
  b568de3fe931        dev-peer1.org2.example.com-mycc-1.0   "chaincode -peer.a..."   4 minutes ago       Up 4 minutes                                                           dev-peer1.org2.example.com-mycc-1.0
  17c1c82087e7        dev-peer0.org1.example.com-mycc-1.0   "chaincode -peer.a..."   4 minutes ago       Up 4 minutes                                                           dev-peer0.org1.example.com-mycc-1.0
  0e1c5034c47b        dev-peer0.org2.example.com-mycc-1.0   "chaincode -peer.a..."   4 minutes ago       Up 4 minutes                                                           dev-peer0.org2.example.com-mycc-1.0
  71339e7e1d38        hyperledger/fabric-peer               "peer node start -..."   5 minutes ago       Up 5 minutes        0.0.0.0:8051->7051/tcp, 0.0.0.0:8053->7053/tcp     peer1.org1.example.com
  add6113ffdcf        hyperledger/fabric-peer               "peer node start -..."   5 minutes ago       Up 5 minutes        0.0.0.0:10051->7051/tcp, 0.0.0.0:10053->7053/tcp   peer1.org2.example.com
  689396c0e520        hyperledger/fabric-peer               "peer node start -..."   5 minutes ago       Up 5 minutes        0.0.0.0:7051->7051/tcp, 0.0.0.0:7053->7053/tcp     peer0.org1.example.com
  65424407a653        hyperledger/fabric-orderer            "orderer"                5 minutes ago       Up 5 minutes        0.0.0.0:7050->7050/tcp                             orderer.example.com
  ce14853db660        hyperledger/fabric-peer               "peer node start -..."   5 minutes ago       Up 5 minutes        0.0.0.0:9051->7051/tcp, 0.0.0.0:9053->7053/tcp     peer0.org2.example.com

If you set a moderately high ``TIMEOUT`` value, then you will see your cli
container as well.

What's happening behind the scenes?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

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

-  A chaincode - **chaincode_example02** - is installed on ``peer0.org1.example.com`` and
   ``peer0.org2.example.com``

-  The chaincode is then "instantiated" on ``peer0.org2.example.com``. Instantiation
   adds the chaincode to the channel, starts the container for the target peer,
   and initializes the key value pairs associated with the chaincode.  The initial
   values for this example are ["a","100" "b","200"]. This "instantiation" results
   in a container by the name of ``dev-peer0.org2.example.com-mycc-1.0`` starting.

-  The instantiation also passes in an argument for the endorsement
   policy. The policy is defined as
   ``-P "OR    ('Org1MSP.member','Org2MSP.member')"``, meaning that any
   transaction must be endorsed by a peer tied to Org1 or Org2.

-  A query against the value of "a" is issued to ``peer0.org1.example.com``. The
   chaincode was previously installed on ``peer0.org1.example.com``, so this will start
   a container for Org1 peer0 by the name of ``dev-peer0.org1.example.com-mycc-1.0``. The result
   of the query is also returned. No write operations have occurred, so
   a query against "a" will still return a value of "100".

-  An invoke is sent to ``peer0.org1.example.com`` to move "10" from "a" to "b"

-  The chaincode is then installed on ``peer1.org2.example.com``

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
blocks, as well as a state database to maintain current fabric state.
This includes those peers that do not have chaincode installed on them
(like ``peer1.org1.example.com`` in the above example) . Finally, the chaincode is accessible
after it is installed (like ``peer1.org2.example.com`` in the above example) because it
has already been instantiated.

How do I see these transactions?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Check the logs for the CLI docker container.

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
  ===================== Query on PEER3 on channel 'mychannel' is successful =====================

  ===================== All GOOD, End-2-End execution completed =====================


   _____   _   _   ____            _____   ____    _____
  | ____| | \ | | |  _ \          | ____| |___ \  | ____|
  |  _|   |  \| | | | | |  _____  |  _|     __) | |  _|
  | |___  | |\  | | |_| | |_____| | |___   / __/  | |___
  |_____| |_| \_| |____/          |_____| |_____| |_____|

How can I see the chaincode logs?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Inspect the individual chaincode containers to see the separate
transactions executed against each container. Here is the combined
output from each container:

.. code:: bash

    $ docker logs dev-peer0.org2.example.com-mycc-1.0
    04:30:45.947 [BCCSP_FACTORY] DEBU : Initialize BCCSP [SW]
    ex02 Init
    Aval = 100, Bval = 200

    $ docker logs dev-peer0.org1.example.com-mycc-1.0
    04:31:10.569 [BCCSP_FACTORY] DEBU : Initialize BCCSP [SW]
    ex02 Invoke
    Query Response:{"Name":"a","Amount":"100"}
    ex02 Invoke
    Aval = 90, Bval = 210

    $ docker logs dev-peer1.org2.example.com-mycc-1.0
    04:31:30.420 [BCCSP_FACTORY] DEBU : Initialize BCCSP [SW]
    ex02 Invoke
    Query Response:{"Name":"a","Amount":"90"}

All in one
^^^^^^^^^^

You can also generate the artifacts and crypto, and drive the tests using a single
shell script. The ``cryptogen``, ``configtxgen`` and ``docker-compose`` commands are
embedded in the script.  If you choose not to supply a channel ID, then the
script will use a default name of ``mychannel``.  The cli timeout parameter
is an optional value; if you choose not to set it, then your cli container
will exit upon conclusion of the script.

.. code:: bash

        ./network_setup.sh up

OR

.. code:: bash

        ./network_setup.sh up <channel-ID> <timeout-value>

Understanding the docker-compose topology
-----------------------------------------

The ``e2e_cli`` folder offers us two flavors of docker-compose files, both of which
are extended from the ``docker-compose-base.yaml``.  Our first flavor,
``docker-compose-cli.yaml``, provides us with a CLI container, along with an orderer,
four peers, and the optional couchDB containers.  We use this docker-compose for
the entirety of the instructions on this page.

The second flavor, ``docker-compose-e2e.yaml``, is constructed to run end-to-end tests
using the Node.js SDK.  Aside from functioning with the SDK, its primary differentiation
is that there are containers for the fabric-ca servers.  As a result, we are able
to send REST calls to the organizational CAs for user registration and enrollment.

If you want to use the ``docker-compose-e2e.yaml`` without first running the
**All in one** script, then we  will need to make four slight modifications.
We need to point to the private keys for our Organization's CA's.  You can locate
these values in your crypto-config folder.  For example, to locate the private
key for Org1 we would follow this path - ``crypto-config/peerOrganizations/org1.example.com/ca/``.
The private key is a long hash value followed by ``_sk``.  The path for Org2
would be - ``crypto-config/peerOrganizations/org2.example.com/ca/``.

In the ``docker-compose-e2e.yaml`` update the FABRIC_CA_SERVER_TLS_KEYFILE variable
for ca0 and ca1.  You also need to edit the path that is provided in the command
to start the ca server.  You are providing the same private key twice for each
CA container.

Manually exercise the commands
------------------------------

Exit the currently-running containers:

.. code:: bash

    docker rm -f $(docker ps -aq)

Execute a ``docker images`` command in your terminal to view the
chaincode images. They will look similar to the following:

.. code:: bash

  REPOSITORY                            TAG                              IMAGE ID            CREATED             SIZE
  dev-peer1.org2.example.com-mycc-1.0   latest                           4bc5e9b5dd97        5 seconds ago       176 MB
  dev-peer0.org1.example.com-mycc-1.0   latest                           6f2aeb032076        22 seconds ago      176 MB
  dev-peer0.org2.example.com-mycc-1.0   latest                           509b8e393cc6        39 seconds ago      176 MB

Remove these images:

.. code:: bash

    docker rmi <IMAGE ID> <IMAGE ID> <IMAGE ID>

For example:

.. code:: bash

    docker rmi -f 4bc 6f2 509

Ensure you have the configuration artifacts. If you deleted them, run
the shell script again:

.. code:: bash

    # remember to supply a channel ID
    ./generateArtifacts.sh <channel-ID>

Modify the docker-compose file
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Open the ``docker-compose-cli.yaml`` file and comment out the command to run
``script.sh``. Navigate down to the cli container and place a ``#`` to the
left of the command. For example:

.. code:: bash

        working_dir: /opt/gopath/src/github.com/hyperledger/fabric/peer
      # command: /bin/bash -c './scripts/script.sh ${CHANNEL_NAME}; sleep $TIMEOUT'

Save the file and return to the ``/e2e_cli`` directory.

Now restart your network:

.. code:: bash

    # make sure you are in the /e2e_cli directory where your docker-compose script resides
    CHANNEL_NAME=<channel-id> TIMEOUT=<pick_a_value> docker-compose -f docker-compose-cli.yaml up -d

If you want to see the realtime logs for your network, then do not supply the ``-d`` flag.
If you let the logs stream, then you will need to open a second terminal to execute the CLI calls.

Command syntax
^^^^^^^^^^^^^^

Refer to the create and join commands in the ``script.sh`` for the exact syntax.

For the following CLI commands against `peer0.org1.example.com` to work, we need
to preface our commands with the four environment variables given below.  These
variables for ``peer0.org1.example.com`` are baked into the CLI container,
therefore we can operate without passing them.  **HOWEVER**, if you want to send
calls to other peers or the orderer, then you will need to provide these
values accordingly.  Inspect the ``docker-compose-base.yaml`` for the specific
paths:

.. code:: bash

    # Environment variables for PEER0

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    CORE_PEER_LOCALMSPID="Org1MSP"
    CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

Create channel
^^^^^^^^^^^^^^

Exec into the cli container:

.. code:: bash

    docker exec -it cli bash

If successful you should see the following:

.. code:: bash

    root@0d78bb69300d:/opt/gopath/src/github.com/hyperledger/fabric/peer#

Specify your channel name with the ``-c`` flag. Specify your channel
configuration transaction with the ``-f`` flag. In this case it is
``channel.tx``, however you can mount your own configuration transaction
with a different name.

.. code:: bash

    # the channel.tx file is mounted in the channel-artifacts directory within your cli container
    # as a result, we pass the full path for the file
    # we also pass the path for the orderer ca-cert in order to verify the TLS handshake
    # be sure to replace the $CHANNEL_NAME variable appropriately

    peer channel create -o orderer.example.com:7050 -c $CHANNEL_NAME -f ./channel-artifacts/channel.tx --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem


.. note:: You will remain in the CLI container for the remainder of
          these manual commands. You must also remember to preface all commands
          with the corresponding environment variables when targeting a peer other than
          ``peer0.org1.example.com``.

Join channel
^^^^^^^^^^^^

Join specific peers to the channel

.. code:: bash

    # By default, this joins ``peer0.org1.example.com`` only
    # the channel.block was returned by the previous command

     peer channel join -b <YOUR_CHANNEL.block>

You can make other peers join the channel as necessary by making appropriate
changes in the four environment variables.

Install chaincode onto a remote peer
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Install the sample go code onto one of the four peer nodes

.. code:: bash

    peer chaincode install -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02

Instantiate chaincode and define the endorsement policy
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Instantiate the chaincode on a peer. This will launch a chaincode
container for the targeted peer and set the endorsement policy for the
chaincode. In this snippet, we define the policy as requiring an
endorsement from one peer node that is a part of either ``Org1`` or ``Org2``.
The command is:

.. code:: bash

    # be sure to replace the $CHANNEL_NAME environment variable
    # if you did not install your chaincode with a name of mycc, then modify that argument as well

    peer chaincode instantiate -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a", "100", "b","200"]}' -P "OR ('Org1MSP.member','Org2MSP.member')"

See the `endorsement
policies <http://hyperledger-fabric.readthedocs.io/en/latest/endorsement-policies.html>`__
documentation for more details on policy implementation.

Invoke chaincode
^^^^^^^^^^^^^^^^

.. code:: bash

    # be sure to set the -C and -n flags appropriately
    peer chaincode invoke -o orderer.example.com:7050  --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem  -C $CHANNEL_NAME -n mycc -c '{"Args":["invoke","a","b","10"]}'

Make sure to wait a few seconds for the operation to complete.

Query chaincode
^^^^^^^^^^^^^^^

.. code:: bash

    # be sure to set the -C and -n flags appropriately
    peer chaincode query -C $CHANNEL_NAME -n mycc -c '{"Args":["query","a"]}'

The result of the above command should be the following:

.. code:: bash

    Query Result: 90

Feel free to start over and manipulate the key value pairs and subsequent
invocations.

Using CouchDB
-------------

The state database can be switched from the default (goleveldb) to CouchDB.
The same chaincode functions are available with CouchDB, however, there is the
added ability to perform rich and complex queries against the state database
data content contingent upon the chaincode data being modeled as JSON.

To use CouchDB instead of the default database (goleveldb), follow the same
procedure in the **Manually exercise the commands** section, except when starting
the network pass the couchdb docker-compose as well:

.. code:: bash

    # make sure you are in the /e2e_cli directory where your docker-compose script resides
    CHANNEL_NAME=<channel-id> TIMEOUT=<pick_a_value> docker-compose -f docker-compose-cli.yaml -f docker-compose-couch.yaml up -d

**chaincode_example02** should now work using CouchDB underneath.

.. note::  If you choose to implement mapping of the fabric-couchdb container
           port to a host port, please make sure you are aware of the security
           implications. Mapping of the port in a development environment makes the
           CouchDB REST API available, and allows the
           visualization of the database via the CouchDB web interface (Fauxton).
           Production environments would likely refrain from implementing port mapping in
           order to restrict outside access to the CouchDB containers.

You can use **chaincode_example02** chaincode against the CouchDB state database
using the steps outlined above, however in order to exercise the CouchDB query
capabilities you will need to use a chaincode that has data modeled as JSON,
(e.g. **marbles02**). You can locate the **marbles02** chaincode in the
``fabric/examples/chaincode/go`` directory.

We will follow the same process to create and join the channel as outlined in the
**Manually exercise the commands** section above.  Once you have joined your
peer(s) to the channel, use the following steps to interact with the **marbles02**
chaincode:

-  Install and instantiate the chaincode on ``peer0.org1.example.com``:

.. code:: bash

       # be sure to modify the $CHANNEL_NAME variable accordingly for the instantiate command

       peer chaincode install -o orderer.example.com:7050 -n marbles -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/marbles02
       peer chaincode instantiate -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/marbles02 -c '{"Args":["init"]}' -P "OR ('Org0MSP.member','Org1MSP.member')"

-  Create some marbles and move them around:

.. code:: bash

        # be sure to modify the $CHANNEL_NAME variable accordingly

        peer chaincode invoke -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -c '{"Args":["initMarble","marble1","blue","35","tom"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -c '{"Args":["initMarble","marble2","red","50","tom"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -c '{"Args":["initMarble","marble3","blue","70","tom"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -c '{"Args":["transferMarble","marble2","jerry"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -c '{"Args":["transferMarblesBasedOnColor","blue","jerry"]}'
        peer chaincode invoke -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/cacerts/ca.example.com-cert.pem -C $CHANNEL_NAME -n marbles -c '{"Args":["delete","marble1"]}'


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

Troubleshooting
---------------

-  It's recommended to start your network fresh.  Use the following command
   to remove artifacts, crypto, containers and chaincode images:

.. code:: bash

      ./network_setup.sh down

-  If you see docker errors, first check your version (should be 1.12 or above),
   and then try restarting your docker process.  Problems with Docker are
   oftentimes not immediately recognizable.  For example, you may see errors
   resulting from an inability to access crypto material mounted within a
   container.

-  If they persist remove your images and start from scratch:

.. code:: bash

       make clean
       make docker

-  If you see the below error:

.. code:: bash

       Error: Error endorsing chaincode: rpc error: code = 2 desc = Error installing chaincode code mycc:1.0(chaincode /var/hyperledger/production/chaincodes/mycc.1.0 exits)

You likely have chaincode images (e.g. ``dev-peer1.org2.example.com-mycc-1.0`` or
``dev-peer0.org1.example.com-mycc-1.0``) from prior runs. Remove them and try
again.

.. code:: bash

    docker rmi -f $(docker images | grep peer[0-9]-peer[0-9] | awk '{print $3}')

- If you see something similar to the following:

.. code:: bash

      Error connecting: rpc error: code = 14 desc = grpc: RPC failed fast due to transport failure
      Error: rpc error: code = 14 desc = grpc: RPC failed fast due to transport failure

Make sure you pointed to the correct binaries in the release folder when
generating the artifacts, and that your backend is running against "beta" images
or compiled images from the current master branch.

If you see the below error:

.. code:: bash

  [configtx/tool/localconfig] Load -> CRIT 002 Error reading configuration: Unsupported Config Type ""
  panic: Error reading configuration: Unsupported Config Type ""

Then you did not set the ``FABRIC_CFG_PATH`` environment variable properly.  The
configtxgen tool needs this variable in order to locate the configtx.yaml.  Go
back and recreate your channel artifacts.

-  To cleanup the network, use the ``down`` option:

.. code:: bash

       ./network_setup.sh down

- If you continue to see errors, share your logs on the **# fabric-questions**
  channel on `Hyperledger Rocket Chat <https://chat.hyperledger.org/home>`__.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
