
Using Private Data in Fabric
============================

This tutorial will demonstrate the use of collections to provide storage
and retrieval of private data on the blockchain network for authorized peers
of organizations.

The information in this tutorial assumes knowledge of private data
stores and their use cases. For more information, check out :doc:`private-data/private-data`.

.. note:: These instructions use the new Fabric chaincode lifecycle introduced
          in the Fabric v2.0 Alpha release. If you would like to use the previous
          lifecycle model to use private data with chaincode, visit the v1.4
          version of the `Using Private Data in Fabric tutorial <https://hyperledger-fabric.readthedocs.io/en/release-1.4/private_data_tutorial.html>`__.

The tutorial will take you through the following steps to practice defining,
configuring and using private data with Fabric:

#. :ref:`pd-build-json`
#. :ref:`pd-read-write-private-data`
#. :ref:`pd-install-define_cc`
#. :ref:`pd-store-private-data`
#. :ref:`pd-query-authorized`
#. :ref:`pd-query-unauthorized`
#. :ref:`pd-purge`
#. :ref:`pd-indexes`
#. :ref:`pd-ref-material`

This tutorial will use the `marbles private data sample <https://github.com/hyperledger/fabric-samples/tree/master/chaincode/marbles02_private>`__
--- running on the Building Your First Network (BYFN) tutorial network --- to
demonstrate how to create, deploy, and use a collection of private data.
The marbles private data sample will be deployed to the :doc:`build_network`
(BYFN) tutorial network. You should have completed the task :doc:`install`;
however, running the BYFN tutorial is not a prerequisite for this tutorial.
Instead the necessary commands are provided throughout this tutorial to use the
network. We will describe what is happening at each step, making it possible to
understand the tutorial without actually running the sample.

.. _pd-build-json:

Build a collection definition JSON file
------------------------------------------

The first step in privatizing data on a channel is to build a collection
definition which defines access to the private data.

The collection definition describes who can persist data, how many peers the
data is distributed to, how many peers are required to disseminate the private
data, and how long the private data is persisted in the private database. Later,
we will demonstrate how chaincode APIs ``PutPrivateData`` and ``GetPrivateData``
are used to map the collection to the private data being secured.

A collection definition is composed of the following properties:

.. _blockToLive:

- ``name``: Name of the collection.

- ``policy``: Defines the organization peers allowed to persist the collection data.

- ``requiredPeerCount``: Number of peers required to disseminate the private data as
  a condition of the endorsement of the chaincode

- ``maxPeerCount``: For data redundancy purposes, the number of other peers
  that the current endorsing peer will attempt to distribute the data to.
  If an endorsing peer goes down, these other peers are available at commit time
  if there are requests to pull the private data.

- ``blockToLive``: For very sensitive information such as pricing or personal information,
  this value represents how long the data should live on the private database in terms
  of blocks. The data will live for this specified number of blocks on the private database
  and after that it will get purged, making this data obsolete from the network.
  To keep private data indefinitely, that is, to never purge private data, set
  the ``blockToLive`` property to ``0``.

- ``memberOnlyRead``: a value of ``true`` indicates that peers automatically
  enforce that only clients belonging to one of the collection member organizations
  are allowed read access to private data.

To illustrate usage of private data, the marbles private data example contains
two private data collection definitions: ``collectionMarbles``
and ``collectionMarblePrivateDetails``. The ``policy`` property in the
``collectionMarbles`` definition allows all members of  the channel (Org1 and
Org2) to have the private data in a private database. The
``collectionMarblesPrivateDetails`` collection allows only members of Org1 to
have the private data in their private database.

For more information on building a policy definition refer to the :doc:`endorsement-policies`
topic.

.. code:: json

 // collections_config.json

 [
   {
        "name": "collectionMarbles",
        "policy": "OR('Org1MSP.member', 'Org2MSP.member')",
        "requiredPeerCount": 0,
        "maxPeerCount": 3,
        "blockToLive":1000000,
        "memberOnlyRead": true
   },

   {
        "name": "collectionMarblePrivateDetails",
        "policy": "OR('Org1MSP.member')",
        "requiredPeerCount": 0,
        "maxPeerCount": 3,
        "blockToLive":3,
        "memberOnlyRead": true
   }
 ]

The data to be secured by these policies is mapped in chaincode and will be
shown later in the tutorial.

