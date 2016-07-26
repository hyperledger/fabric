## Hyperledger Fabric Client (HFC) SDK for Node.js

The Hyperledger Fabric Client (HFC) SDK provides a powerful and easy to use API to interact with a Hyperledger Fabric blockchain.

This document assumes that you already have set up a Node.js development environment. If not, go [here](https://nodejs.org/en/download/package-manager/) to download and install Node.js for your OS. You'll also want the latest version of `npm` installed. For that, execute `sudo npm install npm -g` to get the latest version.

### Installing the hfc module

We publish the `hfc` node module to `npm`. To install `hfc` from npm simply execute the following command:

```
npm install -g hfc
```

## Overview

The sections in this document are as follows:

* The [Getting Started](#getting-started) section is intended to help you quickly get a feel for HFC, how to use it, and some of its common capabilities. This is demonstrated by example.

* The [Getting Set Up](#getting-set-up) section shows you how to set up up your environment and how to [run the unit tests](#running-unit-tests). Looking at the implementation of the unit tests will also help you learn more about the APIs by example, including asset management and confidentiality.

## Getting Started
This purpose of this section is to help you quickly get a feel for HFC and how you may use it. It is not intended to demonstrate all of its power, but to demonstrate common use cases by example.

### Some basic terminology
First, there is some basic terminology you should understand. In order to transact on a hyperledger blockchain, you must first have an identity which has been both **registered** and **enrolled**.

Think of **registration** as *issuing a user invitation* to join a blockchain. It consists of adding a new user name (also called an *enrollment ID*) to the membership service configuration. This can be done programatically with the `Member.register` method, or by adding the enrollment ID directly to the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml) configuration file.

Think of **enrollment** as *accepting a user invitation* to join a blockchain. This is always done by the entity that will transact on the blockchain. This can be done programatically via the `Member.enroll` method.

### Learn by example
The best way to quickly learn HFC is by example.

The following example demonstrates a typical web app. The web app authenticates a user and then transacts on a blockchain on behalf of that user.

```
/**
 * This example shows how to do the following in a web app.
 * 1) At initialization time, enroll the web app with the block chain.
 *    The identity must have already been registered.
 * 2) At run time, after a user has authenticated with the web app:
 *    a) register and enroll an identity for the user;
 *    b) use this identity to deploy, query, and invoke a chaincode.
 */

// To include the package from your hyperledger fabric directory:
//    var hfc = require("myFabricDir/sdk/node");
// To include the package from npm:
//      var hfc = require('hfc');
var hfc = require('hfc');

// Create a client chain.
// The name can be anything as it is only used internally.
var chain = hfc.newChain("targetChain");

// Configure the KeyValStore which is used to store sensitive keys
// as so it is important to secure this storage.
// The FileKeyValStore is a simple file-based KeyValStore, but you
// can easily implement your own to store whereever you want.
chain.setKeyValStore( hfc.newFileKeyValStore('/tmp/keyValStore') );

// Set the URL for member services
chain.setMemberServicesUrl("grpc://localhost:50051");

// Add a peer's URL
chain.addPeer("grpc://localhost:30303");

// Enroll "WebAppAdmin" which is already registered because it is
// listed in fabric/membersrvc/membersrvc.yaml with its one time password.
// If "WebAppAdmin" has already been registered, this will still succeed
// because it stores the state in the KeyValStore
// (i.e. in '/tmp/keyValStore' in this sample).
chain.enroll("WebAppAdmin", "DJY27pEnl16d", function(err, webAppAdmin) {
   if (err) return console.log("ERROR: failed to register %s: %s",err);
   // Successfully enrolled WebAppAdmin during initialization.
   // Set this user as the chain's registrar which is authorized to register other users.
   chain.setRegistrar(webAppAdmin);
   // Now begin listening for web app requests
   listenForUserRequests();
});

// Main web app function to listen for and handle requests
function listenForUserRequests() {
   for (;;) {
      // WebApp-specific logic goes here to await the next request.
      // ...
      // Assume that we received a request from an authenticated user
      // 'userName', and determined that we need to invoke the chaincode
      // with 'chaincodeID' and function named 'fcn' with arguments 'args'.
      handleUserRequest(userName,chaincodeID,fcn,args);
   }
}

// Handle a user request
function handleUserRequest(userName, chaincodeID, fcn, args) {
   // Register and enroll this user.
   // If this user has already been registered and/or enrolled, this will
   // still succeed because the state is kept in the KeyValStore
   // (i.e. in '/tmp/keyValStore' in this sample).
   var registrationRequest = {
        enrollmentID: userName,
        // Customize account & affiliation
        account: "bank_a",
        affiliation: "00001"
   };
   chain.registerAndEnroll( registrationRequest, function(err, user) {
      if (err) return console.log("ERROR: %s",err);
      // Issue an invoke request
      var invokeRequest = {
        // Name (hash) required for invoke
        chaincodeID: chaincodeID,
        // Function to trigger
        fcn: fcn,
        // Parameters for the invoke function
        args: args
     };
     // Invoke the request from the user object.
     var tx = user.invoke(invokeRequest);
     // Listen for the 'submitted' event
     tx.on('submitted', function(results) {
        console.log("submitted invoke: %j",results);
     });
     // Listen for the 'complete' event.
     tx.on('complete', function(results) {
        console.log("completed invoke: %j",results;
     });
     // Listen for the 'error' event.
     tx.on('error', function(err) {
        console.log("error on invoke: %j",err);
     });
   });
}
```

## Getting Set Up

First, you'll want to have a running peer node and CA. You can follow the instructions for setting up a network [here](https://github.com/hyperledger/fabric/blob/master/docs/Setup/ca-setup.md#Operating-the-CA), and start a single peer node and CA.

### Chaincode Deployment Directory Structure

To have the chaincode deployment succeed in network mode, you must properly set up the chaincode project outside of your Hyperledger Fabric source tree. The following instructions will demonstrate how to properly set up the directory structure to deploy *chaincode_example02* in network mode.

The chaincode project must be placed under the `$GOPATH/src` directory. For example, the [chaincode_example02](https://github.com/hyperledger/fabric/blob/master/examples/chaincode/go/chaincode_example02/chaincode_example02.go) project should be placed under `$GOPATH/src/` as shown below.

```
mkdir -p $GOPATH/src/github.com/chaincode_example02/
cd $GOPATH/src/github.com/chaincode_example02
curl GET https://raw.githubusercontent.com/hyperledger/fabric/master/examples/chaincode/go/chaincode_example02/chaincode_example02.go > chaincode_example02.go
```

After you have placed your chaincode project under the `$GOPATH/src`, you will need to vendor the dependencies. From the directory containing your chaincode source, run the following commands:

```
go get -u github.com/kardianos/govendor
cd $GOPATH/src/github.com/chaincode_example02
govendor init
govendor fetch github.com/hyperledger/fabric
```

Now, execute `go build` to verify that all of the chaincode dependencies are present.

```
go build
```

Next, we will switch over to the node sdk directory in the fabric repo to run the node sdk tests, to make sure you have everything properly set up. Verify that the [chain-tests.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/chain-tests.js) unit test file points to the correct chaincode project path. The default directory is set to `github.com/chaincode_example02/` as shown below. If you placed the sample chaincode elsewhere, then you will need to change that.

```
// Path to the local directory containing the chaincode project under $GOPATH
var testChaincodePath = "github.com/chaincode_example02/";
```

**Note:** You will need to run `npm install` the first time you run the sdk tests, in order to install all of the dependencies. Set the `DEPLOY_MODE` environment variable to `net` and run the chain-tests as follows:

```
cd $GOPATH/src/github.com/hyperledger/fabric/sdk/node
npm install
export DEPLOY_MODE='net'
node test/unit/chain-tests.js | node_modules/.bin/tap-spec
```

### Enabling TLS

If you wish to configure TLS with the Membership Services server, the following steps are required:

- Modify `$GOPATH/src/github.com/hyperledger/fabric/membersrvc/membersrvc.yaml` as follows:

```
server:
     tls:
        cert:
            file: "/var/hyperledger/production/.membersrvc/tlsca.cert"
        key:
            file: "/var/hyperledger/production/.membersrvc/tlsca.priv"
```

To specify to the Membership Services (TLS) Certificate Authority (TLSCA) what X.509 v3 Certificate (with a corresponding Private Key) to use:

- Modify `$GOPATH/src/github.com/hyperledger/fabric/peer/core.yaml` as follows:

```
peer:
    pki:
        tls:
            enabled: true
            rootcert:
                file: "/var/hyperledger/production/.membersrvc/tlsca.cert"
```

To configure the peer to connect to the Membership Services server over TLS (otherwise, the connection will fail).

- Bootstrap your Membership Services and the peer. This is needed in order to have the file *tlsca.cert* generated by the member services.

- Copy `/var/hyperledger/production/.membersrvc/tlsca.cert` to `$GOPATH/src/github.com/hyperledger/fabric/sdk/node`.

*Note:* If you cleanup the folder `/var/hyperledger/production` then don't forget to copy again the *tlsca.cert* file as described above.

## Running Unit Tests
HLC includes a set of unit tests implemented with the [tape framework](https://github.com/substack/tape). The [unit test script](https://github.com/hyperledger/fabric/blob/master/sdk/node/bin/run-unit-tests.sh) builds and runs both the membership service server and the peer node for you, therefore you do not have to start those manually.

### Running the SDK unit tests
HFC includes a set of unit tests implemented with the [tape framework](https://github.com/substack/tape). To run the unit tests, execute the following commands:

    cd $GOPATH/src/github.com/hyperledger/fabric
    make node-sdk-unit-tests

The following are brief descriptions of each of the unit tests that are being run.

#### registrar
The [registrar.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/registrar.js) test case exercises registering users with Membership Services. It also tests registering a designated registrar user which can then register additional users.

#### chain-tests
The [chain-tests.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/chain-tests.js) test case exercises the [chaincode_example02.go](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go/chaincode_example02) chaincode when it has been deployed in both development mode and network mode.

#### asset-mgmt
The [asset-mgmt.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/asset-mgmt.js) test case exercises the [asset_management.go](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go/asset_management) chaincode when it has been deployed in both development mode and network mode.

#### asset-mgmt-with-roles
The [asset-mgmt-with-roles.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/asset-mgmt-with-roles.js) test case exercises the [asset_management_with_roles.go](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go/asset_management_with_roles) chaincode when it has been deployed in both development mode and network mode.

#### Troublingshooting
If you see errors stating that the client has already been registered/enrolled, keep in mind that you can perform the enrollment process only once, as the enrollmentSecret is a one-time-use password. You will see these errors if you have performed a user registration/enrollment and subsequently deleted the cryptographic tokens stored on the client side. The next time you try to enroll, errors similar to the ones below will be seen.

   ```
   Error: identity or token do not match
   ```
   ```
   Error: user is already registered
   ```

To address this, remove any stored cryptographic material from the CA server by following the instructions [here](https://github.com/hyperledger/fabric/blob/master/docs/Setup/Chaincode-setup.md#removing-temporary-files-when-security-is-enabled). You will also need to remove any of the cryptographic tokens stored on the client side by deleting the KeyValStore directory. That directory is configurable and is set to `/tmp/keyValStore` within the unit tests.
