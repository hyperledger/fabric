End-to-End Flow
===============

The end-to-end verification demonstrates usage of the configuration
transaction tool for orderer bootstrap and channel creation. The tool
consumes a configuration transaction yaml file, which defines the
cryptographic material (certs) used for member identity and network
authentication. In other words, the MSP information for each member and
its corresponding network entities.

*Currently the crypto material is baked into the directory. However,
there will be a tool in the near future to organically generate the
certificates*

Prerequisites
-------------

-  Follow the steps for setting up a `development
   environment <http://hyperledger-fabric.readthedocs.io/en/latest/dev-setup/devenv.html>`__

-  Clone the Fabric code base.

   .. code:: bash

       git clone http://gerrit.hyperledger.org/r/fabric

   or though a mirrored repository in github:

   ::

       git clone https://github.com/hyperledger/fabric.git

-  Make the ``configtxgen`` tool.

   .. code:: bash

       cd $GOPATH/src/github.com/hyperledger/fabric/devenv
       vagrant up
       vagrant ssh
       # ensure sure you are in the /fabric directory where the Makefile resides
       make configtxgen

-  Make the peer and orderer images.

   .. code:: bash

       # make sure you are in vagrant and in the /fabric directory
       make docker

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

Configuration Transaction Generator
-----------------------------------

The `configtxgen
tool <https://github.com/hyperledger/fabric/blob/master/docs/source/configtxgen.rst>`__
is used to create two artifacts: - orderer **bootstrap block** - fabric
**channel configuration transaction**

The orderer block is the genesis block for the ordering service, and the
channel transaction file is broadcast to the orderer at channel creation
time.

The ``configtx.yaml`` contains the definitions for the sample network.
There are two members, each managing and maintaining two peer nodes.
Inspect this file to better understand the corresponding cryptographic
material tied to the member components. The ``/crypto`` directory
contains the admin certs, ca certs, private keys for each entity, and
the signing certs for each entity.

For ease of use, a script - ``generateCfgTrx.sh`` - is provided. The
script will generate the two configuration artifacts.

Run the shell script
^^^^^^^^^^^^^^^^^^^^

Make sure you are in the ``examples/e2e_cli`` directory and in your
vagrant environment. You can elect to pass in a unique name for your
channel or simply execute the script without the ``channel-ID``
parameter. If you choose not to pass in a unique name, then a channel
with the default name of ``mychannel`` will be generated.

.. code:: bash

    cd examples/e2e_cli
    # note the <channel-ID> parm is optional
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

These configuration transactions will bundle the crypto material for the
participating members and their network components and output an orderer
genesis block and channel transaction artifact. These two artifacts are
required for a functioning transactional network with
sign/verify/authenticate capabilities.

Manually generate the artifacts (optional)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

In your vagrant environment, navigate to the ``/sampleconfig``
directory and replace the ``configtx.yaml`` file with the supplied yaml
file in the ``/e2e_cli`` directory. Then return to the ``/e2e_cli``
directory.

.. code:: bash

    # Generate orderer bootstrap block
    configtxgen -profile TwoOrgs -outputBlock <block-name>
    # example: configtxgen -profile TwoOrgs -outputBlock orderer.block

    # Generate channel configuration transaction
    configtxgen -profile TwoOrgs -outputCreateChannelTx <cfg txn name> -channelID <channel-id>
    # example: configtxgen -profile TwoOrgs -outputCreateChannelTx channel.tx -channelID mychannel

Run the end-to-end test with Docker
-----------------------------------

Make sure you are in the ``/e2e_cli`` directory. Then use docker-compose
to spawn the network entities and drive the tests.

.. code:: bash

    [CHANNEL_NAME=<channel-id>] docker-compose up -d

If you created a unique channel name, be sure to pass in that parameter.
For example,

.. code:: bash

    CHANNEL_NAME=abc docker-compose up -d

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

All in one
^^^^^^^^^^

You can also generate the artifacts and drive the tests using a single
shell script. The ``configtxgen`` and ``docker-compose`` commands are
embedded in the script.

.. code:: bash

    ./network_setup.sh up <channel-ID>

