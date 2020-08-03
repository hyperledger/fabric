Writing Your First Application
==============================

.. note:: If you're not yet familiar with the fundamental architecture of a
          Fabric network, you may want to visit the :doc:`key_concepts` section
          prior to continuing.

          It is also worth noting that this tutorial serves as an introduction
          to Fabric applications and uses simple smart contracts and
          applications. For a more in-depth look at Fabric applications and
          smart contracts, check out our
          :doc:`developapps/developing_applications` section or the
          :doc:`tutorial/commercial_paper`.

This tutorial provides an introduction to how Fabric applications interact
with deployed blockchain networks. The tutorial uses sample programs built using the
Fabric SDKs -- described in detail in the :doc:`/developapps/application` topic --
to invoke a smart contract which queries and updates the ledger with the smart
contract API -- described in detail in :doc:`/developapps/smartcontract`.
We will also use our sample programs and a deployed Certificate Authority to generate
the X.509 certificates that an application needs to interact with a permissioned
blockchain. 

**About Asset Transfer**

This Asset Transfer (basic) sample demonstrates how to initialize a ledger with assets, query those assets, create
a new asset, query a single asset based on an asset ID, update an existing asset, and transfer an asset to a new owner.
It involves the following two components:

  1. Sample application: which makes calls to the blockchain network, invoking transactions
  implemented in the chaincode (smart contract). The application is located in the following directory:

  .. code:: bash

    asset-transfer-basic/application-javascript

  2. Smart contract itself, implementing the transactions that involve interactions with the
  ledger. The smart contract (chaincode) is located in the following directory:

  .. code:: bash

    asset-transfer-basic/chaincode-(javascript, java, go, typescript)

Please note that for the purposes of this tutorial, the terms chaincode and smart contract are used
interchangeably. For this example, we will be using the javascript chaincode.

We’ll go through three principle steps:

  **1. Setting up a development environment.** Our application needs a network
  to interact with, so we'll deploy a basic network for our smart contracts and
  application.

  .. image:: images/AppConceptsOverview.png

  **2. Explore a sample smart contract.**
  We’ll inspect the sample assetTransfer (javascript) smart contract to learn about the transactions within it,
  and how they are used by an application to query and update the ledger.

  **3. Interact with the smart contract with a sample application.** Our application will
  use the assetTransfer smart contract to create, query, and update assets on the ledger.
  We'll get into the code of the app and the transactions they create, including initializing
  the ledger with assets, querying an asset, querying a range of assets, creating a new asset,
  and transferring an asset to a new owner.

After completing this tutorial you should have a basic understanding of how Fabric
applications and smart contracts work together to manage data on the distributed
ledger of a blockchain network.

Before you begin
----------------

In addition to the standard :doc:`prereqs` for Fabric, this tutorial leverages the Hyperledger Fabric SDK for Node.js. See the Node.js SDK `README <https://github.com/hyperledger/fabric-sdk-node#build-and-test>`__ for a up to date list of prerequisites.

- If you are using macOS, complete the following steps:

  1. Install `Homebrew <https://brew.sh/>`_.
  2. Check the Node SDK `prerequisites <https://github.com/hyperledger/fabric-sdk-node#build-and-test>`_ to find out what level of Node to install.
  3. Run ``brew install node`` to download the latest version of node or choose a specific version, for example: ``brew install node@10`` according to what is supported in the prerequisites.
  4. Run ``npm install``.

- If you are on Windows,  you can install the `windows-build-tools <https://github.com/felixrieseberg/windows-build-tools#readme>`_ with npm which installs all required compilers and tooling by running the following command:

  .. code:: bash

    npm install --global windows-build-tools

- If you are on Linux, you need to install `Python v2.7 <https://www.python.org/download/releases/2.7/>`_, `make <https://www.gnu.org/software/make/>`_, and a C/C++ compiler toolchain such as `GCC <https://gcc.gnu.org/>`_. You can run the following command to install the other tools:

  .. code:: bash

    sudo apt install build-essentials

Set up the blockchain network
-----------------------------

If you've already run through :doc:`test_network` tutorial and have a network up
and running, this tutorial will bring down your running network before
bringing up a new one.


Launch the network
^^^^^^^^^^^^^^^^^^

