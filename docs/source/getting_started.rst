Getting Started
===============

The getting started scenario provisions a sample Fabric network consisting of
two organizations, each maintaining two peers, and a "solo" ordering service.
The required crypto material (x509 certs) for the network entities has been
pre-generated and is baked into the appropriate directories and configuration
files.  You do not need to modify any of these certificates.  The ``examples/e2e_cli``
folder contains the docker compose files and supporting scripts that you will use
to create and test the sample network.

This section also demonstrates usage of a configuration generation tool,
``configtxgen``, to generate the network's configuration.  Upon conclusion of
this section, you will have a fully functional transactional network.  Let's get going...

Prerequisites
-------------

Complete the following to install the Fabric source code and build the ``configtxgen``
tool:

-  Follow the steps for setting up a :doc:`development
   environment <dev-setup/devenv>` and ensure that you have properly set the
   $GOPATH environment variable.  This process will load the various software
   dependencies onto your machine.

-  Clone the Fabric code base.

.. code:: bash

       git clone https://github.com/hyperledger/fabric.git

- Build the ``configtxgen`` tool.

If you are running Linux, open a terminal and execute the following from your
``fabric`` directory:

.. code:: bash

    cd $GOPATH/src/github.com/hyperledger/fabric
    make configtxgen
    # if you see an error stating - 'ltdl.h' file not found
    sudo apt install libtool libltdl-dev
    # then run the make again
    make configtxgen

If you are running OSX, first install `Xcode <https://developer.apple.com/xcode/>`__ version 8 or above.
Next, open a terminal and execute the following from your ``fabric`` directory:

.. code:: bash

   # install Homebrew
   /usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
   # add gnu-tar
   brew install gnu-tar --with-default-names
   # add libtool
   brew install libtool
   # make the configtxgen
   make configtxgen

A successful build will result in the following output:

.. code:: bash

   build/bin/configtxgen
   CGO_CFLAGS=" " GOBIN=/Users/johndoe/work/src/github.com/hyperledger/fabric/build/bin go install -ldflags "-X github.com/hyperledger/fabric/common/metadata.Version=1.0.0-snapshot-8d3275f -X github.com/hyperledger/fabric/common /metadata.BaseVersion=0.3.0 -X github.com/hyperledger/fabric/common/metadata.BaseDockerLabel=org.hyperledger.fabric"       github.com/hyperledger/fabric/common/configtx/tool/configtxgen
   Binary available as build/bin/configtxgen``

The executable is placed in the ``build/bin/configtxgen`` directory of the Fabric
source tree.

All in one with docker images
-----------------------------

In order to expedite the process, we provide a script that performs all the heavy
lifting.  This script will generate the configuration artifacts, spin up a
local network, and drive the chaincode tests.  Navigate to the ``examples/e2e_cli``
directory.

First, pull the images from Docker Hub for your Fabric network:

.. code:: bash

        # make the script an executable
        chmod +x download-dockerimages.sh
        # run the script
        ./download-dockerimages.sh

This process will take a few minutes...

You should see the exact output in your terminal upon completion of the script:

.. code:: bash

           ===> List out hyperledger docker images
           hyperledger/fabric-ca          latest               35311d8617b4        7 days ago          240 MB
           hyperledger/fabric-ca          x86_64-1.0.0-alpha   35311d8617b4        7 days ago          240 MB
           hyperledger/fabric-couchdb     latest               f3ce31e25872        7 days ago          1.51 GB
           hyperledger/fabric-couchdb     x86_64-1.0.0-alpha   f3ce31e25872        7 days ago          1.51 GB
           hyperledger/fabric-kafka       latest               589dad0b93fc        7 days ago          1.3 GB
           hyperledger/fabric-kafka       x86_64-1.0.0-alpha   589dad0b93fc        7 days ago          1.3 GB
           hyperledger/fabric-zookeeper   latest               9a51f5be29c1        7 days ago          1.31 GB
           hyperledger/fabric-zookeeper   x86_64-1.0.0-alpha   9a51f5be29c1        7 days ago          1.31 GB
           hyperledger/fabric-orderer     latest               5685fd77ab7c        7 days ago          182 MB
           hyperledger/fabric-orderer     x86_64-1.0.0-alpha   5685fd77ab7c        7 days ago          182 MB
           hyperledger/fabric-peer        latest               784c5d41ac1d        7 days ago          184 MB
           hyperledger/fabric-peer        x86_64-1.0.0-alpha   784c5d41ac1d        7 days ago          184 MB
           hyperledger/fabric-javaenv     latest               a08f85d8f0a9        7 days ago          1.42 GB
           hyperledger/fabric-javaenv     x86_64-1.0.0-alpha   a08f85d8f0a9        7 days ago          1.42 GB
           hyperledger/fabric-ccenv       latest               91792014b61f        7 days ago          1.29 GB
           hyperledger/fabric-ccenv       x86_64-1.0.0-alpha   91792014b61f        7 days ago          1.29 GB


