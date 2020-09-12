Using Private Data in Fabric
============================

This tutorial will demonstrate the use of collections to provide storage
and retrieval of private data on the blockchain network for authorized peers
of organizations.

The information in this tutorial assumes knowledge of private data
stores and their use cases. For more information, check out :doc:`private-data/private-data`.

.. note:: These instructions use the new Fabric chaincode lifecycle introduced
          in the Fabric v2.0 release. If you would like to use the previous
          lifecycle model to use private data with chaincode, visit the v1.4
          version of the `Using Private Data in Fabric tutorial <https://hyperledger-fabric.readthedocs.io/en/release-1.4/private_data_tutorial.html>`__.

The tutorial will take you through the following steps to practice defining,
configuring and using private data with Fabric:

#. :ref:`pd-use-case`
#. :ref:`pd-build-json`
#. :ref:`pd-read-write-private-data`
#. :ref:`pd-install-define_cc`
#. :ref:`pd-register-identities`
#. :ref:`pd-store-private-data`
#. :ref:`pd-query-authorized`
#. :ref:`pd-purge`
#. :ref:`pd-transfer-asset`
#. :ref:`pd-indexes`
#. :ref:`pd-ref-material`

This tutorial will deploy the `asset transfer private data sample <https://github.com/hyperledger/fabric-samples/tree/master/asset-transfer-private-data/chaincode-go>`__
to the Fabric test network to demonstrate how to create, deploy, and use a collection of
private data.
You should have completed the task :doc:`install`.

.. _pd-use-case:

Asset transfer private data sample use case
-------------------------------------------
This sample demonstrates the use of three private data collections, ``assetCollection``, ``Org1MSPPrivateCollection`` & ``Org2MSPPrivateCollection`` to transfer an asset between Org1 and Org2, using following use case:

A member of Org1 creates a new asset, henceforth referred as owner. The public details of the asset,
including the owner, are stored in the private data collection named ``assetCollection``. The asset is also created with an appraised
value supplied by the owner. The appraised value is used by each participant to agree to the transfer of the asset, and is only stored in owner organization's collection. In our case, the initial appraisal value agreed by the owner is stored in the ``Org1MSPPrivateCollection``.

To purchase the asset, the buyer needs to agree to the same appraised value as the asset owner. In this step, the buyer (a member of Org2) creates an agreement to trade and agree to an appraisal value using smart contract function ``'AgreeToTransfer'``. This value is stored in ``Org2MSPPrivateCollection`` collection.

Now, the asset owner can transfer the asset to the buyer using smart contract function ``'TransferAsset'``, which checks for the conditions that must be met for transfer to succeed.

.. _pd-build-json:

Build a collection definition JSON file
---------------------------------------

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

- ``memberOnlyWrite``: a value of ``true`` indicates that peers automatically
  enforce that only clients belonging to one of the collection member organizations
  are allowed write access to private data.

To illustrate usage of private data, the asset transfer private data example contains
three private data collection definitions: ``assetCollection``, ``Org1MSPPrivateCollection``,
and ``Org2MSPPrivateCollection``. The ``policy`` property in the
``assetCollection`` definition allows all members of the channel (Org1 and
Org2) to have the private data in a private database. The
``Org1MSPPrivateCollection`` collection allows only members of Org1 to
have the private data in their private database, and the ``Org2MSPPrivateCollection``
collection allows only members of Org2 to have the private data in their private database.

For more information on building a policy definition refer to the :doc:`endorsement-policies`
topic.

.. code:: json

 // collections_config.json

 [
    {
    "name": "assetCollection",
    "policy": "OR('Org1MSP.member', 'Org2MSP.member')",
    "requiredPeerCount": 1,
    "maxPeerCount": 1,
    "blockToLive":1000000,
    "memberOnlyRead": true,
    "memberOnlyWrite": true
    },
    {
    "name": "Org1MSPPrivateCollection",
    "policy": "OR('Org1MSP.member')",
    "requiredPeerCount": 0,
    "maxPeerCount": 1,
    "blockToLive":3,
    "memberOnlyRead": true,
    "memberOnlyWrite": false,
    "endorsementPolicy": {
        "signaturePolicy": "OR('Org1MSP.member')"
    }
    },
    {
    "name": "Org2MSPPrivateCollection",
    "policy": "OR('Org2MSP.member')",
    "requiredPeerCount": 0,
    "maxPeerCount": 1,
    "blockToLive":3,
    "memberOnlyRead": true,
    "memberOnlyWrite": false,
    "endorsementPolicy": {
        "signaturePolicy": "OR('Org2MSP.member')"
    }
    }
 ]

The data to be secured by these policies is mapped in chaincode and will be
shown later in the tutorial.

This collection definition file is deployed when the chaincode definition is
committed to the channel using the `peer lifecycle chaincode commit command <commands/peerlifecycle.html#peer-lifecycle-chaincode-commit>`__.
More details on this process are provided in Section 3 below.

.. _pd-read-write-private-data:

Read and Write private data using chaincode APIs
------------------------------------------------

The next step in understanding how to privatize data on a channel is to build
the data definition in the chaincode. The asset transfer private data sample divides
the private data into three separate data definitions according to how the data will
be accessed.