This collection definition file is deployed when the chaincode definition is
committed to the channel using the `peer lifecycle chaincode commit command <http://hyperledger-fabric.readthedocs.io/en/latest/commands/peerchaincode.html#peer-chaincode-instantiate>`__.
More details on this process are provided in Section 3 below.

.. _pd-read-write-private-data:

Read and Write private data using chaincode APIs
------------------------------------------------

The next step in understanding how to privatize data on a channel is to build
the data definition in the chaincode. The marbles private data sample divides
the private data into two separate data definitions according to how the data will
be accessed.

.. code-block:: GO

 // Peers in Org1 and Org2 will have this private data in a side database
 type marble struct {
   ObjectType string `json:"docType"`
   Name       string `json:"name"`
   Color      string `json:"color"`
   Size       int    `json:"size"`
   Owner      string `json:"owner"`
 }

 // Only peers in Org1 will have this private data in a side database
 type marblePrivateDetails struct {
   ObjectType string `json:"docType"`
   Name       string `json:"name"`
   Price      int    `json:"price"`
 }

Specifically access to the private data will be restricted as follows:

- ``name, color, size, and owner`` will be visible to all members of the channel (Org1 and Org2)
- ``price`` only visible to members of Org1

Thus two different sets of private data are defined in the marbles private data
sample. The mapping of this data to the collection policy which restricts its
access is controlled by chaincode APIs. Specifically, reading and writing
private data using a collection definition is performed by calling ``GetPrivateData()``
and ``PutPrivateData()``, which can be found `here <https://github.com/hyperledger/fabric/blob/master/core/chaincode/shim/interfaces.go#L179>`_.

The following diagrams illustrate the private data model used by the marbles
private data sample.

 .. image:: images/SideDB-org1.png

 .. image:: images/SideDB-org2.png


Reading collection data
~~~~~~~~~~~~~~~~~~~~~~~~

Use the chaincode API ``GetPrivateData()`` to query private data in the
database.  ``GetPrivateData()`` takes two arguments, the **collection name**
and the data key. Recall the collection  ``collectionMarbles`` allows members of
Org1 and Org2 to have the private data in a side database, and the collection
``collectionMarblePrivateDetails`` allows only members of Org1 to have the
private data in a side database. For implementation details refer to the
following two `marbles private data functions <https://github.com/hyperledger/fabric-samples/blob/master/chaincode/marbles02_private/go/marbles_chaincode_private.go>`__:

 * **readMarble** for querying the values of the ``name, color, size and owner`` attributes
 * **readMarblePrivateDetails** for querying the values of the ``price`` attribute

When we issue the database queries using the peer commands later in this tutorial,
we will call these two functions.

Writing private data
~~~~~~~~~~~~~~~~~~~~

Use the chaincode API ``PutPrivateData()`` to store the private data
into the private database. The API also requires the name of the collection.
Since the marbles private data sample includes two different collections, it is called
twice in the chaincode:

1. Write the private data ``name, color, size and owner`` using the
   collection named ``collectionMarbles``.
2. Write the private data ``price`` using the collection named
   ``collectionMarblePrivateDetails``.

For example, in the following snippet of the ``initMarble`` function,
``PutPrivateData()`` is called twice, once for each set of private data.