Now run the all-in-one script:

.. code:: bash

        ./network_setup.sh up <channel-ID>

If you choose not to pass the ``channel-ID`` parameter, then your channel name
will default to ``mychannel``.  In your terminal you will see the chaincode logs
for the various commands being executed within the script.

When the script completes successfully, you should see the following message
in your terminal:

.. code:: bash

  ===================== Query on PEER3 on channel 'mychannel' is successful =====================

  ===================== All GOOD, End-2-End execution completed =====================

At this point your network is up and running and the tests have completed
successfully.  Continue through this document for more advanced network
operations.

Clean up
^^^^^^^^

Shut down your network:

.. code:: bash

        # make sure you're in the e2e_cli directory
        docker rm -f $(docker ps -aq)

Next, execute a ``docker images`` command in your terminal to view the
**chaincode** images.  They will look similar to the following:

.. code:: bash

  REPOSITORY                     TAG                  IMAGE ID            CREATED             SIZE
  dev-peer3-mycc-1.0             latest               13f6c8b042c6        5 minutes ago       176 MB
  dev-peer0-mycc-1.0             latest               e27456b2bd92        5 minutes ago       176 MB
  dev-peer2-mycc-1.0             latest               111098a7c98c        5 minutes ago       176 MB

Remove these images:

  .. code:: bash

      docker rmi <IMAGE ID> <IMAGE ID> <IMAGE ID>

For example:

  .. code:: bash

      docker rmi -f 13f e27 111

Finally, delete the config artifacts.  Navigate to the ``crypto/orderer``
directory and remove ``orderer.block`` and ``channel.tx``.  You can rerun the
all-in-one script or continue reading for a deeper dive on configuration
transactions and chaincode commands.

Configuration Transaction Generator
-----------------------------------

The :doc:`configtxgen tool <configtxgen>` is used to create two artifacts:
Orderer **bootstrap block** and Fabric **channel configuration transaction**.

The orderer block is the genesis block for the ordering service, and the
channel transaction file is broadcast to the orderer at channel creation
time.

The ``configtx.yaml`` contains the definitions for the sample network and presents
the topology of the network components - two members (Org0 & Org1), each managing
and maintaining two peers.  It also points to the filesystem location
where the cryptographic material for each network entity is stored.   This
directory, ``crypto``, contains the admin certs, ca certs, signing certs, and
private key for each entity.

For ease of use, we provide a script - ``generateCfgTrx.sh`` - that orchestrates
the process of running ``configtxgen``.  The script will output our two
configuration artifacts - ``orderer.block`` and ``channel.tx``.  If you ran the
all-in-one script then these artifacts have already been created.  Navigate to the
``crypto/orderer`` directory and ensure they have been deleted.

Run the ``generateCfgTrx.sh`` shell script
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Make sure you are in the ``e2e_cli`` directory:

.. code:: bash

   cd $GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli

The ``generateCfgTrx.sh`` script takes an optional <channel-ID> argument. If none
is provided, then a channel will be generated with a default channel-ID of ``mychannel``.

.. code:: bash

    # as mentioned above, the <channel-ID> parm is optional
    ./generateCfgTrx.sh <channel-ID>

After you run the shell script, you should see an output in your
terminal similar to the following:

.. code:: bash

    2017/02/28 17:01:52 Generating new channel configtx
    2017/02/28 17:01:52 Creating no-op MSP instance
    2017/02/28 17:01:52 Obtaining default signing identity
    2017/02/28 17:01:52 Creating no-op signing identity instance
    2017/02/28 17:01:52 Serializing identity
    2017/02/28 17:01:52 signing message
    2017/02/28 17:01:52 signing message
    2017/02/28 17:01:52 Writing new channel tx

The script generates two files - ``orderer.block`` and ``channel.tx`` and outputs
them into the ``crypto/orderer directory``.

``orderer.block`` is the genesis block for the ordering service.  ``channel.tx``
contains the configuration information for the new channel.  As mentioned
earlier, both are derived from ``configtx.yaml`` and contain data such as crypto
material and network endpoint information.