.. code-block:: GO

 // Peers in Org1 and Org2 will have this private data in a side database
 type Asset struct {
	Type  string `json:"objectType"` //Type is used to distinguish the various types of objects in state database
	ID    string `json:"assetID"`
	Color string `json:"color"`
	Size  int    `json:"size"`
	Owner string `json:"owner"`
 }

 // AssetPrivateDetails describes details that are private to owners

 // Only peers in Org1 will have this private data in a side database
 type AssetPrivateDetails struct {
	ID             string `json:"assetID"`
	AppraisedValue int    `json:"appraisedValue"`
 }

 // Only peers in Org2 will have this private data in a side database
 type AssetPrivateDetails struct {
	ID             string `json:"assetID"`
	AppraisedValue int    `json:"appraisedValue"`
 }

Specifically, access to the private data will be restricted as follows:

- ``objectType, color, size, and owner`` are stored in ``assetCollection`` and hence will be visible to members of the channel per the definition in the collection policy (Org1 and Org2).
- ``AppraisedValue`` of an asset is stored in collection Org1MSPPrivateCollection or Org2MSPPrivateCollection , depending on the owner of the asset.  The value is only accessible to members of the respective collection.



The storing of private data is illustrated in the asset transfer private data
sample. The mapping of this data to the collection policy which restricts its
access is controlled by chaincode APIs. Specifically, reading and writing
private data using a collection definition is performed by calling ``GetPrivateData()``
and ``PutPrivateData()``, which can be found `here <https://godoc.org/github.com/hyperledger/fabric-chaincode-go/shim#ChaincodeStub>`_.

The following diagram illustrates the private data model used by the asset transfer
private data sample. Note that Org3 is only shown in the diagram below for context to illustrate that
if there were any other organizations on the channel, they would not have access to *any*
of the private data collections where they are not defined in the private data collection policy configuration.

.. image:: images/SideDB-org1-org2.png

Reading collection data
~~~~~~~~~~~~~~~~~~~~~~~~

Use the chaincode API ``GetPrivateData()`` to query private data in the
database.  ``GetPrivateData()`` takes two arguments, the **collection name**
and the data key. Recall the collection  ``assetCollection`` allows members of
Org1 and Org2 to have the private data in a side database, and the collection
``Org1MSPPrivateCollection`` allows only members of Org1 to have their
private data in a side database and ``Org2MSPPrivateCollection`` allows members
of Org2 to have their private data in a side database.
For implementation details refer to the following two `asset transfer private data functions <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/asset-transfer-private-data/chaincode-go/chaincode/asset_queries.go>`__:

 * **ReadAsset** for querying the values of the ``assetID, color, size and owner`` attributes.
 * **ReadAssetPrivateDetails** for querying the values of the ``appraisedValue`` attribute.

When we issue the database queries using the peer commands later in this tutorial,
we will call these two functions.

Writing private data
~~~~~~~~~~~~~~~~~~~~

Use the chaincode API ``PutPrivateData()`` to store the private data
into the private database. The API also requires the name of the collection.
Note that the asset transfer private data sample includes three different private data collections, but it is called
twice in the chaincode (in this scenario acting as Org1). The third collection (``Org2MSPPrivateCollection``) would be used if we were acting as Org2, but
would still only be called twice in the chaincode as it would replace ``Org1MSPPrivateCollection``:

1. Write the private data ``assetID, color, size and owner`` using the
   collection named ``assetCollection``.
2. Write the private data ``appraisedValue`` using the collection named
   ``Org1MSPPrivateCollection``.

For example, in the following snippet of the ``CreateAsset`` function,
``PutPrivateData()`` is called twice, once for each set of private data.