.. code-block:: GO

  // ==== Create marble object, marshal to JSON, and save to state ====
	marble := &marble{
		ObjectType: "marble",
		Name:       marbleInput.Name,
		Color:      marbleInput.Color,
		Size:       marbleInput.Size,
		Owner:      marbleInput.Owner,
	}
	marbleJSONasBytes, err := json.Marshal(marble)
	if err != nil {
		return shim.Error(err.Error())
	}

	// === Save marble to state ===
	err = stub.PutPrivateData("collectionMarbles", marbleInput.Name, marbleJSONasBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// ==== Create marble private details object with price, marshal to JSON, and save to state ====
	marblePrivateDetails := &marblePrivateDetails{
		ObjectType: "marblePrivateDetails",
		Name:       marbleInput.Name,
		Price:      marbleInput.Price,
	}
	marblePrivateDetailsBytes, err := json.Marshal(marblePrivateDetails)
	if err != nil {
		return shim.Error(err.Error())
	}
	err = stub.PutPrivateData("collectionMarblePrivateDetails", marbleInput.Name, marblePrivateDetailsBytes)
	if err != nil {
		return shim.Error(err.Error())
	}


To summarize, the policy definition above for our ``collection.json``
allows all peers in Org1 and Org2 to store and transact
with the marbles private data ``name, color, size, owner`` in their
private database. But only peers in Org1 can store and transact with
the ``price`` private data in its private database.

As an additional data privacy benefit, since a collection is being used,
only the private data hashes go through orderer, not the private data itself,
keeping private data confidential from orderer.

Start the network
-----------------

Now we are ready to step through some commands which demonstrate how to use
private data.

 :guilabel:`Try it yourself`

 Before installing, defining, and using the marbles private data chaincode below,
 we need to start the BYFN network. For the sake of this tutorial, we want to
 operate from a known initial state. The following command will kill any active
 or stale docker containers and remove previously generated artifacts.
 Therefore let's run the following command to clean up any previous
 environments:

 .. code:: bash

    cd fabric-samples/first-network
    ./byfn.sh down


 If you've already run through this tutorial, you'll also want to delete the
 underlying docker containers for the marbles private data chaincode. Let's
 run the following commands to clean up previous environments:

 .. code:: bash

    docker rm -f $(docker ps -a | awk '($2 ~ /dev-peer.*.marblesp.*/) {print $1}')
    docker rmi -f $(docker images | awk '($1 ~ /dev-peer.*.marblesp.*/) {print $3}')

 Start up the BYFN network with CouchDB by running the following command:

 .. code:: bash

    ./byfn.sh up -c mychannel -s couchdb

 This will create a simple Fabric network consisting of a single channel named
 ``mychannel`` with two organizations (each maintaining two peer nodes) and an
 ordering service while using CouchDB as the state database. Either LevelDB
 or CouchDB may be used with collections. CouchDB was chosen to demonstrate
 how to use indexes with private data.

 .. note:: For collections to work, it is important to have cross organizational
           gossip configured correctly. Refer to our documentation on :doc:`gossip`,
           paying particular attention to the section on "anchor peers". Our tutorial
           does not focus on gossip given it is already configured in the BYFN sample,
           but when configuring a channel, the gossip anchors peers are critical to
           configure for collections to work properly.

.. _pd-install-define_cc:

Install and define a chaincode with a collection
-------------------------------------------------

Client applications interact with the blockchain ledger through chaincode.
Therefore we need to install a chaincode on every peer that will execute and
endorse our transactions. However, before we can interact with our chaincode,
the members of the channel need to agree on a chaincode definition that
establishes chaincode governance, including the private data collection
configuration. We are going to package, install, and then define the chaincode
on the channel using :doc:`commands/peerlifecycle`.

Install chaincode on all peers
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The chaincode needs to be packaged before it can be installed on our peers.
We can use the `peer lifecycle chaincode package <http://hyperledger-fabric.readthedocs.io/en/latest/commands/peerlifecycle.html#peer-lifecycle-chaincode-package>`__ command
to package the marbles chaincode.

The BYFN network includes two organizations, Org1 and Org2, with two peers each.
Therefore, the chaincode package has to be installed on four peers:

- peer0.org1.example.com
- peer1.org1.example.com
- peer0.org2.example.com
- peer1.org2.example.com

After the chaincode is packaged, we can use the `peer lifecycle chaincode install <http://hyperledger-fabric.readthedocs.io/en/latest/commands/peerlifecycle.html#peer-lifecycle-chaincode-install>`__
command to install the Marbles chaincode on each peer.

  :guilabel:`Try it yourself`

  Assuming you have started the BYFN network, enter the CLI container.

  .. code:: bash

     docker exec -it cli bash

  Your command prompt will change to something similar to:

  .. code:: bash

     bash-4.4#

  1. Use the following command to package the Marbles private data chaincode from
     the git repository inside your local container.

     .. code:: bash

        peer lifecycle chaincode package marblesp.tar.gz --path github.com/hyperledger/fabric-samples/chaincode/marbles02_private/go/ --lang golang --label marblespv1

     This command will create a chaincode package named marblesp.tar.gz.

  2. Use the following command to install the chaincode package onto the peer
     ``peer0.org1.example.com`` in your BYFN network. By default, after starting
     the BYFN network, the active peer is set to:
     ``CORE_PEER_ADDRESS=peer0.org1.example.com:7051``:

     .. code:: bash

        peer lifecycle chaincode install marblesp.tar.gz

     A successful install command will return the chaincode identifier, similar to
     the response below:

     .. code:: bash

        2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nEmycc:ebd89878c2bbccf62f68c36072626359376aa83c36435a058d453e8dbfd894cc" >
        2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: mycc:ebd89878c2bbccf62f68c36072626359376aa83c36435a058d453e8dbfd894cc

  3. Use the CLI to switch the active peer to the second peer in Org1 and install
     the chaincode. Copy and paste the following entire block of commands into the
     CLI container and run them:

     .. code:: bash

        export CORE_PEER_ADDRESS=peer1.org1.example.com:8051
        peer lifecycle chaincode install marblesp.tar.gz

  4. Use the CLI to switch to Org2. Copy and paste the following block of commands
     as a group into the peer container and run them all at once:

     .. code:: bash

        export CORE_PEER_LOCALMSPID=Org2MSP
        export PEER0_ORG2_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
        export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_ORG2_CA
        export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp

  5. Switch the active peer to the first peer in Org2 and install the chaincode:

     .. code:: bash

        export CORE_PEER_ADDRESS=peer0.org2.example.com:9051
        peer lifecycle chaincode install marblesp.tar.gz

  6. Switch the active peer to the second peer in org2 and install the chaincode:

     .. code:: bash

        export CORE_PEER_ADDRESS=peer1.org2.example.com:10051
        peer lifecycle chaincode install marblesp.tar.gz


Approve the chaincode definition
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Each channel member that wants to use the chaincode needs to approve a chaincode
definition for their organization. Since both organizations are going to use the
chaincode in this tutorial, we need to approve the chaincode definition for both
Org1 and Org2.

The chaincode definition includes the package identifier that was returned by
the install command. This packege ID is used to associate the chaincode package
installed on your peers with the chaincode definition approved by your
organization. We can also use the `peer lifecycle chaincode queryinstalled <http://hyperledger-fabric.readthedocs.io/en/latest/commands/peerlifecycle.html#peer-lifecycle-chaincode-queryinstalled>`__
command to find the package ID of ``marblesp.tar.gz``.

Once we have the package ID, we can then use the `peer lifecycle chaincode approveformyorg <http://hyperledger-fabric.readthedocs.io/en/latest/commands/peerlifecycle.html#peer-lifecycle-chaincode-approveformyorg>`__
command to approve a definition of the marbles chaincode for Org1 and Org2. To approve
the private data collection definition that accompanies the ``marbles02_private``,
sample, provide the path to the collections JSON file using the
``--collections-config`` flag.

  :guilabel:`Try it yourself`

  Run the following commands inside the CLI container to approve a definition for
  Org1 and Org2.

  1. Use the following command to query your peer for the package ID of the
     installed chaincode.

     .. code:: bash

        peer lifecycle chaincode queryinstalled

    The command will return the same package identifier as the install command.
    You should see output similar to the following:

    .. code:: bash

       Get installed chaincodes on peer:
       Package ID: marblespv1:57f5353b2568b79cb5384b5a8458519a47186efc4fcadb98280f5eae6d59c1cd, Label: marblespv1
       Package ID: mycc_1:27ef99cb3cbd1b545063f018f3670eddc0d54f40b2660b8f853ad2854c49a0d8, Label: mycc_1

  2. Declare the package ID as an environment variable. Paste the package ID of
     marblespv1 returned by the ``peer lifecycle chaincode queryinstalled`` into
     the command below. The package ID may not be the same for all users, so you
     need to complete this step using the package ID returned from your console.

     .. code:: bash

         export CC_PACKAGE_ID=marblespv1:57f5353b2568b79cb5384b5a8458519a47186efc4fcadb98280f5eae6d59c1cd

  3. Make sure we are running the CLI as Org1. Copy and paste the following block
     of commands as a group into the peer container and run them all at once:

     .. code :: bash

        export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
        export CORE_PEER_LOCALMSPID=Org1MSP
        export PEER0_ORG1_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
        export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_ORG1_CA
        export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp

  4. Use the following command to approve a definition of the Marbles private data
     chaincode for Org2. This command includes a path to the collection definition
     file. The approval is distributed within each organization using gossip, so
     the command does not need to target every peer within an organization.

     .. code:: bash

        export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
        peer lifecycle chaincode approveformyorg --channelID mychannel --name marblesp --version 1.0 --collections-config $GOPATH/src/github.com/hyperledger/fabric-samples/chaincode/marbles02_private/collections_config.json --signature-policy "OR('Org1MSP.member','Org2MSP.member')" --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile $ORDERER_CA --waitForEvent

     When the command completes successfully you should see something similar to:

     .. code:: bash

        2019-03-18 16:04:09.046 UTC [cli.lifecycle.chaincode] InitCmdFactory -> INFO 001 Retrieved channel (mychannel) orderer endpoint: orderer.example.com:7050
        2019-03-18 16:04:11.253 UTC [chaincodeCmd] ClientWait -> INFO 002 txid [efba188ca77889cc1c328fc98e0bb12d3ad0abcda3f84da3714471c7c1e6c13c] committed with status (VALID) at

  5. Use the CLI to switch to Org2. Copy and paste the following block of commands
     as a group into the peer container and run them all at once.

     .. code:: bash

        export CORE_PEER_ADDRESS=peer0.org2.example.com:9051
        export CORE_PEER_LOCALMSPID=Org2MSP
        export PEER0_ORG2_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
        export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_ORG2_CA
        export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp

  6. You can now approve the chaincode definition for Org2:

     .. code:: bash

        peer lifecycle chaincode approveformyorg --channelID mychannel --name marblesp --version 1.0 --collections-config $GOPATH/src/github.com/hyperledger/fabric-samples/chaincode/marbles02_private/collections_config.json --signature-policy "OR('Org1MSP.member','Org2MSP.member')" --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile $ORDERER_CA --waitForEvent

Commit the chaincode definition
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Once a sufficient number of organizations (in this case, a majority) have
approved a chaincode definition, one organization commit the definition to the
channel.

Use the `peer lifecycle chaincode commit <http://hyperledger-fabric.readthedocs.io/en/latest/commands/peerlifecycle.html#peer-lifecycle-chaincode-commit>`__
command to commit the chaincode definition. This command needs to target the
peers in Org1 and Org2 to collect endorsements for the commit transaction. The
peers will endorse the transaction only if their organizations have approved the
chaincode definition. This command will also deploy the collection definition to
the channel.

We are ready to use the chaincode after the chaincode definition has been
committed to the channel. Because the marbles private data chaincode contains an
initiation function, we need to use the `peer chaincode invoke <http://hyperledger-fabric.readthedocs.io/en/master/commands/peerchaincode.html?%20chaincode%20instantiate#peer-chaincode-instantiate>`__ command
to invoke ``Init()`` before we can use other functions in the chaincode.

  :guilabel:`Try it yourself`

  1. Run the following commands to commit the definition of the marbles private
     data chaincode to the BYFN channel ``mychannel``.

    .. code:: bash

       export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
       export ORG1_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
       export ORG2_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
       peer lifecycle chaincode commit -o orderer.example.com:7050 --channelID mychannel --name marblesp --version 1.0 --sequence 1 --collections-config $GOPATH/src/github.com/hyperledger/fabric-samples/chaincode/marbles02_private/collections_config.json --signature-policy "OR('Org1MSP.member','Org2MSP.member')" --init-required --tls true --cafile $ORDERER_CA --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles $ORG1_CA --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles $ORG2_CA --waitForEvent

    .. note:: When specifying the value of the ``--collections-config`` flag, you will
              need to specify the fully qualified path to the collections_config.json file.
              For example:

              .. code:: bash

                 --collections-config  $GOPATH/src/github.com/hyperledger/fabric-samples/chaincode/marbles02_private/collections_config.json

    When the commit transaction completes successfully you should see something
    similar to:

      .. code:: bash

         [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
         [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc

  2. Use the following command to invoke the ``Init`` function to initialize
     the chaincode:

     .. code:: bash

        peer chaincode invoke -o orderer.example.com:7050 --channelID mychannel --name marblesp --isInit --tls true --cafile $ORDERER_CA --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles $ORG1_CA -c '{"Args":["Init"]}'

 .. _pd-store-private-data:

Store private data
------------------

Acting as a member of Org1, who is authorized to transact with all of the private data
in the marbles private data sample, switch back to an Org1 peer and
submit a request to add a marble:

 :guilabel:`Try it yourself`

 Copy and paste the following set of commands to the CLI command line.

 .. code:: bash

    export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    export CORE_PEER_LOCALMSPID=Org1MSP
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    export PEER0_ORG1_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

 Invoke the marbles ``initMarble`` function which
 creates a marble with private data ---  name ``marble1`` owned by ``tom`` with a color
 ``blue``, size ``35`` and price of ``99``. Recall that private data **price**
 will be stored separately from the private data **name, owner, color, size**.
 For this reason, the ``initMarble`` function calls the ``PutPrivateData()`` API
 twice to persist the private data, once for each collection. Also note that
 the private data is passed using the ``--transient`` flag. Inputs passed
 as transient data will not be persisted in the transaction in order to keep
 the data private. Transient data is passed as binary data and therefore when
 using CLI it must be base64 encoded. We use an environment variable
 to capture the base64 encoded value, and use ``tr`` command to strip off the
 problematic newline characters that linux base64 command adds.

 .. code:: bash

   export MARBLE=$(echo -n "{\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\",\"price\":99}" | base64 | tr -d \\n)
   peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n marblesp -c '{"Args":["initMarble"]}'  --transient "{\"marble\":\"$MARBLE\"}"

 You should see results similar to:

 ``[chaincodeCmd] chaincodeInvokeOrQuery->INFO 001 Chaincode invoke successful. result: status:200``

.. _pd-query-authorized:

Query the private data as an authorized peer
--------------------------------------------

Our collection definition allows all members of Org1 and Org2
to have the ``name, color, size, owner`` private data in their side database,
but only peers in Org1 can have the ``price`` private data in their side
database. As an authorized peer in Org1, we will query both sets of private data.

The first ``query`` command calls the ``readMarble`` function which passes
``collectionMarbles`` as an argument.

.. code-block:: GO

   // ===============================================
   // readMarble - read a marble from chaincode state
   // ===============================================

   func (t *SimpleChaincode) readMarble(stub shim.ChaincodeStubInterface, args []string) pb.Response {
   	var name, jsonResp string
   	var err error
   	if len(args) != 1 {
   		return shim.Error("Incorrect number of arguments. Expecting name of the marble to query")
   	}

   	name = args[0]
   	valAsbytes, err := stub.GetPrivateData("collectionMarbles", name) //get the marble from chaincode state

   	if err != nil {
   		jsonResp = "{\"Error\":\"Failed to get state for " + name + "\"}"
   		return shim.Error(jsonResp)
   	} else if valAsbytes == nil {
   		jsonResp = "{\"Error\":\"Marble does not exist: " + name + "\"}"
   		return shim.Error(jsonResp)
   	}

   	return shim.Success(valAsbytes)
   }

The second ``query`` command calls the ``readMarblePrivateDetails``
function which passes ``collectionMarblePrivateDetails`` as an argument.

.. code-block:: GO

   // ===============================================
   // readMarblePrivateDetails - read a marble private details from chaincode state
   // ===============================================

   func (t *SimpleChaincode) readMarblePrivateDetails(stub shim.ChaincodeStubInterface, args []string) pb.Response {
   	var name, jsonResp string
   	var err error

   	if len(args) != 1 {
   		return shim.Error("Incorrect number of arguments. Expecting name of the marble to query")
   	}

   	name = args[0]
   	valAsbytes, err := stub.GetPrivateData("collectionMarblePrivateDetails", name) //get the marble private details from chaincode state

   	if err != nil {
   		jsonResp = "{\"Error\":\"Failed to get private details for " + name + ": " + err.Error() + "\"}"
   		return shim.Error(jsonResp)
   	} else if valAsbytes == nil {
   		jsonResp = "{\"Error\":\"Marble private details does not exist: " + name + "\"}"
   		return shim.Error(jsonResp)
   	}
   	return shim.Success(valAsbytes)
   }

Now :guilabel:`Try it yourself`

 Query for the ``name, color, size and owner`` private data of ``marble1`` as a member of Org1.
 Note that since queries do not get recorded on the ledger, there is no need to pass
 the marble name as a transient input.

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarble","marble1"]}'

 You should see the following result:

 .. code:: bash

    {"color":"blue","docType":"marble","name":"marble1","owner":"tom","size":35}

 Query for the ``price`` private data of ``marble1`` as a member of Org1.

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 You should see the following result:

 .. code:: bash

    {"docType":"marblePrivateDetails","name":"marble1","price":99}

.. _pd-query-unauthorized:

Query the private data as an unauthorized peer
----------------------------------------------

Now we will switch to a member of Org2 which has the marbles private data
``name, color, size, owner`` in its side database, but does not have the
marbles ``price`` private data in its side database. We will query for both
sets of private data.

Switch to a peer in Org2
~~~~~~~~~~~~~~~~~~~~~~~~

From inside the docker container, run the following commands to switch to
the peer which is unauthorized to access the marbles ``price`` private data.

 :guilabel:`Try it yourself`

 .. code:: bash

    export CORE_PEER_ADDRESS=peer0.org2.example.com:9051
    export CORE_PEER_LOCALMSPID=Org2MSP
    export PEER0_ORG2_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
    export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_ORG2_CA
    export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp

Query private data Org2 is authorized to
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Peers in Org2 should have the first set of marbles private data (``name,
color, size and owner``) in their side database and can access it using the
``readMarble()`` function which is called with the ``collectionMarbles``
argument.

 :guilabel:`Try it yourself`

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarble","marble1"]}'

 You should see something similar to the following result:

 .. code:: json

    {"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}

Query private data Org2 is not authorized to
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Peers in Org2 do not have the marbles ``price`` private data in their side database.
When they try to query for this data, they get back a hash of the key matching
the public state but will not have the private state.

 :guilabel:`Try it yourself`

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 You should see a result similar to:

 .. code:: json

    {"Error":"Failed to get private details for marble1: GET_STATE failed:
    transaction ID: b04adebbf165ddc90b4ab897171e1daa7d360079ac18e65fa15d84ddfebfae90:
    Private data matching public hash version is not available. Public hash
    version = &version.Height{BlockNum:0x6, TxNum:0x0}, Private data version =
    (*version.Height)(nil)"}

Members of Org2 will only be able to see the public hash of the private data.

.. _pd-purge:

Purge Private Data
------------------

For use cases where private data only needs to be on the ledger until it can be
replicated into an off-chain database, it is possible to "purge" the data after
a certain set number of blocks, leaving behind only hash of the data that serves
as immutable evidence of the transaction.

There may be private data including personal or confidential
information, such as the pricing data in our example, that the transacting
parties don't want disclosed to other organizations on the channel. Thus, it
has a limited lifespan, and can be purged after existing unchanged on the
blockchain for a designated number of blocks using the ``blockToLive`` property
in the collection definition.

Our ``collectionMarblePrivateDetails`` definition has a ``blockToLive``
property value of three meaning this data will live on the side database for
three blocks and then after that it will get purged. Tying all of the pieces
together, recall this collection definition  ``collectionMarblePrivateDetails``
is associated with the ``price`` private data in the  ``initMarble()`` function
when it calls the ``PutPrivateData()`` API and passes the
``collectionMarblePrivateDetails`` as an argument.

We will step through adding blocks to the chain, and then watch the price
information get purged by issuing four new transactions (Create a new marble,
followed by three marble transfers) which adds four new blocks to the chain.
After the fourth transaction (third marble transfer), we will verify that the
price private data is purged.

 :guilabel:`Try it yourself`

 Switch back to peer0 in Org1 using the following commands. Copy and paste the
 following code block and run it inside your peer container:

 .. code:: bash

    export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    export CORE_PEER_LOCALMSPID=Org1MSP
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    export PEER0_ORG1_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

 Open a new terminal window and view the private data logs for this peer by
 running the following command:

 .. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'

 You should see results similar to the following. Note the highest block number
 in the list. In the example below, the highest block height is ``4``.

 .. code:: bash

    [pvtdatastorage] func1 -> INFO 023 Purger started: Purging expired private data till block number [0]
    [pvtdatastorage] func1 -> INFO 024 Purger finished
    [kvledger] CommitWithPvtData -> INFO 022 Channel [mychannel]: Committed block [0] with 1 transaction(s)
    [kvledger] CommitWithPvtData -> INFO 02e Channel [mychannel]: Committed block [1] with 1 transaction(s)
    [kvledger] CommitWithPvtData -> INFO 030 Channel [mychannel]: Committed block [2] with 1 transaction(s)
    [kvledger] CommitWithPvtData -> INFO 036 Channel [mychannel]: Committed block [3] with 1 transaction(s)
    [kvledger] CommitWithPvtData -> INFO 03e Channel [mychannel]: Committed block [4] with 1 transaction(s)

 Back in the peer container, query for the **marble1** price data by running the
 following command. (A Query does not create a new transaction on the ledger
 since no data is transacted).

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 You should see results similar to:

 .. code:: bash

    {"docType":"marblePrivateDetails","name":"marble1","price":99}

 The ``price`` data is still in the private data ledger.

 Create a new **marble2** by issuing the following command. This transaction
 creates a new block on the chain.

 .. code:: bash

    export MARBLE=$(echo -n "{\"name\":\"marble2\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\",\"price\":99}" | base64 | tr -d \\n)
    peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n marblesp -c '{"Args":["initMarble"]}' --transient "{\"marble\":\"$MARBLE\"}"

 Switch back to the Terminal window and view the private data logs for this peer
 again. You should see the block height increase by 1.

 .. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'

 Back in the peer container, query for the **marble1** price data again by
 running the following command:

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 The private data has not been purged, therefore the results are unchanged from
 previous query:

 .. code:: bash

    {"docType":"marblePrivateDetails","name":"marble1","price":99}

 Transfer marble2 to "joe" by running the following command. This transaction
 will add a second new block on the chain.

 .. code:: bash

    export MARBLE_OWNER=$(echo -n "{\"name\":\"marble2\",\"owner\":\"joe\"}" | base64 | tr -d \\n)
    peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n marblesp -c '{"Args":["transferMarble"]}' --transient "{\"marble_owner\":\"$MARBLE_OWNER\"}"

 Switch back to the Terminal window and view the private data logs for this peer
 again. You should see the block height increase by 1.

 .. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'

 Back in the peer container, query for the marble1 price data by running
 the following command:

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 You should still be able to see the price private data.

 .. code:: bash

    {"docType":"marblePrivateDetails","name":"marble1","price":99}

 Transfer marble2 to "tom" by running the following command. This transaction
 will create a third new block on the chain.

 .. code:: bash

    export MARBLE_OWNER=$(echo -n "{\"name\":\"marble2\",\"owner\":\"tom\"}" | base64 | tr -d \\n)
    peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n marblesp -c '{"Args":["transferMarble"]}' --transient "{\"marble_owner\":\"$MARBLE_OWNER\"}"

 Switch back to the Terminal window and view the private data logs for this peer
 again. You should see the block height increase by 1.

 .. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'

 Back in the peer container, query for the marble1 price data by running
 the following command:

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 You should still be able to see the price data.

 .. code:: bash

    {"docType":"marblePrivateDetails","name":"marble1","price":99}

 Finally, transfer marble2 to "jerry" by running the following command. This
 transaction will create a fourth new block on the chain. The ``price`` private
 data should be purged after this transaction.

 .. code:: bash

    export MARBLE_OWNER=$(echo -n "{\"name\":\"marble2\",\"owner\":\"jerry\"}" | base64 | tr -d \\n)
    peer chaincode invoke -o orderer.example.com:7050 --tls --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n marblesp -c '{"Args":["transferMarble"]}' --transient "{\"marble_owner\":\"$MARBLE_OWNER\"}"

 Switch back to the Terminal window and view the private data logs for this peer
 again. You should see the block height increase by 1.

 .. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'

 Back in the peer container, query for the marble1 price data by running the following command:

 .. code:: bash

    peer chaincode query -C mychannel -n marblesp -c '{"Args":["readMarblePrivateDetails","marble1"]}'

 Because the price data has been purged, you should no longer be able to see
 it. You should see something similar to:

 .. code:: bash

    Error: endorsement failure during query. response: status:500
    message:"{\"Error\":\"Marble private details does not exist: marble1\"}"

.. _pd-indexes:

Using indexes with private data
-------------------------------

Indexes can also be applied to private data collections, by packaging indexes in
the ``META-INF/statedb/couchdb/collections/<collection_name>/indexes`` directory
alongside the chaincode. An example index is available `here <https://github.com/hyperledger/fabric-samples/blob/master/chaincode/marbles02_private/go/META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexOwner.json>`__ .

For deployment of chaincode to production environments, it is recommended
to define any indexes alongside chaincode so that the chaincode and supporting
indexes are deployed automatically as a unit, once the chaincode has been
installed on a peer and instantiated on a channel. The associated indexes are
automatically deployed upon chaincode instantiation on the channel when
the  ``--collections-config`` flag is specified pointing to the location of
the collection JSON file.


.. _pd-ref-material:

Additional resources
--------------------

For additional private data education, a video tutorial has been created.

.. note:: The video uses the previous lifecycle model to install private data
          collections with chaincode.

.. raw:: html

   <br/><br/>
   <iframe width="560" height="315" src="https://www.youtube.com/embed/qyjDi93URJE" frameborder="0" allowfullscreen></iframe>
   <br/><br/>

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