.. note:: You also have the option to manually exercise the embedded commands within
          the ``generateCfgTrx.sh`` script.  Open the script and inspect the syntax for the
          two commands.  If you do elect to pursue this route, you must
          replace the default ``configtx.yaml`` in the fabric source tree.  Navigate to the
          ``common/configtx/tool`` directory and replace the ``configtx.yaml`` file with
          the supplied yaml file in the ``e2e_cli`` directory. Then return to the ``fabric``
          directory to execute the commands (you will run these manual commands from ``fabric``,
          NOT from ``e2e_cli``).  Be sure to remove any existing artifacts from
          previous runs of the ``generateCfgTrx.sh`` script before commencing.

Start the network
-----------------

We will use docker-compose to launch our network with the images that we pulled
earlier.  If you have not yet pulled the Fabric images, return to the **All in one**
section and follow the instructions to retrieve the images.

Embedded within the docker-compose file is a script, ``script.sh``, which joins
the peers to a channel and sends read/write requests to the peers.  As a result,
you are able to see the transaction flow without manually submitting the commands.
Skip down to the **Manually execute the transactions** section if you don't want
to leverage the script.

Make sure you are in the ``e2e_cli`` directory. Then use docker-compose
to spawn the network entities and kick off the embedded script.

.. code:: bash

    CHANNEL_NAME=<channel-id> docker-compose up -d

If you created a unique channel name, be sure to pass in that parameter.
Otherwise, pass in the default ``mychannel`` string.  For example:

.. code:: bash

    CHANNEL_NAME=mychannel docker-compose up -d

Wait, 30 seconds. Behind the scenes, there are transactions being sent
to the peers. Execute a ``docker ps`` to view your active containers.
You should see an output identical to the following:

.. code:: bash

    vagrant@hyperledger-devenv:v0.3.0-4eec836:/opt/gopath/src/github.com/hyperledger/fabric/examples/e2e_cli$ docker ps
    CONTAINER ID        IMAGE                        COMMAND                  CREATED              STATUS              PORTS                                              NAMES
    45e3e114f7a2        dev-peer3-mycc-1.0           "chaincode -peer.a..."   4 seconds ago        Up 4 seconds                                                           dev-peer3-mycc-1.0
    5970f740ad2b        dev-peer0-mycc-1.0           "chaincode -peer.a..."   24 seconds ago       Up 23 seconds                                                          dev-peer0-mycc-1.0
    b84808d66e99        dev-peer2-mycc-1.0           "chaincode -peer.a..."   48 seconds ago       Up 47 seconds                                                          dev-peer2-mycc-1.0
    16d7d94c8773        hyperledger/fabric-peer      "peer node start -..."   About a minute ago   Up About a minute   0.0.0.0:10051->7051/tcp, 0.0.0.0:10053->7053/tcp   peer3
    3561a99e35e6        hyperledger/fabric-peer      "peer node start -..."   About a minute ago   Up About a minute   0.0.0.0:9051->7051/tcp, 0.0.0.0:9053->7053/tcp     peer2
    0baad3047d92        hyperledger/fabric-peer      "peer node start -..."   About a minute ago   Up About a minute   0.0.0.0:8051->7051/tcp, 0.0.0.0:8053->7053/tcp     peer1
    1216896b7b4f        hyperledger/fabric-peer      "peer node start -..."   About a minute ago   Up About a minute   0.0.0.0:7051->7051/tcp, 0.0.0.0:7053->7053/tcp     peer0
    155ff8747b4d        hyperledger/fabric-orderer   "orderer"                About a minute ago   Up About a minute   0.0.0.0:7050->7050/tcp                             orderer

What happened behind the scenes?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

-  A script - ``script.sh`` - is baked inside the CLI container. The
   script drives the ``createChannel`` command against the default
   ``mychannel`` name.  The ``createChannel`` command uses the ``channel.tx``
   artifact created previously with the configtxgen tool.

-  The output of ``createChannel`` is a genesis block -
   ``mychannel.block`` - which is stored on the file system.

-  The ``joinChannel`` command is exercised for all four peers who will
   pass in the genesis block - ``mychannel.block``.

-  Now we have a channel consisting of four peers, and two
   organizations.

-  ``PEER0`` and ``PEER3`` belong to Org0; ``PEER1`` and ``PEER2``
   belong to Org1

-  Recall that these relationships are defined in the ``configtx.yaml``

