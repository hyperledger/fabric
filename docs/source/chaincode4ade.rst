Writing Your First Chaincode
============================

What is Chaincode?
------------------

Chaincode is a program, written in `Go <https://golang.org>`_, `Node.js <https://nodejs.org>`_,
or `Java <https://java.com/en/>`_ that implements a prescribed interface.
Chaincode runs in a separate process from the peer and initializes and manages
the ledger state through transactions submitted by applications.

A chaincode typically handles business logic agreed to by members of the
network, so it similar to a "smart contract". A chaincode can be invoked to update or query
the ledger in a proposal transaction. Given the appropriate permission, a chaincode
may invoke another chaincode, either in the same channel or in different channels, to access its state.
Note that, if the called chaincode is on a different channel from the calling chaincode,
only read query is allowed. That is, the called chaincode on a different channel is only a ``Query``,
which does not participate in state validation checks in subsequent commit phase.

In the following sections, we will explore chaincode through the eyes of an
application developer. We'll present a asset-transfer chaincode sample walkthrough,
and the purpose of each method in the Fabric Contract API. If you
are a network operator who is deploying a chaincode to running network,
visit the :doc:`deploy_chaincode` tutorial and the :doc:`chaincode_lifecycle`
concept topic.

This tutorial provides an overview of the high level APIs provided by the Fabric Contract API.
To learn more about developing smart contracts using the Fabric contract API, visit the :doc:`developapps/smartcontract` topic.

Fabric Contract API
-------------------

The ``fabric-contract-api`` provides the contract interface, a high level API for application developers to implement Smart Contracts.
Within Hyperledger Fabric, Smart Contracts are also known as Chaincode. Working with this API provides a high level entry point to writing business logic.
Documentation of the Fabric Contract API for different languages can be found at the links below:

  - `Go <https://godoc.org/github.com/hyperledger/fabric-contract-api-go/contractapi>`__
  - `Node.js <https://hyperledger.github.io/fabric-chaincode-node/{BRANCH}/api/>`__
  - `Java <https://hyperledger.github.io/fabric-chaincode-java/{BRANCH}/api/org/hyperledger/fabric/contract/package-summary.html>`__


Note that when using the contract api, each chaincode function that is called is passed a transaction context "ctx", from which
you can get the chaincode stub (GetStub() ), which has functions to access the ledger (e.g. GetState() ) and make requests
to update the ledger (e.g. PutState() ). You can learn more at the language-specific links below.

  - `Go <https://godoc.org/github.com/hyperledger/fabric-chaincode-go/shim#Chaincode>`__
  - `Node.js <https://hyperledger.github.io/fabric-chaincode-node/{BRANCH}/api/fabric-shim.ChaincodeInterface.html>`__
  - `Java <https://hyperledger.github.io/fabric-chaincode-java/{BRANCH}/api/org/hyperledger/fabric/shim/Chaincode.html>`__


In this tutorial using Go chaincode, we will demonstrate the use of these APIs
by implementing a asset-transfer chaincode application that manages simple "assets".

.. _Asset Transfer Chaincode:

Asset Transfer Chaincode
------------------------
Our application is a basic sample chaincode to initialize a ledger with assets, create, read, update, and delete assets, check to see
if an asset exists, and transfer assets from one owner to another.

Choosing a Location for the Code
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

If you haven't been doing programming in Go, you may want to make sure that
you have `Go <https://golang.org>`_ installed and your system properly configured. We assume
you are using a version that supports modules.

Now, you will want to create a directory for your chaincode application.

To keep things simple, let's use the following command:

.. code:: bash

  // atcc is shorthand for asset transfer chaincode
  mkdir atcc && cd atcc

Now, let's create the module and the source file that we'll fill in with code:

.. code:: bash

  go mod init atcc
  touch atcc.go

Housekeeping
^^^^^^^^^^^^

