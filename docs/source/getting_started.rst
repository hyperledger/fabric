Getting Started
===============

.. note:: These instructions have been verified to work against the "alpha" tagged docker
          images and the pre-compiled binaries within the supplied tarball file.
          If you run these commands with images or tools from the current master
          branch, you will see configuration and panic errors.

The getting started scenario provisions a sample Fabric network consisting of
two organizations, each maintaining two peers, and a "solo" ordering service.

Prior to launching the network, we will demonstrate the usage of two fundamental tools:

- cryptogen - generates the x509 certificates used to identify and authenticate
  the various components in the network.
- configtxgen - generates the requisite configuration artifacts for orderer
  bootstrap and channel creation.

In no time we'll have a fully-functioning transactional network with a shared
ledger and digital signature verification.  Let's get going...

Prerequisites and setup
-----------------------

- `Docker <https://www.docker.com/products/overview>`__ - v1.12 or higher
- `Docker Compose <https://docs.docker.com/compose/overview/>`__ - v1.8 or higher
- `Docker Toolbox <https://docs.docker.com/toolbox/toolbox_install_windows/>`__ - Windows users only
- `Go <https://golang.org/>`__ - 1.7 or higher
- `Git Bash <https://git-scm.com/downloads>`__ - Windows users only; provides a better alternative to the Windows command prompt

Curl the artifacts and binaries
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. note:: If you are running on Windows you will want to make use of your Git
          Bash shell for the upcoming terminal commands.

- Download the `cURL <https://curl.haxx.se/download.html>`__ tool if not already installed.
- Determine a location on your machine where you want to place the artifacts and binaries.

.. code:: bash

  mkdir fabric-sample
  cd fabric-sample

Next, execute the following command:

.. code:: bash

  curl -L https://nexus.hyperledger.org/content/repositories/snapshots/sandbox/vex-yul-hyp-jenkins-2/fabric-binaries/release.tar.gz -o release.tar.gz 2> /dev/null;  tar -xvf release.tar.gz

This command pulls and extracts all of the necessary artifacts to set up your
network and places them into a folder titled ``release``.  It also retrieves the
two binaries - cryptogen and configtxgen - which are briefly described at the top
of this page.

Pulling the docker images
^^^^^^^^^^^^^^^^^^^^^^^^^

Change directories into ``release``.  You should see the following:

.. code:: bash

  jdoe-mbp:release johndoe$ ls
  darwin-amd64	linux-amd64	linux-ppc64le	linux-s390x	samples		templates	windows-amd64

You will notice that there are platform-specific folders.  Each folder contains the
corresponding binaries for that platform, along with a script that we will use
to download the Fabric images.  Right now we're only interested in the script.
Navigate into the folder matching your machine's OS and then into ``install``.
For example, if you were running on OSX:

.. code:: bash

  cd darwin-amd64/install

Now run the shell script to download the docker images.  This will take a few
minutes so remember that patience is a virtue:

.. code:: bash

  ./get-docker-images.sh

Execute a ``docker images`` command to view your images.  Assuming you had no
images on your machine prior to running the script, you should see the following:

.. code:: bash

  jdoe-mbp:install johndoe$ docker images
  REPOSITORY                     TAG                  IMAGE ID            CREATED             SIZE
  hyperledger/fabric-couchdb     x86_64-1.0.0-alpha   f3ce31e25872        5 weeks ago         1.51 GB
  hyperledger/fabric-kafka       x86_64-1.0.0-alpha   589dad0b93fc        5 weeks ago         1.3 GB
  hyperledger/fabric-zookeeper   x86_64-1.0.0-alpha   9a51f5be29c1        5 weeks ago         1.31 GB
  hyperledger/fabric-orderer     x86_64-1.0.0-alpha   5685fd77ab7c        5 weeks ago         182 MB
  hyperledger/fabric-peer        x86_64-1.0.0-alpha   784c5d41ac1d        5 weeks ago         184 MB
  hyperledger/fabric-javaenv     x86_64-1.0.0-alpha   a08f85d8f0a9        5 weeks ago         1.42 GB
  hyperledger/fabric-ccenv       x86_64-1.0.0-alpha   91792014b61f        5 weeks ago         1.29 GB