-  A chaincode - **chaincode_example02** - is installed on ``PEER0`` and
   ``PEER2``

-  The chaincode is then "instantiated" on ``PEER2``. Instantiate simply
   refers to starting the container and initializing the key value pairs
   associated with the chaincode. The initial values for this example
   are ["a","100" "b","200"]. This "instantiation" results in a container
   by the name of ``dev-peer2-mycc-1.0`` starting.  Notice that this container
   is specific to ``PEER2``.

-  The instantiation also passes in an argument for the endorsement
   policy. The policy is defined as
   ``-P "OR ('Org0MSP.member','Org1MSP.member')"``, meaning that any
   transaction must be endorsed by a peer tied to Org0 or Org1.

-  A query against the value of "a" is issued to ``PEER0``. The
   chaincode was previously installed on ``PEER0``, so this will start
   another container by the name of ``dev-peer0-mycc-1.0``. The result
   of the query is also returned. No write operations have occurred, so
   a query against "a" will still return a value of "100"

-  An invoke is sent to ``PEER0`` to move "10" from "a" to "b"

-  The chaincode is installed on ``PEER3``

-  A query is sent to ``PEER3`` for the value of "a". This starts a
   third chaincode container by the name of ``dev-peer3-mycc-1.0``. A
   value of 90 is returned, correctly reflecting the previous
   transaction during which the value for key "a" was modified by 10.

What does this demonstrate?
^^^^^^^^^^^^^^^^^^^^^^^^^^^

Chaincode **MUST** be installed on a peer in order for it to
successfully perform read/write operations against the ledger.
Furthermore, a chaincode container is not started for a peer until a
read/write operation is performed against that chaincode (e.g. query for
the value of "a"). The transaction causes the container to start. Also,
all peers in a channel maintain an exact copy of the ledger which
comprises the blockchain to store the immutable, sequenced record in
blocks, as well as a state database to maintain current fabric state.
This includes those peers that do not have chaincode installed on them
(like ``Peer3`` in the above example) . Finally, the chaincode is accessible
after it is installed (like ``Peer3`` in the above example) because it
already has been instantiated.

How do I see these transactions?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Check the logs for the CLI docker container.

.. code:: bash

    docker logs -f cli

You should see the following output:

.. code:: bash

    2017-02-28 04:31:20.841 UTC [logging] InitFromViper -> DEBU 001 Setting default logging level to DEBUG for command 'chaincode'
    2017-02-28 04:31:20.842 UTC [msp] GetLocalMSP -> DEBU 002 Returning existing local MSP
    2017-02-28 04:31:20.842 UTC [msp] GetDefaultSigningIdentity -> DEBU 003 Obtaining default signing identity
    2017-02-28 04:31:20.843 UTC [msp] Sign -> DEBU 004 Sign: plaintext: 0A8F050A59080322096D796368616E6E...6D7963631A0A0A0571756572790A0161
    2017-02-28 04:31:20.843 UTC [msp] Sign -> DEBU 005 Sign: digest: 52F1A41B7B0B08CF3FC94D9D7E916AC4C01C54399E71BC81D551B97F5619AB54
    Query Result: 90
    2017-02-28 04:31:30.425 UTC [main] main -> INFO 006 Exiting.....
    ===================== Query on chaincode on PEER3 on channel 'mychannel' is successful =====================

    ===================== All GOOD, End-2-End execution completed =====================

You also have the option of viewing the logs in real time.  You will need two
terminals for this.  First, kill your docker containers:

.. code:: bash

   docker rm -f $(docker ps -aq)

In the first terminal launch your docker-compose script:

.. code:: bash

   # add the appropriate CHANNEL_NAME parm
   CHANNEL_NAME=<channel-id> docker-compose up -d

In your second terminal view the logs:

.. code:: bash

    docker logs -f cli

This will show you the live output for the transactions being driven by ``script.sh``.

How can I see the chaincode logs?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Inspect the individual chaincode containers to see the separate
transactions executed against each container. Here is the combined
output from each container:

.. code:: bash

    $ docker logs dev-peer2-mycc-1.0
    04:30:45.947 [BCCSP_FACTORY] DEBU : Initialize BCCSP [SW]
    ex02 Init
    Aval = 100, Bval = 200

    $ docker logs dev-peer0-mycc-1.0
    04:31:10.569 [BCCSP_FACTORY] DEBU : Initialize BCCSP [SW]
    ex02 Invoke
    Query Response:{"Name":"a","Amount":"100"}
    ex02 Invoke
    Aval = 90, Bval = 210

    $ docker logs dev-peer3-mycc-1.0
    04:31:30.420 [BCCSP_FACTORY] DEBU : Initialize BCCSP [SW]
    ex02 Invoke
    Query Response:{"Name":"a","Amount":"90"}