.. code-block:: GO

  // CreateAsset creates a new asset by placing the main asset details in the assetCollection
  // that can be read by both organizations. The appraisal value is stored in the owner's org specific collection.
  func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface) error {

        // Get new asset from transient map
        transientMap, err := ctx.GetStub().GetTransient()
        if err != nil {
            return fmt.Errorf("error getting transient: %v", err)
        }

        // Asset properties are private, therefore they get passed in transient field, instead of func args
        transientAssetJSON, ok := transientMap["asset_properties"]
        if !ok {
            //log error to stdout
            return fmt.Errorf("asset not found in the transient map input")
        }

        type assetTransientInput struct {
            Type           string `json:"objectType"` //Type is used to distinguish the various types of objects in state database
            ID             string `json:"assetID"`
            Color          string `json:"color"`
            Size           int    `json:"size"`
            AppraisedValue int    `json:"appraisedValue"`
        }

        var assetInput assetTransientInput
        err = json.Unmarshal(transientAssetJSON, &assetInput)
        if err != nil {
            return fmt.Errorf("failed to unmarshal JSON: %v", err)
        }

        if len(assetInput.Type) == 0 {
            return fmt.Errorf("objectType field must be a non-empty string")
        }
        if len(assetInput.ID) == 0 {
            return fmt.Errorf("assetID field must be a non-empty string")
        }
        if len(assetInput.Color) == 0 {
            return fmt.Errorf("color field must be a non-empty string")
        }
        if assetInput.Size <= 0 {
            return fmt.Errorf("size field must be a positive integer")
        }
        if assetInput.AppraisedValue <= 0 {
            return fmt.Errorf("appraisedValue field must be a positive integer")
        }

        // Check if asset already exists
        assetAsBytes, err := ctx.GetStub().GetPrivateData(assetCollection, assetInput.ID)
        if err != nil {
            return fmt.Errorf("failed to get asset: %v", err)
        } else if assetAsBytes != nil {
            fmt.Println("Asset already exists: " + assetInput.ID)
            return fmt.Errorf("this asset already exists: " + assetInput.ID)
        }

        // Get ID of submitting client identity
        clientID, err := ctx.GetClientIdentity().GetID()
        if err != nil {
            return fmt.Errorf("failed to get verified OrgID: %v", err)
        }

        // Verify that the client is submitting request to peer in their organization
        // This is to ensure that a client from another org doesn't attempt to read or
        // write private data from this peer.
        err = verifyClientOrgMatchesPeerOrg(ctx)
        if err != nil {
            return fmt.Errorf("CreateAsset cannot be performed: Error %v", err)
        }

        // Make submitting client the owner
        asset := Asset{
            Type:  assetInput.Type,
            ID:    assetInput.ID,
            Color: assetInput.Color,
            Size:  assetInput.Size,
            Owner: clientID,
        }
        assetJSONasBytes, err := json.Marshal(asset)
        if err != nil {
            return fmt.Errorf("failed to marshal asset into JSON: %v", err)
        }

        // Save asset to private data collection
        // Typical logger, logs to stdout/file in the fabric managed docker container, running this chaincode
        // Look for container name like dev-peer0.org1.example.com-{chaincodename_version}-xyz
        log.Printf("CreateAsset Put: collection %v, ID %v", assetCollection, assetInput.ID)
        err = ctx.GetStub().PutPrivateData(assetCollection, assetInput.ID, assetJSONasBytes)
        if err != nil {
            return fmt.Errorf("failed to put asset into private data collecton: %v", err)
        }

        // Save asset details to collection visible to owning organization
        assetPrivateDetails := AssetPrivateDetails{
            ID:             assetInput.ID,
            AppraisedValue: assetInput.AppraisedValue,
        }

        assetPrivateDetailsAsBytes, err := json.Marshal(assetPrivateDetails) // marshal asset details to JSON
        if err != nil {
            return fmt.Errorf("failed to marshal into JSON: %v", err)
        }

        // Get collection name for this organization.
        orgCollection, err := getCollectionName(ctx)
        if err != nil {
            return fmt.Errorf("failed to infer private collection name for the org: %v", err)
        }

        // Put asset appraised value into owner's org specific private data collection
        log.Printf("Put: collection %v, ID %v", orgCollection, assetInput.ID)
        err = ctx.GetStub().PutPrivateData(orgCollection, assetInput.ID, assetPrivateDetailsAsBytes)
        if err != nil {
            return fmt.Errorf("failed to put asset private details: %v", err)
        }
        return nil
    }

To summarize, the policy definition above for our ``collections_config.json``
allows all peers in Org1 and Org2 to store and transact
with the asset transfer private data ``assetID, color, size, owner`` in their
private database. But only peers in Org1 can store and transact with
the ``appraisedValue`` key data in the Org1 collection ``Org1MSPPrivateCollection`` and only peers
in Org2 can store and transact with the ``appraisedValue`` key data in the Org2 collection ``Org2MSPPrivateCollection``.

As an additional data privacy benefit, since a collection is being used,
only the private data *hashes* go through orderer, not the private data itself,
keeping private data confidential from orderer.

Start the network
-----------------

Now we are ready to step through some commands which demonstrate how to use
private data.

:guilabel:`Try it yourself`

Before installing, defining, and using the asset transfer private data chaincode below,
we need to start the Fabric test network. For the sake of this tutorial, we want
to operate from a known initial state. The following command will kill any active
or stale Docker containers and remove previously generated artifacts.
Therefore let's run the following command to clean up any previous
environments:

.. code:: bash

   cd fabric-samples/test-network
   ./network.sh down

If you have not run through the tutorial before, you will need to vendor the
chaincode dependencies before we can deploy it to the network. Run the
following commands:

.. code:: bash

    cd ../asset-transfer-private-data/chaincode-go/
    GO111MODULE=on go mod vendor
    cd ../../test-network


If you've already run through this tutorial, you'll also want to delete the
underlying Docker containers for the asset transfer private data chaincode. Let's run
the following commands to clean up previous environments:

.. code:: bash

   docker rm -f $(docker ps -a | awk '($2 ~ /dev-peer.*.privatev1.*/) {print $1}')
   docker rmi -f $(docker images | awk '($1 ~ /dev-peer.*.privatev1.*/) {print $3}')

From the ``test-network`` directory, you can use the following command to start
up the Fabric test network with Certificate Authorities and CouchDB:

.. code:: bash

   ./network.sh up createChannel -ca -s couchdb

This command will deploy a Fabric network consisting of a single channel named
``mychannel`` with two organizations (each maintaining one peer node), certificate authorities, and an
ordering service while using CouchDB as the state database. Either LevelDB or
CouchDB may be used with collections. CouchDB was chosen to demonstrate how to
use indexes with private data.

.. note:: For collections to work, it is important to have cross organizational
           gossip configured correctly. Refer to our documentation on :doc:`gossip`,
           paying particular attention to the section on "anchor peers". Our tutorial
           does not focus on gossip given it is already configured in the test network,
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

The chaincode needs to be packaged before it can be installed on our peers.
We can use the `peer lifecycle chaincode package <commands/peerlifecycle.html#peer-lifecycle-chaincode-package>`__ command
to package the private chaincode.

The test network includes two organizations, Org1 and Org2, with one peer each.
Therefore, the chaincode package has to be installed on two peers:

- peer0.org1.example.com
- peer0.org2.example.com

After the chaincode is packaged, we can use the `peer lifecycle chaincode install <commands/peerlifecycle.html#peer-lifecycle-chaincode-install>`__
command to install the private chaincode on each peer.