.. note:: This tutorial demonstrates the JavaScript versions of the Asset Transfer
          smart contract and application, but the ``fabric-samples`` repo also
          contains Go, Java and TypeScript versions of this sample smart contract. To try the
          Go, Java or TypeScript versions, change the ``javascript`` argument
          for ``./network.sh deployCC -ccl javascript`` below to either ``go``, ``java`` or ``typescript``
          and follow the instructions written to the terminal. You may use any chaincode language sample with
          the javascript application sample (e.g javascript application calling go chaincode functions or
          javascript application calling typescript chaincode functions, etc.)

Navigate to the ``test-network`` subdirectory within your local clone of the
``fabric-samples`` repo.

.. code:: bash

  cd fabric-samples/test-network

Launch the Fabric test network using the ``network.sh`` shell script.

.. code:: bash

  ./network.sh up createChannel -c mychannel -ca

This command will deploy the Fabric test network with two peers, three certificate authorities (Orderer, Org1, Org2),
and an ordering service. Instead of using the cryptogen tool, we bring up the test network using Certificate Authorities, 
hence the ``-ca`` flag. Additionally, the org admin user registration is bootstrapped when the Certificate Authority is started.  
In a later step, we will show how the sample application completes the admin enrollment.

Next, let's deploy the chaincode by calling the ``./network.sh`` script with the chaincode name and language options.

.. code:: bash

  ./network.sh deployCC -ccn basic -ccl javascript

.. note:: Behind the scenes, this script uses the chaincode lifecycle to package, install,
          query installed chaincode, approve chaincode for both Org1 and Org2, and finally commit the chaincode.

If the chaincode is successfully deployed, the end of the output in your terminal should look similar to below:

.. code:: bash

  Committed chaincode definition for chaincode 'basic' on channel 'mychannel':
  Version: 1.0, Sequence: 1, Endorsement Plugin: escc, Validation Plugin: vscc, Approvals: [Org1MSP: true, Org2MSP: true]
  ===================== Query chaincode definition successful on peer0.org2 on channel 'mychannel' ===================== 

  ===================== Chaincode initialization is not required =====================


Sample application
^^^^^^^^^^^^^^^^^^
First component of Asset Transfer, the sample application, is available in javascript:

- `JavaScript <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/asset-transfer-basic/application-javascript>`__

Open a new terminal, and navigate to the ``application-javascript`` folder.

.. code:: bash

  cd asset-transfer-basic/application-javascript

This directory contains sample programs that were developed using the Fabric
SDK for Node.js. Run the following command to install the application dependencies.
It may take up to a minute to complete:

.. code:: bash

  npm install

This process is installing the key application dependencies defined in
``package.json``. The most important of which is the ``fabric-network`` class;
it enables an application to use identities, wallets, and gateways to connect to
channels, submit transactions, and wait for notifications. This tutorial also
uses the ``fabric-ca-client`` class to enroll users with their respective
certificate authorities, generating a valid identity which is then used by
``fabric-network`` class methods.

Once ``npm install`` completes, everything is in place to run the application.
Let's take a look at the sample JavaScript application files we will be using
in this tutorial. Run the following command to list the files in this directory:

.. code:: bash

  ls

You should see the following:

.. code:: bash

  app.js                  node_modules            package.json
  enrollAdmin.js          package-lock.json       registerUser.js

.. note:: The first part of the following section involves communication with the Certificate
          Authority. You may find it useful to stream the CA logs when running
          the upcoming programs by opening a new terminal shell and running
          ``docker logs -f ca_org1``.

When we started the Fabric test network back in the first step, an admin user --- literally called ``admin`` ---
was created as the **registrar** for the Certificate Authority (CA). Our first
step is to generate the private key, public key, and X.509 certificate for
``admin`` by having the application call the ``enrollAdmin`` . This process uses a **Certificate
Signing Request** (CSR) --- the private and public key are first generated
locally and the public key is then sent to the CA which returns an encoded
certificate for use by the application. These credentials are then stored
in the wallet, allowing us to act as an administrator for the CA.

Let's run the application and then step through each of the interactions with the smart contract functions. From the
``asset-transfer-basic/application-javascript`` directory, run the following command:

.. code:: bash

  node app.js


First, the application enrolls the admin user
---------------------------------------------