Manually execute the transactions
---------------------------------

The following section caters towards a more advanced chaincode developer.  It involves
wide usage of global environment variables and requires exact syntax in order
for commands to work properly.  Fully-functional sample commands are provided,
however it is still recommended that you have a fundamental understanding of
Fabric before continuing with this section.

From your current working directory, kill your containers:

.. code:: bash

        docker rm -f $(docker ps -aq)

Next, execute a ``docker images`` command in your terminal to view the
**chaincode** images.  They will look similar to the following:

.. code:: bash

  REPOSITORY                     TAG                  IMAGE ID            CREATED             SIZE
  dev-peer3-mycc-1.0             latest               13f6c8b042c6        5 minutes ago       176 MB
  dev-peer0-mycc-1.0             latest               e27456b2bd92        5 minutes ago       176 MB
  dev-peer2-mycc-1.0             latest               111098a7c98c        5 minutes ago       176 MB

  Remove these images:

  .. code:: bash

      docker rmi <IMAGE ID> <IMAGE ID> <IMAGE ID>

  For example:

  .. code:: bash

      docker rmi -f 13f e27 111

Ensure you have the configuration artifacts. If you deleted them, run
the shell script again:

.. code:: bash

    ./generateCfgTrx.sh <channel-ID>

Or manually generate the artifacts using the commands within the script.

Modify the docker-compose file
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Open the docker-compose file and comment out the command to run
``script.sh``. Navigate down to the cli image and place a ``#`` to the
left of the command. For example:

.. code:: bash

        working_dir: /opt/gopath/src/github.com/hyperledger/fabric/peer
      # command: /bin/bash -c './scripts/script.sh ${CHANNEL_NAME}'

Save the file.  Now restart your network:

.. code:: bash

    # make sure you are in the e2e_cli directory where you docker-compose script resides
    # add the appropriate CHANNEL_NAME parm
    CHANNEL_NAME=<channel-id> docker-compose up -d

Command syntax
^^^^^^^^^^^^^^

Refer to the create and join commands in the ``script.sh``.  The file is
located in the ``scripts`` directory.

For the following cli commands against ``PEER0`` to work, you need to set the
values for the four global environment variables, given below. Please make sure to override
the values accordingly when calling commands against other peers and the
orderer.

.. code:: bash

    # Environment variables for PEER0
    CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peer/peer0/localMspConfig
    CORE_PEER_ADDRESS=peer0:7051
    CORE_PEER_LOCALMSPID="Org0MSP"
    CORE_PEER_TLS_ROOTCERT_FILE=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peer/peer0/localMspConfig/cacerts/peerOrg0.pem

These environment variables for each peer are defined in the supplied
docker-compose file.

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
``channeltx``, however you can mount your own configuration transaction
with a different name.

.. code:: bash

    # the channel.tx and orderer.block are mounted in the crypto/orderer directory within your cli container
    # as a result, we pass the full path for the file
     peer channel create -o orderer0:7050 -c mychannel -f crypto/orderer/channel.tx --tls $CORE_PEER_TLS_ENABLED --cafile $GOPATH/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem

Since the ``channel create`` command runs against the orderer, we need to override the
four environment variables set before. So the above command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig CORE_PEER_LOCALMSPID="OrdererMSP" peer channel create -o orderer0:7050 -c mychannel -f crypto/orderer/channel.tx --tls $CORE_PEER_TLS_ENABLED --cafile $GOPATH/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem


.. note:: You will remain in the CLI container for the remainder of
          these manual commands. You must also remember to preface all commands
          with the corresponding environment variables for targeting a peer other than
          ``PEER0``.

Join channel
^^^^^^^^^^^^

Join specific peers to the channel

.. code:: bash

    # By default, this joins PEER0 only
    # the mychannel.block is also mounted in the crypto/orderer directory
     peer channel join -b mychannel.block

This full command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peer/peer0/localMspConfig CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peer/peer0/localMspConfig/cacerts/peerOrg0.pem peer channel join -b mychannel.block

You can make other peers join the channel as necessary by making appropriate
changes in the four environment variables.

Install chaincode onto a remote peer
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Install the sample go code onto one of the four peer nodes

.. code:: bash

    # remember to preface this command with the global environment variables for the appropriate peer
    peer chaincode install -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02