First, let's start with some housekeeping. As with every chaincode, it implements the
`fabric-contract-api interface <https://godoc.org/github.com/hyperledger/fabric-contract-api-go/contractapi>`_,
so let's add the Go import statements for the necessary dependencies for our chaincode. We'll import the
fabric contract api package and define our SmartContract.

.. code:: go

  package main

  import (
    "fmt"
    "log"
    "github.com/hyperledger/fabric-contract-api-go/contractapi"
  )

  // SmartContract provides functions for managing an Asset
  type SmartContract struct {
    contractapi.Contract
  }

Next, let's add a struct ``Asset`` to represent simple assets on the ledger.
Note the JSON annotations, which will be used to marshal the asset to JSON which is stored on the ledger.

.. code:: go

  // Asset describes basic details of what makes up a simple asset
  type Asset struct {
    ID             string `json:"ID"`
    Color          string `json:"color"`
    Size           int    `json:"size"`
    Owner          string `json:"owner"`
    AppraisedValue int    `json:"appraisedValue"`
  }

Initializing the Chaincode
^^^^^^^^^^^^^^^^^^^^^^^^^^

Next, we'll implement the ``InitLedger`` function to populate the ledger with some initial data.

.. code:: go

  // InitLedger adds a base set of assets to the ledger
  func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
    assets := []Asset{
      {ID: "asset1", Color: "blue", Size: 5, Owner: "Tomoko", AppraisedValue: 300},
      {ID: "asset2", Color: "red", Size: 5, Owner: "Brad", AppraisedValue: 400},
      {ID: "asset3", Color: "green", Size: 10, Owner: "Jin Soo", AppraisedValue: 500},
      {ID: "asset4", Color: "yellow", Size: 10, Owner: "Max", AppraisedValue: 600},
      {ID: "asset5", Color: "black", Size: 15, Owner: "Adriana", AppraisedValue: 700},
      {ID: "asset6", Color: "white", Size: 15, Owner: "Michel", AppraisedValue: 800},
    }

    for _, asset := range assets {
      assetJSON, err := json.Marshal(asset)
      if err != nil {
          return err
      }

      err = ctx.GetStub().PutState(asset.ID, assetJSON)
      if err != nil {
          return fmt.Errorf("failed to put to world state. %v", err)
      }
    }

    return nil
  }

Next, we write a function to create an asset on the ledger that does not yet exist. When writing chaincode, it
is a good idea to check for the existence of something on the ledger prior to taking an action on it, as is demonstrated
in the ``CreateAsset`` function below.


.. code:: go

    // CreateAsset issues a new asset to the world state with given details.
    func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface, id string, color string, size int, owner string, appraisedValue int) error {
      exists, err := s.AssetExists(ctx, id)
      if err != nil {
        return err
      }
      if exists {
        return fmt.Errorf("the asset %s already exists", id)
      }

      asset := Asset{
        ID:             id,
        Color:          color,
        Size:           size,
        Owner:          owner,
        AppraisedValue: appraisedValue,
      }
      assetJSON, err := json.Marshal(asset)
      if err != nil {
        return err
      }

      return ctx.GetStub().PutState(id, assetJSON)
    }

Now that we have populated the ledger with some initial assets and created an asset,
let's write a function ``ReadAsset`` that allows us to read an asset from the ledger.

.. code:: go

  // ReadAsset returns the asset stored in the world state with given id.
  func (s *SmartContract) ReadAsset(ctx contractapi.TransactionContextInterface, id string) (*Asset, error) {
    assetJSON, err := ctx.GetStub().GetState(id)
    if err != nil {
      return nil, fmt.Errorf("failed to read from world state: %v", err)
    }
    if assetJSON == nil {
      return nil, fmt.Errorf("the asset %s does not exist", id)
    }

    var asset Asset
    err = json.Unmarshal(assetJSON, &asset)
    if err != nil {
      return nil, err
    }

    return &asset, nil
  }