Look at the names for each image; these are the components that will ultimately
comprise our Fabric network.

Using the cryptogen tool
------------------------

First, let's set the environment variable for our platform.  This command
will detect your OS and use the appropriate binaries for the subsequent steps:

.. code:: bash

  # for power or z
  os_arch=$(echo "$(uname -s)-$(uname -m)" | awk '{print tolower($0)}')
  # for linux, osx or windows
  os_arch=$(echo "$(uname -s)-amd64" | awk '{print tolower($0)}')

Check to make sure the ``$os_arch`` variable is properly set:

.. code:: bash

  echo $os_arch

Ok now for the fun stuff - generating the crypto material.  Pop into the ``e2e`` folder:

.. code:: bash

  cd ../../samples/e2e

We are going to pass in the ``crypto-config.yaml`` file as an argument for the
upcoming command.  This file contains the definition/structure of our network
and lists the components that we are generating certs for.  If you open the file
you will see that our network will consist of - one ``OrdererOrg`` and two
``PeerOrgs`` each maintaining two peers. You can easily modify this file to
generate certs for a more elaborate network, however we will leave the sample configuration
for the sake of simplicity.  Got it?  Let's run the tool now:

.. code:: bash

  # this syntax requires you to be in the e2e directory
  # notice that we will pass in the $os_arch variable in order to use the correct binary
  ./../../$os_arch/bin/cryptogen generate --config=./crypto-config.yaml

If the tool runs successfully, you will see the various KeyStores churn out in
your terminal.  The certs are then parked into a ``crypto-config`` folder that
is generated when you run the tool.

Using the configtxgen tool
--------------------------

We will now use our second tool - configtxgen - to create our ordering service
genesis block and a channel configuration artifact.  As the abbreviation suggests,
this tool is a configuration transaction generator.  More info on the configtxgen
tool can be found `here <http://hyperledger-fabric.readthedocs.io/en/latest/configtxgen.html>`__
However, at this stage (and for the sake of brevity) we will simply make use of
the tool to generate our two artifacts.

.. note:: The ``configtx.yaml`` file contains the definitions for our sample
          network and presents the topology of the network components - three members
          (OrdererOrg, Org0 & Org1), and the anchor peers for each PeerOrg
          (peer0.org1 and peer0.org2).  You will notice
          that it is structured similarly to the ``crypto-config.yaml`` that we
          just passed to generate our certs.  The main difference is that we can
          now point to the locations of those certs.  You'll recall that in the
          previous step we created a new folder called ``crypto-config`` and parked
          the certs there.  The ``configtx.yaml`` points to that directory and
          allows us to bundle the root certs for the Orgs constituting our
          network into the genesis block.  This is a critical concept.  Now any
          network entity communicating with the ordering service can have its
          digital signature verified.

Generate the orderer genesis block
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

From your ``e2e`` folder first execute the following:

.. code:: bash

  # this command will not return a response
  export FABRIC_CFG_PATH=$PWD

Then use the tool:

.. code:: bash

  # notice at the top of configtx.yaml we define the profile as TwoOrgs
  ./../../$os_arch/bin/configtxgen -profile TwoOrgs -outputBlock orderer.block
  # for example, if you are running OSX then the binary from darwin-amd64 would have been used

The orderer genesis block - ``orderer.block`` - is output into the ``e2e`` directory.

Generate the channel configuration artifact
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

When we call the ``createChannel`` API, and send the proposal to the ordering
service, we need to pass a channel configuration artifact along with this call.
We will once again leverage the ``configtx.yaml`` and use the same profile
definition - TwoOrgs - that we used to create the orderer genesis block.  In
other words, this channel we are creating is a network-wide channel.  All Orgs
are included.

Still in your ``e2e`` folder execute the following:

.. code:: bash

  # replace the <CHANNEL_NAME> parm with a name of your choosing
  ./../../$os_arch/bin/configtxgen -profile TwoOrgs -outputCreateChannelTx channel.tx -channelID <CHANNEL_NAME>