Instantiate chaincode and define the endorsement policy
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Instantiate the chaincode on a peer. This will launch a chaincode
container for the targeted peer and set the endorsement policy for the
chaincode. In this snippet, we define the policy as requiring an
endorsement from one peer node that is a part of either ``Org0`` or ``Org1``.
The command is:

.. code:: bash

    # remember to preface this command with the global environment variables for the appropriate peer
    # remember to pass in the correct string for the -C argument.  The default is mychannel
    peer chaincode instantiate -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile $GOPATH/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a", "100", "b","200"]}' -P "OR ('Org0MSP.member','Org1MSP.member')"

See the `endorsement
policies <endorsement-policies>` documentation for more details on policy implementation.

Invoke chaincode
^^^^^^^^^^^^^^^^

.. code:: bash

    # remember to preface this command with the global environment variables for the appropriate peer
    peer chaincode invoke -o orderer0:7050  --tls $CORE_PEER_TLS_ENABLED --cafile $GOPATH/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem  -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

**NOTE**: Make sure to wait a few seconds for the operation to complete.

Query chaincode
^^^^^^^^^^^^^^^

.. code:: bash

    # remember to preface this command with the global environment variables for the appropriate peer
    peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

The result of the above command should be as below:

.. code:: bash

    Query Result: 90

Manually generate images
------------------------

Fabric developers can elect to create the images against the latest iteration
of the code base.  This is a useful method for testing new features that have
not yet been baked into the published images.

-  Make the peer and orderer images.

.. code:: bash

     # make sure you are in the fabric directory
     # if you are unable to generate the images natively, you may need to be in a vagrant environment
     make peer-docker orderer-docker

Execute a ``docker images`` command in your terminal. If the images
compiled successfully, you should see an output similar to the
following:

.. code:: bash

               vagrant@hyperledger-devenv:v0.3.0-4eec836:/opt/gopath/src/github.com/hyperledger/fabric$ docker images
               REPOSITORY                     TAG                             IMAGE ID            CREATED             SIZE
               hyperledger/fabric-orderer     latest                          264e45897bfb        10 minutes ago      180 MB
               hyperledger/fabric-orderer     x86_64-0.7.0-snapshot-a0d032b   264e45897bfb        10 minutes ago      180 MB
               hyperledger/fabric-peer        latest                          b3d44cff07c6        10 minutes ago      184 MB
               hyperledger/fabric-peer        x86_64-0.7.0-snapshot-a0d032b   b3d44cff07c6        10 minutes ago      184 MB
               hyperledger/fabric-javaenv     latest                          6e2a2adb998a        10 minutes ago      1.42 GB
               hyperledger/fabric-javaenv     x86_64-0.7.0-snapshot-a0d032b   6e2a2adb998a        10 minutes ago      1.42 GB
               hyperledger/fabric-ccenv       latest                          0ce0e7dc043f        12 minutes ago      1.29 GB
               hyperledger/fabric-ccenv       x86_64-0.7.0-snapshot-a0d032b   0ce0e7dc043f        12 minutes ago      1.29 GB
               hyperledger/fabric-baseimage   x86_64-0.3.0                    f4751a503f02        4 weeks ago         1.27 GB
               hyperledger/fabric-baseos      x86_64-0.3.0                    c3a4cf3b3350        4 weeks ago         161 MB


Use the native binaries
-------------------------------------------------

Similar to the previous two sections, this is catered towards advanced developers
with a working understanding of the Fabric codebase.

Open your vagrant environment:

.. code:: bash

    cd $GOPATH/src/github.com/hyperledger/fabric/devenv

.. code:: bash

    # you may have to first start your VM with vagrant up
    vagrant ssh

From the ``fabric`` directory issue the following commands to
build the peer and orderer executables:

.. code:: bash

    make clean
    make native

You will also need the ``ccenv`` image. From the ``fabric`` directory:

.. code:: bash

    make peer-docker

Next, open two more terminals and start your vagrant environment in
each. You should now have a total of three terminals, all within
vagrant.

Before starting, make sure to clear your ledger folder
``/var/hyperledger/``. You will want to do this after each run to avoid
errors and duplication.