Now that we have assets on our ledger we can interact with, let's write a chaincode function
``UpdateAsset`` that allows us to update attributes of the asset that we are allowed to change.

.. code:: go

  // UpdateAsset updates an existing asset in the world state with provided parameters.
  func (s *SmartContract) UpdateAsset(ctx contractapi.TransactionContextInterface, id string, color string, size int, owner string, appraisedValue int) error {
    exists, err := s.AssetExists(ctx, id)
    if err != nil {
      return err
    }
    if !exists {
      return fmt.Errorf("the asset %s does not exist", id)
    }

    // overwriting original asset with new asset
    asset := Asset{
      ID:             id,
      Color:          color,
      Size:           size,
      Owner:          owner,
      AppraisedValue: appraisedValue,
    }
    assetJSON, err := json.Marshal(asset)
    if err != nil {
      return err
    }

    return ctx.GetStub().PutState(id, assetJSON)
  }

There may be cases where we need the ability to delete an asset from the ledger,
so let's write a ``DeleteAsset`` function to handle that requirement.

.. code:: go

  // DeleteAsset deletes an given asset from the world state.
  func (s *SmartContract) DeleteAsset(ctx contractapi.TransactionContextInterface, id string) error {
    exists, err := s.AssetExists(ctx, id)
    if err != nil {
      return err
    }
    if !exists {
      return fmt.Errorf("the asset %s does not exist", id)
    }

    return ctx.GetStub().DelState(id)
  }


We said earlier that is was a good idea to check to see if an asset exists before
taking an action on it, so let's write a function called ``AssetExists`` to implement that requirement.

.. code:: go

  // AssetExists returns true when asset with given ID exists in world state
  func (s *SmartContract) AssetExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
    assetJSON, err := ctx.GetStub().GetState(id)
    if err != nil {
      return false, fmt.Errorf("failed to read from world state: %v", err)
    }

    return assetJSON != nil, nil
  }

Next, we'll write a function we'll call ``TransferAsset`` that enables the transfer of an asset from one owner to another.

.. code:: go

  // TransferAsset updates the owner field of asset with given id in world state.
  func (s *SmartContract) TransferAsset(ctx contractapi.TransactionContextInterface, id string, newOwner string) error {
    asset, err := s.ReadAsset(ctx, id)
    if err != nil {
      return err
    }

    asset.Owner = newOwner
    assetJSON, err := json.Marshal(asset)
    if err != nil {
      return err
    }

    return ctx.GetStub().PutState(id, assetJSON)
  }

Let's write a function we'll call ``GetAllAssets`` that enables the querying of the ledger to
return all of the assets on the ledger.

.. code:: go

  // GetAllAssets returns all assets found in world state
  func (s *SmartContract) GetAllAssets(ctx contractapi.TransactionContextInterface) ([]*Asset, error) {
    // range query with empty string for startKey and endKey does an
    // open-ended query of all assets in the chaincode namespace.
    resultsIterator, err := ctx.GetStub().GetStateByRange("", "")
    if err != nil {
      return nil, err
    }
    defer resultsIterator.Close()

    var assets []*Asset
    for resultsIterator.HasNext() {
      queryResponse, err := resultsIterator.Next()
      if err != nil {
        return nil, err
      }

      var asset Asset
      err = json.Unmarshal(queryResponse.Value, &asset)
      if err != nil {
        return nil, err
      }
      assets = append(assets, &asset)
    }

    return assets, nil
  }

.. _Chaincode Sample:


.. Note:: The full chaincode sample below is presented as a way to
          to keep this tutorial as clear and straightforward as possible. In a
          real-world implementation, it is likely that packages will be segmented
          where a ``main`` package imports the chaincode package to allow for easy unit testing.
          To see what this looks like, see the asset-transfer `Go chaincode <https://github.com/hyperledger/fabric-samples/tree/master/asset-transfer-basic/chaincode-go>`__
          in fabric-samples. If you look at ``assetTransfer.go``, you will see that
          it contains ``package main`` and imports ``package chaincode`` defined in ``smartcontract.go`` and
          located at ``fabric-samples/asset-transfer-basic/chaincode-go/chaincode/``.