Once again, if you choose not to pass the ``channel-ID`` parameter, then
your channel will default to ``mychannel``.

What's happening behind the scenes?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

-  A script - ``script.sh`` - is baked inside the CLI container. The
   script drives the ``createChannel`` command against the default
   ``mychannel`` name.

-  The output of ``createChannel`` is a genesis block -
   ``mychannel.block`` - which is stored on the file system.

-  the ``joinChannel`` command is exercised for all four peers who will
   pass in the genesis block.

-  Now we have a channel consisting of four peers, and two
   organizations.

-  ``PEER0`` and ``PEER1`` belong to Org1; ``PEER2`` and ``PEER3``
   belong to Org2

-  Recall that these relationships are defined in the ``configtx.yaml``

-  A chaincode - *chaincode\_example02* is installed on ``PEER0`` and
   ``PEER2``

-  The chaincode is then "instantiated" on ``PEER2``. Instantiate simply
   refers to starting the container and initializing the key value pairs
   associated with the chaincode. The initial values for this example
   are "a","100" "b","200". This "instantiation" results in a container
   by the name of ``dev-peer2-mycc-1.0`` starting.

-  The instantiation also passes in an argument for the endorsement
   policy. The policy is defined as
   ``-P "OR    ('Org1MSP.member','Org2MSP.member')"``, meaning that any
   transaction must be endorsed by a peer tied to Org1 or Org2.

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
(like ``Peer1`` in the above example) . Finally, the chaincode is accessible
after it is installed (like ``Peer3`` in the above example) because it
already has been instantiated.

How do I see these transactions?
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Check the logs for the CLI docker container.

::

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

Run the end-to-end test manually with Docker
--------------------------------------------

From your vagrant environment exit the currently running containers:

.. code:: bash

    docker rm -f $(docker ps -aq)

Execute a ``docker images`` command in your terminal to view the
chaincode images. They will look similar to the following:

.. code:: bash

    REPOSITORY                     TAG                             IMAGE ID            CREATED             SIZE
    dev-peer3-mycc-1.0             latest                          3415bc2e146c        5 hours ago         176 MB
    dev-peer0-mycc-1.0             latest                          140d7ee3e911        5 hours ago         176 MB
    dev-peer2-mycc-1.0             latest                          6e4fc412969e        5 hours ago         176 MB

Remove these images:

.. code:: bash

    docker rmi <IMAGE ID> <IMAGE ID> <IMAGE ID>

For example:

.. code:: bash

    docker rmi -f 341 140 6e4

Ensure you have the configuration artifacts. If you deleted them, run
the shell script again:

.. code:: bash

    ./generateCfgTrx.sh <channel-ID>

Modify the docker-compose file
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Open the docker-compose file and comment out the command to run
``script.sh``. Navigate down to the cli image and place a ``#`` to the
left of the command. For example:

.. code:: bash

        working_dir: /opt/gopath/src/github.com/hyperledger/fabric/peer
      # command: /bin/bash -c './scripts/script.sh ${CHANNEL_NAME}'

Save the file and return to the ``/e2e_cli`` directory.

Now restart your network:

.. code:: bash

    # make sure you are in the /e2e_cli directory where you docker-compose script resides
    docker-compose up -d

Command syntax
^^^^^^^^^^^^^^

Refer to the create and join commands in the ``script.sh``.

For the following CLI commands against `peer0` to work, you need to set the
values for four environment variables, given below. Please make sure to override
the values accordingly when calling commands against other peers and the
orderer.

.. code:: bash

    # Environment variables for PEER0
    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peer/peer0/localMspConfig
    CORE_PEER_ADDRESS=peer0:7051
    CORE_PEER_LOCALMSPID="Org1MSP"
    CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peer/peer0/localMspConfig/cacerts/peerOrg0.pem

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

    # the channel.tx and orderer.block are mounted in the crypto/orderer folder within your cli container
    # as a result, we pass the full path for the file
     peer channel create -o orderer0:7050 -c mychannel -f crypto/orderer/channel.tx --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem

Since the `channel create` runs against the orderer, we need to override the
four environment variables set before. So the above command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig CORE_PEER_LOCALMSPID="OrdererMSP" peer channel create -o orderer0:7050 -c mychannel -f crypto/orderer/channel.tx --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem


**Note**: You will remain in the CLI container for the remainder of
these manual commands. You must also remember to preface all commands
with the corresponding environment variables for targetting a peer other than
`peer0`.

Join channel
^^^^^^^^^^^^

Join specific peers to the channel

.. code:: bash

    # By default, this joins PEER0 only
    # the mychannel.block is also mounted in the crypto/orderer directory
     peer channel join -b mychannel.block

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
endorsement from one peer node that is a part of either `Org1` or `Org2`.
The command is:

.. code:: bash

    peer chaincode instantiate -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a", "100", "b","200"]}' -P "OR ('Org1MSP.member','Org2MSP.member')"

See the `endorsement
policies <http://hyperledger-fabric.readthedocs.io/en/latest/endorsement-policies.html>`__
documentation for more details on policy implementation.

Invoke chaincode
^^^^^^^^^^^^^^^^

.. code:: bash

    peer chaincode invoke -o orderer0:7050  --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem  -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

**NOTE**: Make sure to wait a few seconds for the operation to complete.

Query chaincode
^^^^^^^^^^^^^^^

.. code:: bash

    peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

The result of the above command should be as below:

.. code:: bash

    Query Result: 90

Run the end-to-end test using the native binaries
-------------------------------------------------

Open your vagrant environment:

.. code:: bash

    cd $GOPATH/src/github.com/hyperledger/fabric/devenv

.. code:: bash

    # you may have to first start your VM with vagrant up
    vagrant ssh

From the ``fabric`` directory build the issue the following commands to
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

::

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

Start the peer in *"chainless"* mode

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

::

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

       # make sure you are in the /fabric directory
       make couchdb

-  Open the ``fabric/examples/e2e_cli/docker-compose.yaml`` and un-comment
   all commented statements relating to CouchDB containers and peer container
   use of CouchDB. These instructions are are also outlined in the
   same ``docker-compose.yaml`` file. Search the file for 'couchdb' (case insensitive) references.

*chaincode_example02* should now work using CouchDB underneath.

***Note***: If you choose to implement mapping of the fabric-couchdb container
port to a host port, please make sure you are aware of the security
implications. Mapping of the port in a development environment allows the
visualization of the database via the CouchDB web interface (Fauxton).
Production environments would likely refrain from implementing port mapping in
order to restrict outside access to the CouchDB containers.

You can use *chaincode_example02* chaincode against the CouchDB state database
using the steps outlined above, however in order to exercise the query
capabilities you will need to use a chaincode that has data modeled as JSON,
(e.g. *marbles02*). You can locate the *marbles02* chaincode in the
``fabric/examples/chaincode/go`` directory.

Install, instantiate, invoke, and query *marbles02* chaincode by following the
same general steps outlined above for *chaincode_example02* in the **Manually
create the channel and join peers through CLI** section. After the **Join
channel** step, use the following steps to interact with the *marbles02*
chaincode:

-  Install and instantiate the chaincode in ``peer0``:

   .. code:: bash

       peer chaincode install -o orderer0:7050 -n marbles -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/marbles02
       peer chaincode instantiate -o orderer0:7050 --tls $CORE_PEER_TLS_ENABLED --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/orderer/localMspConfig/cacerts/ordererOrg0.pem -C mychannel -n marbles -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/marbles02 -c '{"Args":["init"]}' -P "OR      ('Org1MSP.member','Org2MSP.member')"

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

-  You can run regular queries from the `cli` (e.g. reading ``marble2``):

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



-  You can also perform rich queries on the data content, such as querying marble fields by owner ``jerry``:

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

   You likely have chaincode images (e.g. ``peer0-peer0-mycc-1.0`` or
   ``peer1-peer0-mycc1-1.0``) from prior runs. Remove them and try
   again.

.. code:: bash

    docker rmi -f $(docker images | grep peer[0-9]-peer[0-9] | awk '{print $3}')

-  To cleanup the network, use the ``down`` option:

   .. code:: bash

       ./network_setup.sh down