.. code:: bash

    rm -rf /var/hyperledger/*

**Vagrant window 1**

Use the ``configtxgen`` tool to create the orderer genesis block:

.. code:: bash

    configtxgen -profile SampleSingleMSPSolo -outputBlock orderer.block

**Vagrant window 2**

Start the orderer with the genesis block you just generated:

.. code:: bash

    ORDERER_GENERAL_GENESISMETHOD=file ORDERER_GENERAL_GENESISFILE=./orderer.block orderer

**Vagrant window 1**

Create the channel configuration transaction:

.. code:: bash

    configtxgen -profile SampleSingleMSPSolo -outputCreateChannelTx channel.tx -channelID <channel-ID>

This will generate a ``channel.tx`` file in your current directory

**Vagrant window 3**

Start the peer in "chainless" mode

.. code:: bash

    peer node start --peer-defaultchain=false

**Note**: Use Vagrant window 1 for the remainder of commands

Create channel
^^^^^^^^^^^^^^

Ask peer to create a channel with the configuration parameters in
``channel.tx``

.. code:: bash

    peer channel create -o 127.0.0.1:7050 -c mychannel -f channel.tx

This will return a channel genesis block - ``mychannel.block`` - in your
current directory.

Join channel
^^^^^^^^^^^^

Ask peer to join the channel by passing in the channel genesis block:

.. code:: bash

    peer channel join -b mychannel.block

Install
^^^^^^^

Install chaincode on the peer:

.. code:: bash

    peer chaincode install -o 127.0.0.1:7050 -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02

Make sure the chaincode is in the filesystem:

.. code:: bash

    ls /var/hyperledger/production/chaincodes

You should see ``mycc.1.0``

Instantiate
^^^^^^^^^^^

Instantiate the chaincode:

.. code:: bash

    peer chaincode instantiate -o 127.0.0.1:7050 -C mychannel -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a", "100", "b","200"]}'

Check your active containers:

.. code:: bash

    docker ps

If the chaincode container started successfully, you should see:

.. code:: bash

    CONTAINER ID        IMAGE               COMMAND                  CREATED             STATUS              PORTS               NAMES
    bd9c6bda7560        dev-jdoe-mycc-1.0   "chaincode -peer.a..."   5 seconds ago       Up 5 seconds                            dev-jdoe-mycc-1.0

Invoke
^^^^^^

Issue an invoke to move "10" from "a" to "b":

.. code:: bash

    peer chaincode invoke -o 127.0.0.1:7050 -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

Wait a few seconds for the operation to complete

Query
^^^^^

Query for the value of "a":

.. code:: bash

    # this should return 90
    peer chaincode query -o 127.0.0.1:7050 -C mychannel -n mycc -c '{"Args":["query","a"]}'

Don't forget to clear ledger folder ``/var/hyperledger/`` after each
run!

.. code:: bash

    rm -rf /var/hyperledger/*

Using CouchDB
-------------

The state database can be switched from the default (goleveldb) to CouchDB.
The same chaincode functions are available with CouchDB, however, there is the
added ability to perform rich and complex queries against the state database
data content contingent upon the chaincode data being modeled as JSON.

To use CouchDB instead of the default database (goleveldb), follow the same
procedure in the **Prerequisites** section, and additionally perform the
following two steps to enable the CouchDB containers and associate each peer
container with a CouchDB container:

-  Make the CouchDB image.

.. code:: bash

       # make sure you are in the fabric directory
       make couchdb

-  Open the ``fabric/examples/e2e_cli/docker-compose.yaml`` and un-comment
   all commented statements relating to CouchDB containers and peer container
   use of CouchDB. These instructions are are also outlined in the
   same ``docker-compose.yaml`` file. Search the file for 'couchdb' (case insensitive) references.

**chaincode_example02** should now work using CouchDB underneath.

.. note:: If you choose to implement mapping of the fabric-couchdb container
          port to a host port, please make sure you are aware of the security
          implications. Mapping of the port in a development environment allows the
          visualization of the database via the CouchDB web interface (Fauxton).
          Production environments would likely refrain from implementing port mapping in
          order to restrict outside access to the CouchDB containers.

You can use **chaincode_example02** chaincode against the CouchDB state database
using the steps outlined above, however in order to exercise the query
capabilities you will need to use a chaincode that has data modeled as JSON,
(e.g. **marbles02**). You can locate the **marbles02** chaincode in the
``fabric/examples/chaincode/go`` directory.

Install, instantiate, invoke, and query **marbles02** chaincode by following the
same general steps outlined above for **chaincode_example02** in the
**Manually execute transactions** section. After the **Join channel** step, use the
following commands to interact with the **marbles02** chaincode:

-  Install and instantiate the chaincode on ``PEER0``:

.. code:: bash

       peer chaincode install -o orderer0:7050 -n marbles -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/marbles02
       peer chaincode instantiate -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/marbles02 -c '{"Args":["init"]}' -P "OR ('Org0MSP.member','Org1MSP.member')"

-  Create some marbles and move them around:

.. code:: bash

        peer chaincode invoke -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -c '{"Args":["initMarble","marble1","blue","35","tom"]}'
        peer chaincode invoke -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -c '{"Args":["initMarble","marble2","red","50","tom"]}'
        peer chaincode invoke -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -c '{"Args":["initMarble","marble3","blue","70","tom"]}'
        peer chaincode invoke -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -c '{"Args":["transferMarble","marble2","jerry"]}'
        peer chaincode invoke -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -c '{"Args":["transferMarblesBasedOnColor","blue","jerry"]}'
        peer chaincode invoke -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -c '{"Args":["delete","marble1"]}'


-  If you chose to activate port mapping, you can now view the state database
   through the CouchDB web interface (Fauxton) by opening a browser and
   navigating to one of the two URLs below.

   For containers running in a vagrant environment:

   ``http://localhost:15984/_utils``

   For non-vagrant environment, use the port address that was mapped in CouchDB
   container specification:

   ``http://localhost:5984/_utils``

   You should see a database named ``mychannel`` and the documents
   inside it.

-  You can run regular queries from the cli (e.g. reading ``marble2``):

.. code:: bash

      peer chaincode query -C mychannel -n marbles -c '{"Args":["readMarble","marble2"]}'

You should see the details of ``marble2``:

.. code:: bash

       Query Result: {"color":"red","docType":"marble","name":"marble2","owner":"jerry","size":50}

Retrieve the history of ``marble1``:

.. code:: bash

      peer chaincode query -C mychannel -n marbles -c '{"Args":["getHistoryForMarble","marble1"]}'

You should see the transactions on ``marble1``:

.. code:: bash

      Query Result: [{"TxId":"1c3d3caf124c89f91a4c0f353723ac736c58155325f02890adebaa15e16e6464", "Value":{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}},{"TxId":"755d55c281889eaeebf405586f9e25d71d36eb3d35420af833a20a2f53a3eefd", "Value":{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"jerry"}},{"TxId":"819451032d813dde6247f85e56a89262555e04f14788ee33e28b232eef36d98f", "Value":}]

You can also perform rich queries on the data content, such as querying marble fields by owner ``jerry``:

.. code:: bash

      peer chaincode query -C mychannel -n marbles -c '{"Args":["queryMarblesByOwner","jerry"]}'

The output should display the two marbles owned by ``jerry``:

.. code:: bash

       Query Result: [{"Key":"marble2", "Record":{"color":"red","docType":"marble","name":"marble2","owner":"jerry","size":50}},{"Key":"marble3", "Record":{"color":"blue","docType":"marble","name":"marble3","owner":"jerry","size":70}}]

Query by field ``owner`` where the value is ``jerry``:

.. code:: bash

      peer chaincode query -C mychannel -n marbles -c '{"Args":["queryMarbles","{\"selector\":{\"owner\":\"jerry\"}}"]}'

The output should display:

.. code:: bash

       Query Result: [{"Key":"marble2", "Record":{"color":"red","docType":"marble","name":"marble2","owner":"jerry","size":50}},{"Key":"marble3", "Record":{"color":"blue","docType":"marble","name":"marble3","owner":"jerry","size":70}}]

A Note on Data Persistence
--------------------------

If data persistence is desired on the peer container or the CouchDB container,
one option is to mount a directory in the docker-host into a relevant directory
in the container. For example, you may add the following two lines in
the peer container specification in the ``docker-compose.yaml`` file:

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

-  Ensure you clear the file system after each run

-  If you see docker errors, remove your images and start from scratch.

.. code:: bash

       make clean
       make peer-docker orderer-docker

-  If you see the below error:

.. code:: bash

       Error: Error endorsing chaincode: rpc error: code = 2 desc = Error installing chaincode code mycc:1.0(chaincode /var/hyperledger/production/chaincodes/mycc.1.0 exits)

You likely have chaincode images (e.g. ``dev-peer0-mycc-1.0`` or ``dev-peer1-mycc-1.0``)
from prior runs. Remove them and try again.

.. code:: bash

    docker rmi -f $(docker images | grep peer[0-9]-peer[0-9] | awk '{print $3}')

-  To cleanup the network, use the ``down`` option:

.. code:: bash

       ./network_setup.sh down