Pulling it All Together
^^^^^^^^^^^^^^^^^^^^^^^

Finally, we need to add the ``main`` function, which will call the
`ContractChaincode.Start <https://godoc.org/github.com/hyperledger/fabric-contract-api-go/contractapi#ContractChaincode.Start>`_
function. Here's the whole chaincode program source.

.. code:: go

  package main

  import (
    "encoding/json"
    "fmt"
    "log"

    "github.com/hyperledger/fabric-contract-api-go/contractapi"
  )

  // SmartContract provides functions for managing an Asset
  type SmartContract struct {
    contractapi.Contract
  }

  // Asset describes basic details of what makes up a simple asset
  type Asset struct {
    ID             string `json:"ID"`
    Color          string `json:"color"`
    Size           int    `json:"size"`
    Owner          string `json:"owner"`
    AppraisedValue int    `json:"appraisedValue"`
  }

  // InitLedger adds a base set of assets to the ledger
  func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
    assets := []Asset{
      {ID: "asset1", Color: "blue", Size: 5, Owner: "Tomoko", AppraisedValue: 300},
      {ID: "asset2", Color: "red", Size: 5, Owner: "Brad", AppraisedValue: 400},
      {ID: "asset3", Color: "green", Size: 10, Owner: "Jin Soo", AppraisedValue: 500},
      {ID: "asset4", Color: "yellow", Size: 10, Owner: "Max", AppraisedValue: 600},
      {ID: "asset5", Color: "black", Size: 15, Owner: "Adriana", AppraisedValue: 700},
      {ID: "asset6", Color: "white", Size: 15, Owner: "Michel", AppraisedValue: 800},
    }

    for _, asset := range assets {
      assetJSON, err := json.Marshal(asset)
      if err != nil {
        return err
      }

      err = ctx.GetStub().PutState(asset.ID, assetJSON)
      if err != nil {
        return fmt.Errorf("failed to put to world state. %v", err)
      }
    }

    return nil
  }

  // CreateAsset issues a new asset to the world state with given details.
  func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface, id string, color string, size int, owner string, appraisedValue int) error {
    exists, err := s.AssetExists(ctx, id)
    if err != nil {
      return err
    }
    if exists {
      return fmt.Errorf("the asset %s already exists", id)
    }

    asset := Asset{
      ID:             id,
      Color:          color,
      Size:           size,
      Owner:          owner,
      AppraisedValue: appraisedValue,
    }
    assetJSON, err := json.Marshal(asset)
    if err != nil {
      return err
    }

    return ctx.GetStub().PutState(id, assetJSON)
  }

  // ReadAsset returns the asset stored in the world state with given id.
  func (s *SmartContract) ReadAsset(ctx contractapi.TransactionContextInterface, id string) (*Asset, error) {
    assetJSON, err := ctx.GetStub().GetState(id)
    if err != nil {
      return nil, fmt.Errorf("failed to read from world state: %v", err)
    }
    if assetJSON == nil {
      return nil, fmt.Errorf("the asset %s does not exist", id)
    }

    var asset Asset
    err = json.Unmarshal(assetJSON, &asset)
    if err != nil {
      return nil, err
    }

    return &asset, nil
  }

  // UpdateAsset updates an existing asset in the world state with provided parameters.
  func (s *SmartContract) UpdateAsset(ctx contractapi.TransactionContextInterface, id string, color string, size int, owner string, appraisedValue int) error {
    exists, err := s.AssetExists(ctx, id)
    if err != nil {
      return err
    }
    if !exists {
      return fmt.Errorf("the asset %s does not exist", id)
    }

    // overwriting original asset with new asset
    asset := Asset{
      ID:             id,
      Color:          color,
      Size:           size,
      Owner:          owner,
      AppraisedValue: appraisedValue,
    }
    assetJSON, err := json.Marshal(asset)
    if err != nil {
      return err
    }

    return ctx.GetStub().PutState(id, assetJSON)
  }

  // DeleteAsset deletes an given asset from the world state.
  func (s *SmartContract) DeleteAsset(ctx contractapi.TransactionContextInterface, id string) error {
    exists, err := s.AssetExists(ctx, id)
    if err != nil {
      return err
    }
    if !exists {
      return fmt.Errorf("the asset %s does not exist", id)
    }

    return ctx.GetStub().DelState(id)
  }

  // AssetExists returns true when asset with given ID exists in world state
  func (s *SmartContract) AssetExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
    assetJSON, err := ctx.GetStub().GetState(id)
    if err != nil {
      return false, fmt.Errorf("failed to read from world state: %v", err)
    }

    return assetJSON != nil, nil
  }

  // TransferAsset updates the owner field of asset with given id in world state.
  func (s *SmartContract) TransferAsset(ctx contractapi.TransactionContextInterface, id string, newOwner string) error {
    asset, err := s.ReadAsset(ctx, id)
    if err != nil {
      return err
    }

    asset.Owner = newOwner
    assetJSON, err := json.Marshal(asset)
    if err != nil {
      return err
    }

    return ctx.GetStub().PutState(id, assetJSON)
  }

  // GetAllAssets returns all assets found in world state
  func (s *SmartContract) GetAllAssets(ctx contractapi.TransactionContextInterface) ([]*Asset, error) {
    // range query with empty string for startKey and endKey does an
    // open-ended query of all assets in the chaincode namespace.
    resultsIterator, err := ctx.GetStub().GetStateByRange("", "")
    if err != nil {
      return nil, err
    }
    defer resultsIterator.Close()

    var assets []*Asset
    for resultsIterator.HasNext() {
      queryResponse, err := resultsIterator.Next()
      if err != nil {
        return nil, err
      }

      var asset Asset
      err = json.Unmarshal(queryResponse.Value, &asset)
      if err != nil {
        return nil, err
      }
      assets = append(assets, &asset)
    }

    return assets, nil
  }

  func main() {
    assetChaincode, err := contractapi.NewChaincode(&SmartContract{})
    if err != nil {
      log.Panicf("Error creating asset-transfer-basic chaincode: %v", err)
    }

    if err := assetChaincode.Start(); err != nil {
      log.Panicf("Error starting asset-transfer-basic chaincode: %v", err)
    }
  }