:guilabel:`Try it yourself`

Assuming you have started the test network, copy and paste the following
environment variables in your CLI to interact with the network and operate as
the Org1 admin. Make sure that you are in the `test-network` directory.

.. code:: bash

    export PATH=${PWD}/../bin:$PATH
    export FABRIC_CFG_PATH=$PWD/../config/
    export CORE_PEER_TLS_ENABLED=true
    export CORE_PEER_LOCALMSPID="Org1MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    export CORE_PEER_ADDRESS=localhost:7051

1. Use the following command to package the private data chaincode.

.. code:: bash

    peer lifecycle chaincode package private.tar.gz --path ../asset-transfer-private-data/chaincode-go/ --lang golang --label privatev1

This command will create a chaincode package named private.tar.gz.

2. Use the following command to install the chaincode package onto the peer
``peer0.org1.example.com``.

.. code:: bash

    peer lifecycle chaincode install private.tar.gz

A successful install command will return the chaincode identifier, similar to
the response below:

.. code:: bash

    2020-09-12 06:25:51.242 CDT [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nJprivatev1:c10ad5913bd3166c1bcc294bb378ecef5f815b516b717f98d8a68ff2f6f4eee9\022\tprivatev1" > 
    2020-09-12 06:25:51.242 CDT [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: privatev1:c10ad5913bd3166c1bcc294bb378ecef5f815b516b717f98d8a68ff2f6f4eee9

3. Now use the CLI as the Org2 admin. Copy and paste the following block of
commands as a group and run them all at once:

.. code:: bash

    export CORE_PEER_LOCALMSPID="Org2MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
    export CORE_PEER_ADDRESS=localhost:9051

4. Run the following command to install the chaincode on the Org2 peer:

.. code:: bash

    peer lifecycle chaincode install private.tar.gz


Approve the chaincode definition
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Each channel member that wants to use the chaincode needs to approve a chaincode
definition for their organization. Since both organizations are going to use the
chaincode in this tutorial, we need to approve the chaincode definition for both
Org1 and Org2 using the `peer lifecycle chaincode approveformyorg <commands/peerlifecycle.html#peer-lifecycle-chaincode-approveformyorg>`__
command. The chaincode definition also includes the private data collection
definition that accompanies the ``asset transfer`` sample. We will provide
the path to the collections JSON file using the ``--collections-config`` flag.

:guilabel:`Try it yourself`

Run the following commands from the ``test-network`` directory to approve a
definition for Org1 and Org2.

1. Use the following command to query your peer for the package ID of the
installed chaincode.

.. code:: bash

    peer lifecycle chaincode queryinstalled

The command will return the same package identifier as the install command.
You should see output similar to the following:

.. code:: bash

    Installed chaincodes on peer:
    Package ID: privatev1:c10ad5913bd3166c1bcc294bb378ecef5f815b516b717f98d8a68ff2f6f4eee9, Label: privatev1

2. Declare the package ID as an environment variable. Paste the package ID of
privatev1 returned by the ``peer lifecycle chaincode queryinstalled`` into
the command below. The package ID may not be the same for all users, so you
need to complete this step using the package ID returned from your console.

.. code:: bash

    export CC_PACKAGE_ID=privatev1:c10ad5913bd3166c1bcc294bb378ecef5f815b516b717f98d8a68ff2f6f4eee9

3. Make sure we are running the CLI as Org1. Copy and paste the following block
of commands as a group into the peer container and run them all at once:

.. code :: bash

    export CORE_PEER_LOCALMSPID="Org1MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    export CORE_PEER_ADDRESS=localhost:7051

4. Use the following command to approve a definition of the asset transfer private data
chaincode for Org1. This command includes a path to the collection definition
file.

.. code:: bash

    export ORDERER_CA=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
    peer lifecycle chaincode approveformyorg -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name private --version 1.0 --collections-config ../asset-transfer-private-data/chaincode-go/collections_config.json --signature-policy "OR('Org1MSP.member','Org2MSP.member')" --package-id $CC_PACKAGE_ID --sequence 1 --tls --cafile $ORDERER_CA

When the command completes successfully you should see something similar to:

.. code:: bash

    2020-09-12 06:33:36.879 CDT [chaincodeCmd] ClientWait -> INFO 001 txid [842e70c59da37a28d8358ec3275d75acff2050714d8c02797642beba81392e5e] committed with status (VALID) at

5. Now use the CLI to switch to Org2. Copy and paste the following block of commands
as a group into the peer container and run them all at once.

.. code:: bash

    export CORE_PEER_LOCALMSPID="Org2MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
    export CORE_PEER_ADDRESS=localhost:9051

6. You can now approve the chaincode definition for Org2:

.. code:: bash

    peer lifecycle chaincode approveformyorg -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name private --version 1.0 --collections-config ../asset-transfer-private-data/chaincode-go/collections_config.json --signature-policy "OR('Org1MSP.member','Org2MSP.member')" --package-id $CC_PACKAGE_ID --sequence 1 --tls --cafile $ORDERER_CA

Commit the chaincode definition
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Once a sufficient number of organizations (in this case, a majority) have
approved a chaincode definition, one organization can commit the definition to
the channel.

Use the `peer lifecycle chaincode commit <commands/peerlifecycle.html#peer-lifecycle-chaincode-commit>`__
command to commit the chaincode definition. This command will also deploy the
collection definition to the channel.

We are ready to use the chaincode after the chaincode definition has been
committed to the channel. 

:guilabel:`Try it yourself`

1. Run the following commands to commit the definition of the asset transfer private
data chaincode to the channel ``mychannel``.

.. code:: bash

    export ORDERER_CA=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
    export ORG1_CA=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export ORG2_CA=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
    peer lifecycle chaincode commit -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name private --version 1.0 --sequence 1 --collections-config ../asset-transfer-private-data/chaincode-go/collections_config.json --signature-policy "OR('Org1MSP.member','Org2MSP.member')" --tls --cafile $ORDERER_CA --peerAddresses localhost:7051 --tlsRootCertFiles $ORG1_CA --peerAddresses localhost:9051 --tlsRootCertFiles $ORG2_CA


When the commit transaction completes successfully you should see something
similar to:

.. code:: bash

    2020-09-12 06:40:35.126 CDT [chaincodeCmd] ClientWait -> INFO 001 txid [701468d079f8deded428a67f84e9bda6f6dcc894b006ee27a4d47b1dafdf5621] committed with status (VALID) at localhost:7051
    2020-09-12 06:40:35.162 CDT [chaincodeCmd] ClientWait -> INFO 002 txid [701468d079f8deded428a67f84e9bda6f6dcc894b006ee27a4d47b1dafdf5621] committed with status (VALID) at localhost:9051

.. _pd-register-identities:

Register identities
-------------------
The private data transfer smart contract supports ownership by individual identities that belong to the network. In our scenario, the owner of the asset will be a member of Org1, while the buyer will belong to Org2. To highlight the connection between the `GetClientIdentity().GetID()` API and the information within a user's certificate, we will register two new identities using the Org1 and Org2 Certificate Authorities (CA's), and then use the CA's to generate each identity's certificate and private key.

First, we need to set the following environment variables to use the Fabric CA client:

.. code :: bash

    export PATH=${PWD}/../bin:${PWD}:$PATH
    export FABRIC_CFG_PATH=$PWD/../config/

We will use the Org1 CA to create the identity asset owner. Set the Fabric CA client home to the MSP of the Org1 CA admin (this identity was generated by the test network script):

.. code:: bash

    export FABRIC_CA_CLIENT_HOME=${PWD}/organizations/peerOrganizations/org1.example.com/

You can register a new owner client identity using the `fabric-ca-client` tool:

.. code:: bash

    fabric-ca-client register --caname ca-org1 --id.name owner --id.secret ownerpw --id.type client --tls.certfiles ${PWD}/organizations/fabric-ca/org1/tls-cert.pem


You can now generate the identity certificates and MSP folder by providing the enroll name and secret to the enroll command:

.. code:: bash

    fabric-ca-client enroll -u https://owner:ownerpw@localhost:7054 --caname ca-org1 -M ${PWD}/organizations/peerOrganizations/org1.example.com/users/owner@org1.example.com/msp --tls.certfiles ${PWD}/organizations/fabric-ca/org1/tls-cert.pem


Run the command below to copy the Node OU configuration file into the owner identity MSP folder.

.. code:: bash

    cp ${PWD}/organizations/peerOrganizations/org1.example.com/msp/config.yaml ${PWD}/organizations/peerOrganizations/org1.example.com/users/owner@org1.example.com/msp/config.yaml


We can now use the Org2 CA to create the buyer identity. Set the Fabric CA client home the Org2 CA admin:

.. code:: bash

    export FABRIC_CA_CLIENT_HOME=${PWD}/organizations/peerOrganizations/org2.example.com/

You can register a new owner client identity using the `fabric-ca-client` tool:

.. code:: bash

    fabric-ca-client register --caname ca-org2 --id.name buyer --id.secret buyerpw --id.type client --tls.certfiles ${PWD}/organizations/fabric-ca/org2/tls-cert.pem


We can now enroll to generate the identity MSP folder:

.. code:: bash

    fabric-ca-client enroll -u https://buyer:buyerpw@localhost:8054 --caname ca-org2 -M ${PWD}/organizations/peerOrganizations/org2.example.com/users/buyer@org2.example.com/msp --tls.certfiles ${PWD}/organizations/fabric-ca/org2/tls-cert.pem


Run the command below to copy the Node OU configuration file into the buyer identity MSP folder.

.. code:: bash

    cp ${PWD}/organizations/peerOrganizations/org2.example.com/msp/config.yaml ${PWD}/organizations/peerOrganizations/org2.example.com/users/buyer@org2.example.com/msp/config.yaml

.. _pd-store-private-data:

Store private data
------------------

Acting as a member of Org1, who is authorized to transact with all of the private data
in the asset transfer private data sample, switch back to an Org1 peer and
submit a request to create an asset:

:guilabel:`Try it yourself`

Copy and paste the following set of commands into your CLI in the `test-network`
directory:

.. code :: bash

    export CORE_PEER_LOCALMSPID="Org1MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/owner@org1.example.com/msp
    export CORE_PEER_ADDRESS=localhost:7051

Invoke the asset transfer (private) ``CreateAsset`` function which
creates an asset with private data ---  assetID ``asset1`` with a color
``green``, size ``20`` and appraisedValue of ``100``. Recall that private data **appraisedValue**
will be stored separately from the private data **assetID, color, size**.
For this reason, the ``CreateAsset`` function calls the ``PutPrivateData()`` API
twice to persist the private data, once for each collection. Also note that
the private data is passed using the ``--transient`` flag. Inputs passed
as transient data will not be persisted in the transaction in order to keep
the data private. Transient data is passed as binary data and therefore when
using CLI it must be base64 encoded. We use an environment variable
to capture the base64 encoded value, and use ``tr`` command to strip off the
problematic newline characters that linux base64 command adds.

Run the following command to define the asset properties:

.. code:: bash

    export ASSET_PROPERTIES=$(echo -n "{\"objectType\":\"asset\",\"assetID\":\"asset1\",\"color\":\"green\",\"size\":20,\"appraisedValue\":100}" | base64 | tr -d \\n)
    peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"CreateAsset","Args":[]}' --transient "{\"asset_properties\":\"$ASSET_PROPERTIES\"}"