The channel configuration artifact - ``channel.tx`` - is output into the ``e2e`` directory.

Start the network (No TLS)
--------------------------

We will leverage a docker-compose script to spin up our network.  The docker-compose
points to the images that we have already downloaded, and bootstraps the orderer
with our previously generated ``orderer.block``.  Before launching the network,
open the docker-compose file and comment out the script.sh in the CLI container.
Your docker-compose should look like this:

.. code:: bash

  working_dir: /opt/gopath/src/github.com/hyperledger/fabric/peer
  #command: /bin/bash -c './scripts/script.sh ${CHANNEL_NAME}; '
  volumes:

If left uncommented, the script will exercise all of the CLI commands when the
network is started.  However, we want to go through the commands manually in order to
expose the syntax and functionality of each call.

Start your network:

.. code:: bash

    # this sets our OS
    export ARCH_TAG=$(uname -m)
    # this starts the network in "detached" mode; enter the appropriate value for the CHANNEL_NAME parm
    CHANNEL_NAME=<CHANNEL_NAME> docker-compose -f docker-compose-no-tls.yaml up -d

If you'd like to see the realtime logs for the components, then remove the ``-d`` flag:

.. code:: bash

    CHANNEL_NAME=<CHANNEL_NAME> docker-compose -f docker-compose-no-tls.yaml up

Now open another terminal and navigate back to ``release/samples/e2e``.

Create & Join Channel
---------------------

Go into the cli container:

.. code:: bash

    docker exec -it cli bash

You should see the following:

.. code:: bash

    root@bb5e894d9668:/opt/gopath/src/github.com/hyperledger/fabric/peer#

Create Channel
^^^^^^^^^^^^^^

Recall that we used the configtxgen tool to generate a channel configuration
artifact - ``channel.tx``.  We are going to pass in this artifact to the
orderer as part of the create channel request.

.. note:: For this to work, we must pass in the path of the orderer's local MSP in order to sign
          this create channel call.  Recall that we bootstrapped the orderer
          with the root certificates (ca certs) for all the members of our
          network.  As a result, the orderer can verify the digital signature
          of the submitting client.  This call will also work if we pass in the
          local MSP for Org0 or Org1.

The following environment variables for the orderer must be passed:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com
    CORE_PEER_LOCALMSPID="OrdererMSP"
    CHANNEL_NAME=<YOUR_CHANNEL_NAME>

The syntax is as follows:

.. code:: bash

    peer channel create -o <ORDERER_NAME>:7050 -c <CHANNEL_NAME> -f channel.tx

So our command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com CORE_PEER_LOCALMSPID="OrdererMSP" peer channel create -o orderer.example.com:7050 -c mychannel -f channel.tx

This command returns a genesis block - ``mychannel.block`` - which we will use
to join the channel.

Environment variables
~~~~~~~~~~~~~~~~~~~~~

You can see the syntax for all commands by inspecting the ``script.sh`` file in the ``scripts`` directory.

For the following cli commands against ``PEER0`` to work, we need to set the
values for the four global environment variables given below. Please make sure to override
the values accordingly when calling commands against other peers and the orderer.

.. code:: bash

      # Environment variables for PEER0
      CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com
      CORE_PEER_ADDRESS=peer0.org1.example.com:7051
      CORE_PEER_LOCALMSPID="Org0MSP"
      CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem

These environment variables for each peer are defined in the supplied docker-compose file.

.. note:: In these examples, we are using the default ``mychannel`` for all CHANNEL_NAME arguments.
          If you elect to create a uniquely named channel, be conscious to modify
          your strings accordingly.

Join channel
^^^^^^^^^^^^

Now let's join ``PEER0`` to the channel by passing in the genesis block that was
just returned to us upon the create channel command.

The syntax is as follows:

.. code:: bash

    peer channel join -b <CHANNEL_NAME>.block

Remember, we need to pass the four global variables.  So this command in its
entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com CORE_PEER_ADDRESS=peer0.org1.example.com:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem peer channel join -b mychannel.block

Install
^^^^^^^

Now we will install the chaincode source onto the peer's filesystem.  The syntax
is as follows:

.. code:: bash

    peer chaincode install -n <CHAINCODE_NAME> -v <CHAINCODE_VERSION> -p <CHAINCODE_PATH>

This command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com CORE_PEER_ADDRESS=peer0.org1.example.com:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem peer chaincode install -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 >&log.txt

Instantiate
^^^^^^^^^^^

Now we start the chaincode container and initialize our key value pairs.  The
syntax for instantiate is as follows:

.. code:: bash

    peer chaincode instantiate -o <ORDERER_NAME>:7050 -C <CHANNEL_NAME> -n <CHAINCODE_NAME> -v <VERSION> -c '{"Args":["init","key","value"]}' -P "OR/AND (CHAINCODE_POLICY)"

Take note of the ``-P`` argument.  This is our policy where we specify the
required level of endorsement for a transaction against this chaincode to be
validated.  In the command below you'll notice that we specify our policy as
``-P "OR ('Org0MSP.member','Org1MSP.member')"``.  This means that we need
"endorsement" from a peer belonging to Org0 **OR** Org1 (i.e. only one endorsement).
If we changed the syntax to ``AND`` then we would need two endorsements.

This command in its entirety would be:

.. code:: bash

    # we instantiate with the following key value pairs: "a","100","b","200"
    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com CORE_PEER_ADDRESS=peer0.org1.example.com:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem peer chaincode instantiate -o orderer.example.com:7050 -C mychannel -n mycc -v 1.0 -c '{"Args":["init","a","100","b","200"]}' -P "OR ('Org0MSP.member','Org1MSP.member')"

.. note::   The above command will only start a single chaincode container.  If
            you want to interact with different peers, you must first install
            the source code onto that peer's filesystem.  You can then send
            an invoke or query to the peer.  You needn't instantiate twice, this
            command will propagate to the entire channel.

Query
^^^^^

Lets query for the value of "a" to make sure the chaincode was properly instantiated
and the state DB was populated.  The syntax for query is as follows:

.. code:: bash

    peer chaincode query -C <CHANNEL_NAME> -n <CHAINCODE_NAME> -c '{"Args":["query","key"]}'

This command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com CORE_PEER_ADDRESS=peer0.org1.example.com:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

Invoke
^^^^^^

Lastly we will move "10" from "a" to "b".  This transaction will cut a new block
and update the state DB.  The syntax for invoke is as follows:

.. code:: bash

    peer chaincode invoke -o <ORDERER_NAME>:7050 -C <CHANNEL_NAME> -n <CHAINCODE_NAME> -c '{"Args":["invoke","key","key","value"]}'

This command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com CORE_PEER_ADDRESS=peer0.org1.example.com:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem peer chaincode invoke -o orderer.example.com:7050 -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

Query
^^^^^

Lets confirm that our previous invocation executed properly.  We initialized the
key "a" with a value of "100".  Therefore, removing "10" should return a value
of "90" when we query "a".  The syntax for query is outlined above.

This query command in its entirety would be:

.. code:: bash

    CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com CORE_PEER_ADDRESS=peer0.org1.example.com:7051 CORE_PEER_LOCALMSPID="Org0MSP" CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/cacerts/org1.example.com-cert.pem peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

Start the network (TLS enabled)
------------------------------

Use the ``script.sh`` to see the exact syntax for TLS-enabled CLI commands.

Before starting, we need to modify our docker-compose file to reflect the appropriate private keys for
the orderer and peers.

From your ``e2e`` directory execute the following:

.. code:: bash

    PRIV_KEY=$(ls crypto-config/ordererOrganizations/example.com/orderers/orderer.example.com/keystore/) sed -i "s/ORDERER_PRIVATE_KEY/${PRIV_KEY}/g" docker-compose.yaml
    PRIV_KEY=$(ls crypto-config/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/keystore/) sed -i "s/PEER0_ORG1_PRIVATE_KEY/${PRIV_KEY}/g" docker-compose.yaml
    PRIV_KEY=$(ls crypto-config/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/keystore/) sed -i "s/PEER0_ORG2_PRIVATE_KEY/${PRIV_KEY}/g" docker-compose.yaml
    PRIV_KEY=$(ls crypto-config/peerOrganizations/org1.example.com/peers/peer1.org1.example.com/keystore/) sed -i "s/PEER1_ORG1_PRIVATE_KEY/${PRIV_KEY}/g" docker-compose.yaml
    PRIV_KEY=$(ls crypto-config/peerOrganizations/org2.example.com/peers/peer1.org2.example.com/keystore/) sed -i "s/PEER1_ORG2_PRIVATE_KEY/${PRIV_KEY}/g" docker-compose.yaml