Chaincode access control
------------------------

Chaincode can utilize the client (submitter) certificate for access
control decisions with ``ctx.GetStub().GetCreator()``. Additionally
the Fabric Contract API provides extension APIs that extract client identity
from the submitter's certificate that can be used for access control decisions,
whether that is based on client identity itself, or the org identity,
or on a client identity attribute.

For example an asset that is represented as a key/value may include the
client's identity as part of the value (for example as a JSON attribute
indicating that asset owner), and only this client may be authorized
to make updates to the key/value in the future. The client identity
library extension APIs can be used within chaincode to retrieve this
submitter information to make such access control decisions.


.. _vendoring:

Managing external dependencies for chaincode written in Go
----------------------------------------------------------
Your Go chaincode depends on Go packages (like the chaincode shim) that are not
part of the standard library. The source to these packages must be included in
your chaincode package when it is installed to a peer. If you have structured
your chaincode as a module, the easiest way to do this is to "vendor" the
dependencies with ``go mod vendor`` before packaging your chaincode.

.. code:: bash

  go mod tidy
  go mod vendor

This places the external dependencies for your chaincode into a local ``vendor``
directory.

Once dependencies are vendored in your chaincode directory, ``peer chaincode package``
and ``peer chaincode install`` operations will then include code associated with the
dependencies into the chaincode package.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