You should see results similar to:

.. code:: bash

    [chaincodeCmd] chaincodeInvokeOrQuery->INFO 001 Chaincode invoke successful. result: status:200

Note that command above only targets the Org1 peer. The ``CreateAsset`` transaction writes to two collections, ``assetCollection`` and ``Org1MSPPrivateCollection``.
The ``Org1MSPPrivateCollection`` requires an endorsement from the Org1 peer in order to write to the collection, while the ``assetCollection`` inherits the endorsement policy of the chaincode, ``"OR('Org1MSP.peer','Org2MSP.peer')"``.
An endorsement from the Org1 peer can meet both endorsement policies and is able to create an asset without an endorsement from Org2.

.. _pd-query-authorized:

Query the private data as an authorized peer
--------------------------------------------

Our collection definition allows all members of Org1 and Org2
to have the ``assetID, color, size, and owner`` private data in their side database,
but only peers in Org1 can have Org1's opinion of their ``appraisedValue`` private data in their side
database. As an authorized peer in Org1, we will query both sets of private data.

The first ``query`` command calls the ``ReadAsset`` function which passes
``assetCollection`` as an argument.

.. code-block:: GO

   // ReadAsset reads the information from collection
   func (s *SmartContract) ReadAsset(ctx contractapi.TransactionContextInterface, assetID string) (*Asset, error) {

        log.Printf("ReadAsset: collection %v, ID %v", assetCollection, assetID)
        assetJSON, err := ctx.GetStub().GetPrivateData(assetCollection, assetID) //get the asset from chaincode state
        if err != nil {
            return nil, fmt.Errorf("failed to read asset: %v", err)
        }

        //No Asset found, return empty response
        if assetJSON == nil {
            log.Printf("%v does not exist in collection %v", assetID, assetCollection)
            return nil, nil
        }

        var asset *Asset
        err = json.Unmarshal(assetJSON, &asset)
        if err != nil {
            return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
        }

        return asset, nil

    }