.. note:: It is important to note that enrolling the admin and registering the app user are interactions that
          take place between the application and the Certificate Authority, not between the application and the chaincode.
          If you examine the chaincode in ``asset-transfer-basic/chaincode-javascript/lib`` you will find that the chaincode
          does not contain any functionality that supports enrolling the admin or registering the user.

In the sample application code below, you will see that after getting reference to the
common connection profile path, making sure the connection profile exists, and specifying where to create the wallet,
``enrollAdmin.EnrollAdminUser()`` is executed and the admin credentials are generated from the Certificate Authority.

.. code:: bash

    ...
    // pre-requisites:
    // fabric-sample test-network setup with two peers and an ordering service,
    // the companion chaincode is deployed, approved and committed on the channel mychannel
    async function main() {
        try {
            // load the network configuration
            const ccpPath = path.resolve(__dirname, '..', '..', 'test-network', 'organizations', 'peerOrganizations', 'org1.example.com', 'connection-org1.json');
            const fileExists = fs.existsSync(ccpPath);
            if (!fileExists) {
                throw new Error(`no such file or directory: ${ccpPath}`);
            }
            const ccp = JSON.parse(fs.readFileSync(ccpPath, 'utf8'));

            // Create a new file system based wallet for managing identities.
            const walletPath = path.join(__dirname, 'wallet');
            const wallet = await Wallets.newFileSystemWallet(walletPath);
            console.log(`Wallet path: ${walletPath}`);


            // Steps:
            // Note: Steps 1 & 2 need to done only once in an app-server per blockchain network
            // 1. register & enroll admin user with CA, stores admin identity in local wallet
            await enrollAdmin.EnrollAdminUser();

This command stores the CA administrator's credentials in the ``wallet`` directory.
You can find administrator's certificate and private key in the ``wallet/admin.id``
file.

.. note:: If you decide to start over by taking down the network and bringing it back up again, you will
          have to delete the ``wallet`` folder and its identities prior to re-running the javascript application
          or you will get an error. This happens because the Certificate Authority and its database are taken down
          when the test-network is taken down but the original wallet still remains in the application-javascript directory
          so it must be deleted.  When you re-run the sample javascript application, a new wallet and credentials will 
          be generated.


If you scroll back up to the beginning of the output in your terminal, it should be similar to below:

.. code:: bash

  Wallet path: /Users/<your_username>/fabric-samples/asset-transfer-basic/application-javascript/wallet
  Successfully enrolled admin user and imported it into the wallet
  
Because the admin registration step is bootstrapped when the Certificate Authority
is started, we only need to enroll the admin.  As for the app user, we need the application
to register and enroll the user in the next step.


Second, the application registers and enrolls an application user
-----------------------------------------------------------------

Our ``admin`` is used to work with the Certificate Authority. Now that we have the administrator's
credentials in a wallet, the application registers and enrolls an app user which will be used
to interact with the blockchain. The section of the application code is shown below.

.. code:: bash

  // 2. register & enroll application user with CA, which is used as client identify to make chaincode calls, stores app user identity in local wallet
        await registerUser.RegisterAppUser();

Similar to the admin enrollment, this program uses a CSR to enroll ``appUser`` and
store its credentials alongside those of ``admin`` in the wallet. We now have
identities for two separate users --- ``admin`` and ``appUser`` --- that can be
used by our application.

Scrolling further down in your terminal output, you should see confirmation of the app user registration
similar to this:

.. code:: bash

  Successfully registered and enrolled user appUser and imported it into the wallet

Third, how the sample application interacts with the chaincode
-----------------------------------------------------------

In the prior steps, the application generated the admin and app user credentials and placed them in the wallet.
If the credentials exist and have the correct permissions attributes associated with them, the sample application user
will be able to call chaincode functionality from the sample application after getting reference to the channel name and contract name.

.. note:: Our connection configuration specifies only the peer from your own Org.
          We tell node client sdk to use the service discovery (running on the peer),
          which fetches other peers that are currently online, metadata like relevant endorsement policies
          and any static information it would have otherwise needed to communicate with the rest of the nodes.
          The ``asLocalhost`` set to ``true`` tells it to connect as localhost, since our client is running on same network as the other fabric nodes.
          In deployments where you are not running the client on the same network as the other fabric nodes, 
          the ``asLocalhost`` option would be set to ``false``. 

You will notice that in the following lines of application code, the application is getting reference
to the Contract using the contract name and channel name via Gateway:

.. code:: bash

    //3. Prepare to call chaincode using fabric javascript node sdk
    // Create a new gateway for connecting to our peer node.
        const gateway = new Gateway();
        await gateway.connect(ccp, {
            wallet,
            identity: registerUser.ApplicationUserId,
            discovery: {enabled: true, asLocalhost: true}
        });
        try {
            // Get the network (channel) our contract is deployed to.
            const network = await gateway.getNetwork(myChannel);

            // Get the contract from the network.
            const contract = network.getContract(myChaincodeName);

Fourth, initializing the ledger with some data we can work with
---------------------------------------------------------------

Now that we are at the point where we are actually having the sample application submit transactions, let’s
go through them in sequence, look at the snippets of the application code and the corresponding
chaincode, and what the terminal output looks like.

Sample application -- ``'InitLedger'``

.. code:: bash

    //4. Init a set of asset data on the channel using chaincode 'InitLedger'
    console.log('Submit Transaction: InitLedger creates the initial set of assets on the ledger.');
    await contract.submitTransaction('InitLedger');

Chaincode -- ``'InitLedger'``

.. code:: bash

   async InitLedger(ctx) {
        const assets = [
            {
                ID: 'asset1',
                Color: 'blue',
                Size: 5,
                Owner: 'Tomoko',
                AppraisedValue: 300,
            },
            {
                ID: 'asset2',
                Color: 'red',
                Size: 5,
                Owner: 'Brad',
                AppraisedValue: 400,
            },
            {
                ID: 'asset3',
                Color: 'green',
                Size: 10,
                Owner: 'Jin Soo',
                AppraisedValue: 500,
            },
            {
                ID: 'asset4',
                Color: 'yellow',
                Size: 10,
                Owner: 'Max',
                AppraisedValue: 600,
            },
            {
                ID: 'asset5',
                Color: 'black',
                Size: 15,
                Owner: 'Adriana',
                AppraisedValue: 700,
            },
            {
                ID: 'asset6',
                Color: 'white',
                Size: 15,
                Owner: 'Michel',
                AppraisedValue: 800,
            },
        ];

        for (const asset of assets) {
            asset.docType = 'asset';
            await ctx.stub.putState(asset.ID, Buffer.from(JSON.stringify(asset)));
            console.info(`Asset ${asset.ID} initialized`);
        }
    }

The terminal output entry should look similar to below:

.. code:: bash

  Submit Transaction: InitLedger creates the initial set of assets on the ledger.

Fifth, the rest of the transaction submission sequence from the sample application
----------------------------------------------------------------------------------

First, a word about querying the ledger.

Each peer in a blockchain network hosts a copy of the `ledger <./ledger/ledger.html>`_. An application
program can view the most recent data from the ledger using read-only invocations of
a smart contract running on your peers called a query.

Here is a simplified representation of how a query works:

.. image:: tutorial/write_first_app.diagram.1.png

The most common queries involve the current values of data in the ledger -- its
`world state <./ledger/ledger.html#world-state>`_. The world state is
represented as a set of key-value pairs, and applications can query data for a
single key or multiple keys. Moreover, you can use complex queries to read the
data on the ledger when you use CouchDB as your state database and model your data in JSON.
This can be very helpful when looking for all assets that match certain keywords
with particular values; all assets with a particular owner, for example.

Below, the sample application is just getting all the assets that we populated in the prior
step when we initialized the ledger with data.

Sample application -- ``'GetAllAssets'``

.. code:: bash

  // GetAllAssets returns all the current assets on the ledger
     let result = await contract.evaluateTransaction('GetAllAssets');
     console.log(`Evaluate Transaction: GetAllAssets, result: ${prettyJSONString(result.toString())}`);

Chaincode -- ``'GetAllAssets'``

.. code:: bash

   // GetAllAssets returns all assets found in the world state.
    async GetAllAssets(ctx) {
        const allResults = [];
        // range query with empty string for startKey and endKey does an open-ended query of all assets in the chaincode namespace.
        const iterator = await ctx.stub.getStateByRange('', '');
        let result = await iterator.next();
        while (!result.done) {
            const strValue = Buffer.from(result.value.value.toString()).toString('utf8');
            let record;
            try {
                record = JSON.parse(strValue);
            } catch (err) {
                console.log(err);
                record = strValue;
            }
            allResults.push({ Key: result.value.key, Record: record });
            result = await iterator.next();
        }
        return JSON.stringify(allResults);
    }


The terminal output should look like this:

.. code:: json

    Evaluate Transaction: GetAllAssets, result: [
    {
      "Key": "asset1",
      "Record": {
        "ID": "asset1",
        "Color": "blue",
        "Size": 5,
        "Owner": "Tomoko",
        "AppraisedValue": 300,
        "docType": "asset"
      }
    },
    {
      "Key": "asset2",
      "Record": {
        "ID": "asset2",
        "Color": "red",
        "Size": 5,
        "Owner": "Brad",
        "AppraisedValue": 400,
        "docType": "asset"
      }
    },
    {
      "Key": "asset3",
      "Record": {
        "ID": "asset3",
        "Color": "green",
        "Size": 10,
        "Owner": "Jin Soo",
        "AppraisedValue": 500,
        "docType": "asset"
      }
    },
    {
      "Key": "asset4",
      "Record": {
        "ID": "asset4",
        "Color": "yellow",
        "Size": 10,
        "Owner": "Max",
        "AppraisedValue": 600,
        "docType": "asset"
      }
    },
    {
      "Key": "asset5",
      "Record": {
        "ID": "asset5",
        "Color": "black",
        "Size": 15,
        "Owner": "Adriana",
        "AppraisedValue": 700,
        "docType": "asset"
      }
    },
    {
      "Key": "asset6",
      "Record": {
        "ID": "asset6",
        "Color": "white",
        "Size": 15,
        "Owner": "Michel",
        "AppraisedValue": 800,
        "docType": "asset"
      }
    }
  ]

In the next part of the sequence, the sample application submits a transaction to create 'asset13'
and then immediately evaluates (query) for 'asset13'.

Sample application -- ``'CreateAsset'`` and ``'ReadAsset'``

.. code:: bash

  console.log('\n***********************');
  console.log('Submit Transaction: CreateAsset asset13');
  //CreateAsset creates an asset with ID asset13, color yellow, owner Tom, size 5 and appraisedValue of 1300
  await contract.submitTransaction('CreateAsset', 'asset13', 'yellow', '5', 'Tom', '1300');

  console.log('Evaluate Transaction: ReadAsset asset13');
  // ReadAsset returns an asset with given assetID
  result = await contract.evaluateTransaction('ReadAsset', 'asset13');
  console.log(`  result: ${prettyJSONString(result.toString())}`);

Chaincode -- ``'CreateAsset'`` and ``'ReadAsset'``

.. code:: bash

  // CreateAsset issues a new asset to the world state with given details.
  async CreateAsset(ctx, id, color, size, owner, appraisedValue) {
    const asset = {
        ID: id,
        Color: color,
        Size: size,
        Owner: owner,
        AppraisedValue: appraisedValue,
    };
    return ctx.stub.putState(id, Buffer.from(JSON.stringify(asset)));
  }

  // ReadAsset returns the asset stored in the world state with given id.
  async ReadAsset(ctx, id) {
    const assetJSON = await ctx.stub.getState(id); // get the asset from chaincode state
    if (!assetJSON || assetJSON.length === 0) {
        throw new Error(`The asset ${id} does not exist`);
    }
    return assetJSON.toString();
  }

Terminal output:

.. code:: bash

  ***********************
  Submit Transaction: CreateAsset asset13
  Evaluate Transaction: ReadAsset asset13
    result: {
    "ID": "asset13",
    "Color": "yellow",
    "Size": "5",
    "Owner": "Tom",
    "AppraisedValue": "1300"
  }

.. note:: In the application and chaincode snippets above, it is important
          to note that the sample application submits the ``'CreateAsset'``
          transaction with the same type and number of arguments the chaincode
          is expecting, and in the correct sequence. In this case, the transaction
          name and correctly sequenced arguments are: ``'CreateAsset'``, ``'asset13'``,
          ``'yellow'``, ``'5'``, ``'Tom'``, ``'1300'`` because the corresponding chaincode
          CreateAsset is expecting the correct sequence and type of arguments that define the asset object:
          sequence: ID, Color, Size, Owner, and AppraisedValue

          type: ID (string), Color (string), Size (int), Owner (string), AppraisedValue (int).


In the next part of the sequence, the sample application evaluates to see if
``asset1`` exists, which will return a boolean value of true, because we populated
the ledger with ``asset1`` when we initialized the ledger with assets. You may recall
that the original appraised value of ``asset1`` was ``300``.
The application then submits a transaction to update ``asset1`` with a new appraised value, and
then immediately evaluates to read ``asset1`` from the ledger to show the new appraised value of ``350``.

Sample application -- ``'AssetExists'``, ``'UpdateAsset'``, and ``'ReadAsset'``

.. code:: bash

  // AssetExists returns 'true' if an asset with given assetID exist
  result = await contract.evaluateTransaction('AssetExists', 'asset1');
  console.log(`  result: ${prettyJSONString(result.toString())}`);

  console.log('Submit Transaction: UpdateAsset asset1, new AppraisedValue : 350');
  // UpdateAsset updates an existing asset with new properties. Same args as CreateAsset
  await contract.submitTransaction('UpdateAsset', 'asset1', 'blue', '5', 'Tomoko', '350');

  console.log('Evaluate Transaction: ReadAsset asset1');
  result = await contract.evaluateTransaction('ReadAsset', 'asset1');
  console.log(`  result: ${prettyJSONString(result.toString())}`);

Chaincode -- ``'AssetExists'``, ``'UpdateAsset'``, and ``'ReadAsset'``

.. code:: bash 

 // AssetExists returns true when asset with given ID exists in world state.
    async AssetExists(ctx, id) {
        const assetJSON = await ctx.stub.getState(id);
        return assetJSON && assetJSON.length > 0;
    }
 // UpdateAsset updates an existing asset in the world state with provided parameters.
    async UpdateAsset(ctx, id, color, size, owner, appraisedValue) {
        const exists = await this.AssetExists(ctx, id);
        if (!exists) {
            throw new Error(`The asset ${id} does not exist`);
        }

        // overwriting original asset with new asset
        const updatedAsset = {
            ID: id,
            Color: color,
            Size: size,
            Owner: owner,
            AppraisedValue: appraisedValue,
        };
        return ctx.stub.putState(id, Buffer.from(JSON.stringify(updatedAsset)));
    }
  // ReadAsset returns the asset stored in the world state with given id.
  async ReadAsset(ctx, id) {
      const assetJSON = await ctx.stub.getState(id); // get the asset from chaincode state
      if (!assetJSON || assetJSON.length === 0) {
          throw new Error(`The asset ${id} does not exist`);
      }
      return assetJSON.toString();
  }

Terminal Output:

.. code:: bash

  ***********************
  Evaluate Transaction: AssetExists asset1
    result: true
  Submit Transaction: UpdateAsset asset1, new AppraisedValue : 350
  Evaluate Transaction: ReadAsset asset1
    result: {
    "ID": "asset1",
    "Color": "blue",
    "Size": "5",
    "Owner": "Tomoko",
    "AppraisedValue": "350"
  }

In this part of the sequence, the sample application attempts to submit
a ``'UdpateAsset'`` transaction for an asset that we know does not exist (``asset70``).
We expect that we will get an error because you cannot update an asset that does not exist,
which is why it is a good idea to check if an asset exists prior to attempting an
asset update or deletion.

Sample application -- ``'UpdateAsset'``

.. code:: bash

   try {
        console.log('\nSubmit Transaction: UpdateAsset asset70');
        //Non existing asset asset70 should throw Error
        await contract.submitTransaction('UpdateAsset', 'asset70', 'blue', '5', 'Tomoko', '300');
    } catch (error) {
        console.log(`Expected an error on UpdateAsset of non-existing Asset: ${error}`);
    }

Chaincode -- ``'UpdateAsset'``

.. code:: bash

  // UpdateAsset updates an existing asset in the world state with provided parameters.
  async UpdateAsset(ctx, id, color, size, owner, appraisedValue) {
      const exists = await this.AssetExists(ctx, id);
      if (!exists) {
          throw new Error(`The asset ${id} does not exist`);
      }

        // overwriting original asset with new asset
        const updatedAsset = {
            ID: id,
            Color: color,
            Size: size,
            Owner: owner,
            AppraisedValue: appraisedValue,
        };
        return ctx.stub.putState(id, Buffer.from(JSON.stringify(updatedAsset)));
    }

Terminal output:

.. code:: bash

  Submit Transaction: UpdateAsset asset70
  2020-08-02T11:12:12.322Z - error: [Transaction]: Error: No valid responses from any peers. Errors:
    peer=peer0.org1.example.com:7051, status=500, message=error in simulation: transaction returned with failure: Error: The asset asset70 does not exist
    peer=peer0.org2.example.com:9051, status=500, message=error in simulation: transaction returned with failure: Error: The asset asset70 does not exist
  Expected an error on UpdateAsset of non-existing Asset: Error: No valid responses from any peers. Errors:
    peer=peer0.org1.example.com:7051, status=500, message=error in simulation: transaction returned with failure: Error: The asset asset70 does not exist
    peer=peer0.org2.example.com:9051, status=500, message=error in simulation: transaction returned with failure: Error: The asset asset70 does not exist


In this final part of the sample application transaction sequence, the application
submits a transaction to transfer an existing asset to a new owner and then reads the
asset back from the ledger to display the new owner ``Tom``.

Sample application -- ``'TransferAsset'``, and ``'ReadAsset'``

.. code:: bash

  console.log('Submit Transaction: TransferAsset asset1 from owner Tomoko > owner Tom');
  // TransferAsset transfers an asset with given ID to new owner Tom
  await contract.submitTransaction('TransferAsset', 'asset1', 'Tom');

  console.log('Evaluate Transaction: ReadAsset asset1');
  result = await contract.evaluateTransaction('ReadAsset', 'asset1');
  console.log(`  result: ${prettyJSONString(result.toString())}`);

Chaincode -- ``'TransferAsset'``, and ``'ReadAsset'``

.. code:: bash

  // TransferAsset updates the owner field of asset with given id in the world state.
  async TransferAsset(ctx, id, newOwner) {
      const assetString = await this.ReadAsset(ctx, id);
      const asset = JSON.parse(assetString);
      asset.Owner = newOwner;
      return ctx.stub.putState(id, Buffer.from(JSON.stringify(asset)));
  }
   // ReadAsset returns the asset stored in the world state with given id.
  async ReadAsset(ctx, id) {
      const assetJSON = await ctx.stub.getState(id); // get the asset from chaincode state
      if (!assetJSON || assetJSON.length === 0) {
          throw new Error(`The asset ${id} does not exist`);
      }
      return assetJSON.toString();
  }

Terminal output:

.. code:: bash

  ***********************
  Submit Transaction: TransferAsset asset1 from owner Tomoko > owner Tom
  Evaluate Transaction: ReadAsset asset1
    result: {
    "ID": "asset1",
    "Color": "blue",
    "Size": "5",
    "Owner": "Tom",
    "AppraisedValue": "350"
  }


A closer look
-------------

Let's take a closer look at how the sample javascript application uses the APIs provided by the
`Fabric Node SDK <https://hyperledger.github.io/fabric-sdk-node/>`__ to
interact with our Fabric network. Use an editor (e.g. atom or visual studio) to
open ``app.js`` located in the ``asset-transfer-basic/application-javascript`` directory.

The application starts by bringing in scope two key classes from the
``fabric-network`` module; ``Wallets`` and ``Gateway``. These classes
will be used to locate the ``appUser`` identity in the wallet, and use it to
connect to the network:

.. code:: bash

  const { Gateway, Wallets } = require('fabric-network');

First, the program uses the Wallet class to get our application user from our file system.

.. code:: bash

  const identity = await wallet.get(registerUser.ApplicationUserId);

Once the program has an identity, it uses the Gateway class to connect to our network.

.. code:: bash

  const gateway = new Gateway();
  await gateway.connect(ccp, { wallet, identity: registerUser.ApplicationUserID, discovery: { enabled: true, asLocalhost: true } });

``ccpPath`` describes the path to the connection profile that our application will use
to connect to our network. The connection profile was loaded from inside the
``fabric-samples/test-network`` directory and parsed as a JSON file:

.. code:: bash

  const ccpPath = path.resolve(__dirname, '..', '..', 'test-network','organizations','peerOrganizations','org1.example.com', 'connection-org1.json');

If you'd like to understand more about the structure of a connection profile,
and how it defines the network, check out
`the connection profile topic <./developapps/connectionprofile.html>`_.

A network can be divided into multiple channels, and the next important line of
code connects the application to a particular channel within the network,
``mychannel``, where our smart contract was deployed. Note that we assigned constants
near the top of the sample application to account for the channel name and the contract name:

.. code:: bash

  const myChannel = 'mychannel';
  const myChaincodeName = 'basic';

.. code:: bash

  const network = await gateway.getNetwork('myChannel');

Within this channel, we can access the asset-transfer ('basic') smart contract to interact
with the ledger:

.. code:: bash

  const contract = network.getContract(myChaincodeName);

Within asset-transfer ('basic') there are many different **transactions**, and our application
initially uses the ``InitLedger`` transaction to populate the ledger world state with
data:

.. code:: bash

  await contract.submitTransaction('InitLedger');

The ``evaluateTransaction`` method represents one of the simplest interactions
with a smart contract in blockchain network. It simply picks a peer defined in
the connection profile and sends the request to it, where it is evaluated. The
smart contract queries the assets on the peer's copy of the ledger and returns
the result to the application. This interaction does not result in an update of
the ledger.

``submitTransaction`` is much more sophisticated than ``evaluateTransaction``.
Rather than interacting with a single peer, the SDK will send the
``submitTransaction`` proposal to every required organization's peer in the
blockchain network. Each of these peers will execute the requested smart
contract using this proposal, to generate a transaction response which it signs
and returns to the SDK. The SDK collects all the signed transaction responses
into a single transaction, which it then sends to the orderer. The orderer
collects and sequences transactions from every application into a block of
transactions. It then distributes these blocks to every peer in the network,
where every transaction is validated and committed. Finally, the SDK is
notified, allowing it to return control to the application.

.. note:: ``submitTransaction`` also includes a listener that checks to make
          sure the transaction has been validated and committed to the ledger.
          Applications should either utilize a commit listener, or
          leverage an API like ``submitTransaction`` that does this for you.
          Without doing this, your transaction may not have been successfully
          ordered, validated, and committed to the ledger.

``submitTransaction`` does all this for the application! The process by which
the application, smart contract, peers and ordering service work together to
keep the ledger consistent across the network is called consensus, and it is
explained in detail in this `section <./peers/peers.html>`_.

Updating the ledger
-------------------

From an application perspective, updating the ledger is simple. An application
submits a transaction to the blockchain network, and when it has been validated
and committed, the application receives a notification that the transaction has been successful.
Behind the scenes, this involves the process of consensus whereby the different components
of the blockchain network work together to ensure that every proposed update to the ledger
is valid and performed in an agreed and consistent order.

.. image:: tutorial/write_first_app.diagram.2.png

The asset-transfer ('basic') smart contract
-------------------------
The smart contract sample is available in the following languages:

- `Golang <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/asset-transfer-basic/chaincode-go>`__
- `Java <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/asset-transfer-basic/chaincode-java>`__
- `JavaScript <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/asset-transfer-basic/chaincode-javascript>`__
- `Typescript <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/asset-transfer-basic/chaincode-typescript>`__

Clean up
--------

When you are finished using the asset-transfer sample, you can bring down the test
network using ``network.sh`` script.


.. code:: bash

  ./network.sh down

This command will bring down the CAs, peers, and ordering node of the network
that we created. Note that all of the data on the ledger will be lost.
If you want to go through the tutorial again, you will start from a clean initial state.

Summary
-------

Now that we’ve seen how the sample application and chaincode are written and how they interact with each other, you should have a pretty
good sense of how applications interact with a blockchain network using a smart
contract to query or update the ledger. You’ve seen the basics of the roles
smart contracts, APIs, and the SDK play in queries and updates and you should
have a feel for how different kinds of applications could be used to perform
other business tasks and operations.

Additional resources
--------------------

As we said in the introduction, we have a whole section on
:doc:`developapps/developing_applications` that includes in-depth information on
smart contracts, process and data design, a tutorial using a more in-depth
Commercial Paper `tutorial <./tutorial/commercial_paper.html>`_ and a large
amount of other material relating to the development of applications.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