These commands will modify the TLS_KEY_FILE variables in your docker-compose.
Once you have executed all five commands, spin the network back up and begin
by creating your channel.

Scripts
-------

We exposed the verbosity of the commands in order to provide some edification
on the underlying flow and the appropriate syntax.  Entering the commands manually
through the CLI is quite onerous, therefore we provide a few scripts to do the
entirety of the heavy lifting.

Clean up
^^^^^^^^

Let's clean things up before continuing.  This command will remove both the
active and exited containers:


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

Lastly, remove the `crypto-config`` folder and the two artifacts - ``channel.tx``
& ``orderer.block``.

.. code:: bash

    # from the e2e directory
    rm -rf channel.tx orderer.block crypto-config

All in one
^^^^^^^^^^

This script will do it all for you!  From the ``e2e`` directory:

.. code:: bash

    ./network_setup.sh up <channel_name>

.. note:: If you choose not to pass a channel_name value, then the default
          ``mychannel`` will be used.

Now shut down your network and remove the chaincode images and artifacts:

.. code:: bash

    ./network_setup.sh down

If you want to restart:

.. code:: bash

    ./network_setup.sh restart

APIs only
^^^^^^^^^

The other option is to manually generate your crypto material and configuration
artifacts, and then use the embedded ``script.sh`` in the docker-compose files
to drive your network.  Make sure this script is not commented out in your
CLI container.

When the scripts complete successfully, you should see the following message
in your terminal:

.. code:: bash

  ===================== Query on PEER3 on channel 'mychannel' is successful =====================

  ===================== All GOOD, End-2-End execution completed =====================

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

-  Open the ``release/samples/e2e/docker-compose.yaml`` and un-comment
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
``release/samples/chaincodes/go`` directory.

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

-  Ensure you clear the file system after each run.

-  If you see docker errors, remove your containers and start again.

.. code:: bash

       docker rm -f $(docker ps -aq)

- If you elect to run the "All in one" option, be sure you have deleted your
  crypto directory and the two artifacts.  This can be achieved with the following
  command:

.. code:: bash

      ./network_setup.sh down

-  If you see the below error:

.. code:: bash

       Error: Error endorsing chaincode: rpc error: code = 2 desc = Error installing chaincode code mycc:1.0(chaincode /var/hyperledger/production/chaincodes/mycc.1.0 exits)

You likely have chaincode images (e.g. ``dev-peer0-mycc-1.0`` or ``dev-peer1-mycc-1.0``)
from prior runs. Remove them and try again.

.. code:: bash

    docker rmi -f $(docker images | grep peer[0-9]-peer[0-9] | awk '{print $3}')

- If you see connectivity or communication errors, try restarting your Docker process.

- If you see something similar to the following:

.. code:: bash

  Error connecting: rpc error: code = 14 desc = grpc: RPC failed fast due to transport failure
  Error: rpc error: code = 14 desc = grpc: RPC failed fast due to transport failure

Make sure you are using the supplied binaries in the tarball file, and running
your backend against "alpha" images.

If you see the below error:

.. code:: bash

  [configtx/tool/localconfig] Load -> CRIT 002 Error reading configuration: Unsupported Config Type ""
  panic: Error reading configuration: Unsupported Config Type ""

Then you have an environment variable - ``ORDERER_CFG_PATH`` that is no longer
in use.  You can manually change this value in the scripts, or re-download
the tarball file.

- If you continue to see errors, share your logs on the **# fabric-questions**
channel on `Hyperledger Rocket Chat <https://chat.hyperledger.org/home>`__.

-------------------------------------------------------------------------------