The second ``query`` command calls the ``ReadAssetPrivateDetails``
function which passes ``Org1MSPPrivateDetails`` as an argument.

.. code-block:: GO

   // ReadAssetPrivateDetails reads the asset private details in organization specific collection
   func (s *SmartContract) ReadAssetPrivateDetails(ctx contractapi.TransactionContextInterface, collection string, assetID string) (*AssetPrivateDetails, error) {
        log.Printf("ReadAssetPrivateDetails: collection %v, ID %v", collection, assetID)
        assetDetailsJSON, err := ctx.GetStub().GetPrivateData(collection, assetID) // Get the asset from chaincode state
        if err != nil {
            return nil, fmt.Errorf("failed to read asset details: %v", err)
        }
        if assetDetailsJSON == nil {
            log.Printf("AssetPrivateDetails for %v does not exist in collection %v", assetID, collection)
            return nil, nil
        }

        var assetDetails *AssetPrivateDetails
        err = json.Unmarshal(assetDetailsJSON, &assetDetails)
        if err != nil {
            return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
        }

        return assetDetails, nil
    }

Now :guilabel:`Try it yourself`

We can read the main details of the asset that was created by using the `ReadAsset` function
to query the `assetCollection` collection as Org1:

.. code:: bash

    peer chaincode query -C mychannel -n private -c '{"function":"ReadAsset","Args":["asset1"]}'

When successful, the command will return the following result:

.. code:: bash

    {"objectType":"asset","assetID":"asset1","color":"green","size":20,"owner":"eDUwOTo6Q049b3JnMWFkbWluLE9VPWFkbWluLE89SHlwZXJsZWRnZXIsU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUzo6Q049Y2Eub3JnMS5leGFtcGxlLmNvbSxPPW9yZzEuZXhhbXBsZS5jb20sTD1EdXJoYW0sU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUw=="}

The `"owner"` of the asset is the identity that created the asset by invoking the smart contract. The `GetClientIdentity().GetID()` API reads the common name and issuer of the identity certificate.
You can see that information by decoding the owner string out of base64 format:

.. code:: bash

    echo eDUwOTo6Q049b3JnMWFkbWluLE9VPWFkbWluLE89SHlwZXJsZWRnZXIsU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUzo6Q049Y2Eub3JnMS5leGFtcGxlLmNvbSxPPW9yZzEuZXhhbXBsZS5jb20sTD1EdXJoYW0sU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUw== | base64 --decode

The result will show the common name and issuer of the owner certificate:

.. code:: bash

    x509::CN=org1admin,OU=admin,O=Hyperledger,ST=North Carolina,C=US::CN=ca.org1.example.com,O=org1.example.com,L=Durham,ST=North Carolina,C=US    


Query for the ``appraisedValue`` private data of ``asset1`` as a member of Org1.

.. code:: bash

    peer chaincode query -C mychannel -n private -c '{"function":"ReadAssetPrivateDetails","Args":["Org1MSPPrivateCollection","asset1"]}'

You should see the following result:

.. code:: bash

    {"assetID":"asset1","appraisedValue":100}


Query the private data as an unauthorized peer
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Now we will switch to a member of Org2. Org2 has the asset transfer private data
``assetID, color, size, owner`` in its side database as defined in the assetCollection policy, but does not store the
asset ``appraisedValue`` data for Org1. We will query for both sets of private data.

Switch to a peer in Org2
~~~~~~~~~~~~~~~~~~~~~~~~

Run the following commands to operate as an Org2 member and query the Org2 peer.

:guilabel:`Try it yourself`

.. code:: bash

    export CORE_PEER_LOCALMSPID="Org2MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/buyer@org2.example.com/msp
    export CORE_PEER_ADDRESS=localhost:9051

Query private data Org2 is authorized to
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Peers in Org2 should have the first set of asset transfer private data (``assetID,
color, size and owner``) in their side database and can access it using the
``ReadAsset()`` function which is called with the ``assetCollection``
argument.

:guilabel:`Try it yourself`

.. code:: bash

    peer chaincode query -C mychannel -n private -c '{"function":"ReadAsset","Args":["asset1"]}'

When successful, should see something similar to the following result:

.. code:: json

    {"objectType":"asset","assetID":"asset1","color":"green","size":20,"owner":"eDUwOTo6Q049b3JnMWFkbWluLE9VPWFkbWluLE89SHlwZXJsZWRnZXIsU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUzo6Q049Y2Eub3JnMS5leGFtcGxlLmNvbSxPPW9yZzEuZXhhbXBsZS5jb20sTD1EdXJoYW0sU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUw=="}

Query private data Org2 is not authorized to
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Peers in Org2 do not have the Org1 asset1 ``appraisedValue`` private data in their side database.
When they try to query for this data, they get back a hash of the key matching
the public state but will not have the private state.

:guilabel:`Try it yourself`

.. code:: bash

    peer chaincode query -C mychannel -n private -c '{"function":"ReadAssetPrivateDetails","Args":["Org1MSPPrivateCollection","asset1"]}'

You should see a result similar to:

.. code:: json

    Error: endorsement failure during query. response: status:500 message:"failed to 
    read asset details: GET_STATE failed: transaction ID: d23e4bc0538c3abfb7a6bd4323fd5f52306e2723be56460fc6da0e5acaee6b23: tx
    creator does not have read access permission on privatedata in chaincodeName:private collectionName: Org1MSPPrivateCollection"

Members of Org2 will only be able to see the public hash of the private data.

.. _pd-purge:

Purge Private Data
------------------

For use cases where private data only needs to be on the ledger until it can be
replicated into an off-chain database, it is possible to "purge" the data after
a certain set number of blocks, leaving behind only hash of the data that serves
as immutable evidence of the transaction.

There may be private data including personal or confidential
information, such as the ``appraisedValue`` data in our example, that the transacting
parties don't want disclosed to other organizations on the channel. Thus, it
has a limited lifespan, and can be purged after existing unchanged on the
blockchain for a designated number of blocks using the ``blockToLive`` property
in the collection definition.

Our ``Org1MSPPrivateCollection`` definition has a ``blockToLive``
property value of ``3`` meaning this data will live on the side database for
three blocks and then after that it will get purged. Tying all of the pieces
together, recall this collection definition  ``Org1MSPPrivateCollection``
is associated with the ``appraisedValue`` private data in the  ``CreateAsset()`` function
when it calls the ``PutPrivateData()`` API and passes the
``AssetPrivateDetails`` as an argument.

As we continue with the tutorial by invoking chaincode that adds blocks to the chain, the ``appraisedValue``
information will get purged when we issue the fourth new transaction from the block where we initially created ``asset1``
because you will recall our ``blockToLive`` is set to ``3`` for the ``Org1MSPPrivateCollection`` definition.

.. _pd-transfer-asset:

Transfer the Asset
------------------

Let's see what it takes to transfer ``asset1`` to Org2. In this case, Org2 needs to agree
to buy the asset from Org1, and they need to agree on the ``appraisedValue``. You may be wondering how they can
agree if Org1 keeps their opinion of the ``appraisedValue`` in their private side database. For the answer
to this, lets continue.

:guilabel:`Try it yourself`

Since we need to keep track of how many blocks we add to see if our private data gets purged, open a new terminal window and view the private data logs for this peer by
running the following command. Note the highest block number.

.. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'


Switch back to the terminal with our peer CLI. Now that we are operating
as a member of Org2, we can demonstrate that the asset appraisal is not stored
in Org2MSPPrivateCollection, on the Org2 peer:

.. code:: bash

    peer chaincode query -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"ReadAssetPrivateDetails","Args":["Org2MSPPrivateCollection","asset1"]}'

The empty response shows that the asset1 private details do not exist in buyer (Org2) private collection.

Nor can a member of Org2, read the Org1 private data collection:

.. code:: bash

    peer chaincode query -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"ReadAssetPrivateDetails","Args":["Org1MSPPrivateCollection","asset1"]}'

By setting `"memberOnlyRead": true` in the collection configuration file, we specify that only members of Org1 can read data from the collection. An Org2 member who tries to read the collection would only get the following response:

.. code:: bash

    Error: endorsement failure during query. response: status:500 message:"failed to read from asset details GET_STATE failed: transaction ID: 10d39a7d0b340455a19ca4198146702d68d884d41a0e60936f1599c1ddb9c99d: tx creator does not have read access permission on privatedata in chaincodeName:private collectionName: Org1MSPPrivateCollection"

To transfer an asset, the buyer (recipient) needs to agree to the same ``appraisedValue`` as the asset owner, by calling chaincode function `AgreeToTransfer`. The agreed value will be stored in the `Org2MSPDetailsCollection` collection on the Org2 peer. Run the following command to agree to the appraised value of 100, as Org2:

.. code:: bash

    export ASSET_VALUE=$(echo -n "{\"assetID\":\"asset1\",\"appraisedValue\":100}" | base64 | tr -d \\n)
    peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"AgreeToTransfer","Args":[]}' --transient "{\"asset_value\":\"$ASSET_VALUE\"}"


The buyer can now query the value they agreed to in the Org2 private data collection:

.. code:: bash

    peer chaincode query -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"ReadAssetPrivateDetails","Args":["Org2MSPPrivateCollection","asset1"]}'

The invoke will return the following value:

.. code:: bash

    {"assetID":"asset1","appraisedValue":100}

Switch back to the Terminal window and view the private data logs for this peer
again. You should see the block height increase by 1.

.. code:: bash

    docker logs peer0.org1.example.com 2>&1 | grep -i -a -E 'private|pvt|privdata'

Back in the peer container, let's transfer the asset to Org2. Let's go back to acting as Org1:

.. code:: bash

    export CORE_PEER_LOCALMSPID="Org1MSP"
    export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/owner@org1.example.com/msp
    export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    export CORE_PEER_ADDRESS=localhost:7051

Now that buyer has agreed to buy the asset for appraised value, the owner
from Org1 can read the data added by `AgreeToTransfer` to see buyer identity:

.. code:: bash

    peer chaincode query -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"ReadTransferAgreement","Args":["asset1"]}'

.. code:: bash

    {"assetID":"asset1","buyerID":"eDUwOTo6Q049YnV5ZXIsT1U9Y2xpZW50LE89SHlwZXJsZWRnZXIsU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUzo6Q049Y2Eub3JnMi5leGFtcGxlLmNvbSxPPW9yZzIuZXhhbXBsZS5jb20sTD1IdXJzbGV5LFNUPUhhbXBzaGlyZSxDPVVL"}

The owner from Org1 can now transfer the asset to Org2. To transfer the asset, the owner needs to pass the MSP ID of the new asset owner Org. The transfer
function will read the client ID of the interested buyer user from the transfer agreement:

.. code:: bash

    export ASSET_OWNER=$(echo -n "{\"assetID\":\"asset1\",\"buyerMSP\":\"Org2MSP\"}" | base64 | tr -d \\n)

The owner of the asset needs to initiate the transfer:

.. code:: bash

    peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"TransferAsset","Args":[]}' --transient "{\"asset_owner\":\"$ASSET_OWNER\"}" --peerAddresses localhost:7051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

You can ReadAsset `asset1` to see the results of the transfer:

.. code:: bash

    peer chaincode query -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"ReadAsset","Args":["asset1"]}'

The results will show that the buyer identity now owns the asset:

.. code:: bash

    {"objectType":"asset","assetID":"asset1","color":"green","size":20,"owner":"eDUwOTo6Q049YnV5ZXIsT1U9Y2xpZW50LE89SHlwZXJsZWRnZXIsU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUzo6Q049Y2Eub3JnMi5leGFtcGxlLmNvbSxPPW9yZzIuZXhhbXBsZS5jb20sTD1IdXJzbGV5LFNUPUhhbXBzaGlyZSxDPVVL"}

You can base64 decode the `"owner"` to see that it is the buyer identity:

.. code:: bash

   echo eDUwOTo6Q049YnV5ZXIsT1U9Y2xpZW50LE89SHlwZXJsZWRnZXIsU1Q9Tm9ydGggQ2Fyb2xpbmEsQz1VUzo6Q049Y2Eub3JnMi5leGFtcGxlLmNvbSxPPW9yZzIuZXhhbXBsZS5jb20sTD1IdXJzbGV5LFNUPUhhbXBzaGlyZSxDPVVL | base64 --decode

.. code:: bash

    x509::CN=buyer,OU=client,O=Hyperledger,ST=North Carolina,C=US::CN=ca.org2.example.com,O=org2.example.com,L=Hursley,ST=Hampshire,C=UK

You can also confirm that transfer removed the private details from the Org1 collection:

.. code:: bash

    peer chaincode query -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n private -c '{"function":"ReadAssetPrivateDetails","Args":["Org1MSPPrivateCollection","asset1"]}'

Your query will return empty result, since the asset private data is removed from the Org1 private data collection.
If you go back and look at the block height in your logs, you will see that the private data did not get purged from Org1
due to hitting the fourth block (we only increased by two blocks in transferring the asset to Org2), but rather
because when the asset transfer occurred, Org1 did not meet the ``read`` policy of the ``Org2MSPPrivateCollection``.

.. _pd-indexes:

Using indexes with private data
-------------------------------

Indexes can also be applied to private data collections, by packaging indexes in
the ``META-INF/statedb/couchdb/collections/<collection_name>/indexes`` directory
alongside the chaincode. An example index is available `here <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}//asset-transfer-private-data/chaincode-go/META-INF/statedb/couchdb/collections/assetCollection/indexes/indexOwner.json>`__ .

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
