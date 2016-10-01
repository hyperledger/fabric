# APIs - CLI, REST, and Node.js

## Overview

This document covers the available APIs for interacting with a peer node. Three interface choices are provided:

1. [CLI](#cli)
2. [REST API](#rest-api)
3. [Node.js Application](#nodejs-application)
   * [Using Swagger JS Plugin](#using-swagger-js-plugin)
   * [Marbles Demo Application](#marbles-demo-application)
   * [Commercial Paper Demo Application](#commercial-paper-demo-application)

**Note:** If you are working with APIs with security enabled, please review the [security setup instructions](https://github.com/hyperledger/fabric/blob/master/docs/Setup/Chaincode-setup.md#security-setup-optional) before proceeding.

## CLI

To view the currently available CLI commands, execute the following:

    cd /opt/gopath/src/github.com/hyperledger/fabric
    build/bin/peer

You will see output similar to the example below (**NOTE:** rootcommand below is hardcoded in [main.go](https://github.com/hyperledger/fabric/blob/master/main.go). Currently, the build will create a *peer* executable file).

```
    Usage:
      peer [flags]
      peer [command]

    Available Commands:
      version     Print fabric peer version.
      node        node specific commands.
      network     network specific commands.
      chaincode   chaincode specific commands.
      help        Help about any command

    Flags:
      -h, --help[=false]: help for peer
          --logging-level="": Default logging level and overrides, see core.yaml for full syntax
          --test.coverprofile="coverage.cov": Done
      -v, --version[=false]: Show current version number of fabric peer server


    Use "peer [command] --help" for more information about a command.

```

The `peer` command supports several subcommands and flags, as shown above. To
facilitate its use in scripted applications, the `peer` command always
produces a non-zero return code in the event of command failure. Upon success,
many of the subcommands produce a result on **stdout** as shown in the table
below:

Command | **stdout** result in the event of success
--- | ---
`version`          | String form of `peer.version` defined in [core.yaml](https://github.com/hyperledger/fabric/blob/master/peer/core.yaml)
`node start`       | N/A
`node status`      | String form of [StatusCode](https://github.com/hyperledger/fabric/blob/master/protos/server_admin.proto#L36)
`node stop`        | String form of [StatusCode](https://github.com/hyperledger/fabric/blob/master/protos/server_admin.proto#L36)
`network login`    | N/A
`network list`     | The list of network connections to the peer node.
`chaincode deploy` | The chaincode container name (hash) required for subsequent `chaincode invoke` and `chaincode query` commands
`chaincode invoke` | The transaction ID (UUID)
`chaincode query`  | By default, the query result is formatted as a printable string. Command line options support writing this value as raw bytes (-r, --raw), or formatted as the hexadecimal representation of the raw bytes (-x, --hex). If the query response is empty then nothing is output.


### Deploy a Chaincode

Deploy creates the docker image for the chaincode and subsequently deploys the package to the validating peer. An example is below.

`peer chaincode deploy -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Function":"init", "Args": ["a","100", "b", "200"]}'`

The response to the chaincode deploy command will contain the chaincode identifier (hash) which will be required on subsequent `chaincode invoke` and `chaincode query` commands in order to identify the deployed chaincode.

With security enabled, modify the command to include the -u parameter passing the username of a logged in user as follows:

`peer chaincode deploy -u jim -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Function":"init", "Args": ["a","100", "b", "200"]}'`

**Note:** If your GOPATH environment variable contains more than one element, the chaincode must be found in the first one or deployment will fail.

### Verify Results

To verify that the block containing the latest transaction has been added to the blockchain, use the `/chain` REST endpoint from the command line. Target the IP address of either a validating or a non-validating node. In the example below, 172.17.0.2 is the IP address of a validating or a non-validating node and 7050 is the REST interface port defined in [core.yaml](https://github.com/hyperledger/fabric/blob/master/peer/core.yaml).

`curl 172.17.0.2:7050/chain`

An example of the response is below.

```
{
    "height":1,
    "currentBlockHash":"4Yc4yCO95wcpWHW2NLFlf76OGURBBxYZMf3yUyvrEXs5TMai9qNKfy9Yn/=="
}
```

The returned BlockchainInfo message is defined inside [fabric.proto](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L96).

```
message BlockchainInfo {
    uint64 height = 1;
    bytes currentBlockHash = 2;
    bytes previousBlockHash = 3;
}
```

To verify that a specific block is inside the blockchain, use the `/chain/blocks/{Block}` REST endpoint. Likewise, target the IP address of either a validating or a non-validating node on port 7050.

`curl 172.17.0.2:7050/chain/blocks/0`

The returned Block message structure is defined inside [fabric.proto](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L84).

```
message Block {
    uint32 version = 1;
    google.protobuf.Timestamp timestamp = 2;
    repeated Transaction transactions = 3;
    bytes stateHash = 4;
    bytes previousBlockHash = 5;
    bytes consensusMetadata = 6;
    NonHashData nonHashData = 7;
}
```

An example of a returned Block structure is below.

```
{
    "transactions":[{
        "type":1,
        "chaincodeID": {
            "path":"github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
        },
        "payload":"ClwIARJYCk9naXRod...",
        "uuid":"abdcec99-ae5e-415e-a8be-1fca8e38ba71"
    }],
    "stateHash":"PY5YcQRu2g1vjiAqHHshoAhnq8CFP3MqzMslcEAJbnmXDtD+LopmkrUHrPMOGSF5UD7Kxqhbg1XUjmQAi84paw=="
}
```

For additional information on the available CLI commands, please see the [protocol specification](https://github.com/hyperledger/fabric/blob/master/docs/protocol-spec.md) section 6.3 on CLI.

## REST API

You can work with the REST API through any tool of your choice. For example, the curl command line utility or a browser based client such as the Firefox Rest Client or Chrome Postman. You can likewise trigger REST requests directly through [Swagger](http://swagger.io/). You can utilize the Swagger service directly or, if you prefer, you can set up Swagger locally by following the instructions [here](#to-set-up-swagger-ui).

**Note:** The default REST interface port is `7050`. It can be configured in [core.yaml](https://github.com/hyperledger/fabric/blob/master/peer/core.yaml) using the `rest.address` property. If using Vagrant, the REST port mapping is defined in [Vagrantfile](https://github.com/hyperledger/fabric/blob/master/devenv/Vagrantfile).

**Note on constructing a test blockchain** If you want to test the REST API locally, construct a test blockchain by running the TestServerOpenchain_API_GetBlockCount test implemented inside [api_test.go](https://github.com/hyperledger/fabric/blob/master/core/rest/api_test.go). This test will create a test blockchain with 5 blocks. Subsequently restart the peer process.

```
    cd /opt/gopath/src/github.com/hyperledger/fabric/core/rest
    go test -v -run TestServerOpenchain_API_GetBlockCount
```

### REST Endpoints

To learn about the REST API through Swagger, please take a look at the Swagger document [here](https://github.com/hyperledger/fabric/blob/master/core/rest/rest_api.json). You can upload the service description file to the Swagger service directly or, if you prefer, you can set up Swagger locally by following the instructions [here](#to-set-up-swagger-ui).

* [Block](#block)
  * GET /chain/blocks/{Block}
* [Blockchain](#blockchain)
  * GET /chain
* [Chaincode](#chaincode)
    * POST /chaincode
* [Network](#network)
  * GET /network/peers
* [Registrar](#registrar)
  * POST /registrar
  * DELETE /registrar/{enrollmentID}
  * GET /registrar/{enrollmentID}
  * GET /registrar/{enrollmentID}/ecert
  * GET /registrar/{enrollmentID}/tcert
* [Transactions](#transactions)
    * GET /transactions/{UUID}

#### Block

* **GET /chain/blocks/{Block}**

Use the Block API to retrieve the contents of various blocks from the blockchain. The returned Block message structure is defined inside [fabric.proto](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L84).

```
message Block {
    uint32 version = 1;
    google.protobuf.Timestamp Timestamp = 2;
    repeated Transaction transactions = 3;
    bytes stateHash = 4;
    bytes previousBlockHash = 5;
}
```

#### Blockchain

* **GET /chain**

Use the Chain API to retrieve the current state of the blockchain. The returned BlockchainInfo message is defined inside [fabric.proto](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L96).

```
message BlockchainInfo {
    uint64 height = 1;
    bytes currentBlockHash = 2;
    bytes previousBlockHash = 3;
}
```

#### Chaincode

* **POST /chaincode**

Use the /chaincode endpoint to deploy, invoke, and query a target chaincode. This service endpoint implements the [JSON RPC 2.0 specification](http://www.jsonrpc.org/specification) with the payload identifying the desired chaincode operation within the `method` field. The supported methods are `deploy`, `invoke`, and `query`.

The /chaincode endpoint implements the [JSON RPC 2.0 specification](http://www.jsonrpc.org/specification) and as such, must have the required fields of `jsonrpc`, `method`, and in our case `params` supplied within the payload. The client should also add the `id` element within the payload if they wish to receive a response to the request. If the `id` element is missing from the request payload, the request is assumed to be a notification and the server will not produce a response.

The following sample payloads may be used to deploy, invoke, and query a sample chaincode. To deploy a chaincode, supply the [ChaincodeSpec](https://github.com/hyperledger/fabric/blob/master/protos/chaincode.proto#L60) identifying the chaincode to deploy within the request payload.

Chaincode Deployment Request without security enabled:

```
POST host:port/chaincode

{
  "jsonrpc": "2.0",
  "method": "deploy",
  "params": {
    "type": 1,
    "chaincodeID":{
        "path":"github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
    },
    "ctorMsg": {
        "args":["init", "a", "1000", "b", "2000"]
    }
  },
  "id": 1
}
```

To deploy a chaincode with security enabled, supply the `secureContext` element containing the registrationID of a registered and logged in user together with the payload from above.

Chaincode Deployment Request with security enabled (add `secureContext` element):

```
POST host:port/chaincode

{
  "jsonrpc": "2.0",
  "method": "deploy",
  "params": {
    "type": 1,
    "chaincodeID":{
        "path":"github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
    },
    "ctorMsg": {
        "args":["init", "a", "1000", "b", "2000"]
    },
    "secureContext": "lukas"
  },
  "id": 1
}
```

The response to a chaincode deployment request will contain a `status` element confirming successful completion of the request. The response to a successful deployment request will likewise contain the generated chaincode hash which must be used in subsequent invocation and query requests sent to this chaincode.

Chaincode Deployment Response:

```
{
    "jsonrpc": "2.0",
    "result": {
        "status": "OK",
        "message": "52b0d803fc395b5e34d8d4a7cd69fb6aa00099b8fabed83504ac1c5d61a425aca5b3ad3bf96643ea4fdaac132c417c37b00f88fa800de7ece387d008a76d3586"
    },
    "id": 1
}
```

To invoke a chaincode, supply the [ChaincodeSpec](https://github.com/hyperledger/fabric/blob/master/protos/chaincode.proto#L60) identifying the chaincode to invoke within the request payload. Note the chaincode `name` field, which is the hash returned from the deployment request.

Chaincode Invocation Request without security enabled:

```
{
  "jsonrpc": "2.0",
  "method": "invoke",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"52b0d803fc395b5e34d8d4a7cd69fb6aa00099b8fabed83504ac1c5d61a425aca5b3ad3bf96643ea4fdaac132c417c37b00f88fa800de7ece387d008a76d3586"
      },
      "ctorMsg": {
         "args":["invoke", "a", "b", "100"]
      }
  },
  "id": 3
}
```

To invoke a chaincode with security enabled, supply the `secureContext` element containing the registrationID of a registered and logged in user together with the payload from above.

Chaincode Invocation Request with security enabled (add `secureContext` element):

```
{
  "jsonrpc": "2.0",
  "method": "invoke",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"52b0d803fc395b5e34d8d4a7cd69fb6aa00099b8fabed83504ac1c5d61a425aca5b3ad3bf96643ea4fdaac132c417c37b00f88fa800de7ece387d008a76d3586"
      },
      "ctorMsg": {
         "args":["invoke", "a", "b", "100"]
      },
      "secureContext": "lukas"
  },
  "id": 3
}
```

The response to a chaincode invocation request will contain a `status` element confirming successful completion of the request. The response will likewise contain the transaction id number for that specific transaction. The client may use the returned transaction id number to check on the status of the transaction after it has been submitted to the system, as the transaction execution is asynchronous.

Chaincode Invocation Response:

```
{
    "jsonrpc": "2.0",
    "result": {
        "status": "OK",
        "message": "5a4540e5-902b-422d-a6ab-e70ab36a2e6d"
    },
    "id": 3
}
```

To query a chaincode, supply the [ChaincodeSpec](https://github.com/hyperledger/fabric/blob/master/protos/chaincode.proto#L60) identifying the chaincode to query within the request payload. Note the chaincode `name` field, which is the hash returned from the deployment request.

Chaincode Query Request without security enabled:

```
{
  "jsonrpc": "2.0",
  "method": "query",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"52b0d803fc395b5e34d8d4a7cd69fb6aa00099b8fabed83504ac1c5d61a425aca5b3ad3bf96643ea4fdaac132c417c37b00f88fa800de7ece387d008a76d3586"
      },
      "ctorMsg": {
         "args":["query", "a"]
      }
  },
  "id": 5
}
```

To query a chaincode with security enabled, supply the `secureContext` element containing the registrationID of a registered and logged in user together with the payload from above.

Chaincode Query Request with security enabled (add `secureContext` element):

```
{
  "jsonrpc": "2.0",
  "method": "query",
  "params": {
      "type": 1,
      "chaincodeID":{
          "name":"52b0d803fc395b5e34d8d4a7cd69fb6aa00099b8fabed83504ac1c5d61a425aca5b3ad3bf96643ea4fdaac132c417c37b00f88fa800de7ece387d008a76d3586"
      },
      "ctorMsg": {
         "args":["query", "a"]
      },
      "secureContext": "lukas"
  },
  "id": 5
}
```

The response to a chaincode query request will contain a `status` element confirming successful completion of the request. The response will likewise contain an appropriate `message`, as defined by the chaincode. The `message` received depends on the chaincode implementation and may be a string or number indicating the value of a specific chaincode variable.

Chaincode Query Response:

```
{
    "jsonrpc": "2.0",
    "result": {
        "status": "OK",
        "message": "-400"
    },
    "id": 5
}
```

#### Network

* **GET /network/peers**

Use the Network APIs to retrieve information about the network of peer nodes comprising the blockchain network.

The /network/peers endpoint returns a list of all existing network connections for the target peer node. The list includes both validating and non-validating peers. The list of peers is returned as type [`PeersMessage`](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L138), containing an array of [`PeerEndpoint`](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L127).

```
message PeersMessage {
    repeated PeerEndpoint peers = 1;
}
```

```
message PeerEndpoint {
    PeerID ID = 1;
    string address = 2;
    enum Type {
      UNDEFINED = 0;
      VALIDATOR = 1;
      NON_VALIDATOR = 2;
    }
    Type type = 3;
    bytes pkiID = 4;
}
```

```
message PeerID {
    string name = 1;
}
```

#### Registrar

* **POST /registrar**
* **DELETE /registrar/{enrollmentID}**
* **GET /registrar/{enrollmentID}**
* **GET /registrar/{enrollmentID}/ecert**
* **GET /registrar/{enrollmentID}/tcert**

Use the Registrar APIs to manage end user registration with the CA. These API endpoints are used to register a user with the CA, determine whether a given user is registered, and to remove any login tokens for a target user preventing them from executing any further transactions. The Registrar APIs are also used to retrieve user enrollment and transaction certificates from the system.

The /registrar endpoint is used to register a user with the CA. The required Secret payload is defined in [devops.proto](https://github.com/hyperledger/fabric/blob/master/protos/devops.proto#L50).

```
message Secret {
    string enrollId = 1;
    string enrollSecret = 2;
}
```

The response to the registration request is either a confirmation of successful registration or an error, containing a reason for the failure. An example of a valid Secret message to register user 'lukas' is shown below.

```
{
  "enrollId": "lukas",
  "enrollSecret": "NPKYL39uKbkj"
}
```

The GET /registrar/{enrollmentID} endpoint is used to confirm whether a given user is registered with the CA. If so, a confirmation will be returned. Otherwise, an authorization error will result.

The DELETE /registrar/{enrollmentID} endpoint is used to delete login tokens for a target user. If the login tokens are deleted successfully, a confirmation will be returned. Otherwise, an authorization error will result. No payload is required for this endpoint. Note, that registration with the CA is a one time process for a given user, utilizing a single-use registrationID and registrationPW. If the user registration is deleted through this API, the user will not be able to register with the CA a second time.

The GET /registrar/{enrollmentID}/ecert endpoint is used to retrieve the enrollment certificate of a given user from local storage. If the target user has already registered with the CA, the response will include a URL-encoded version of the enrollment certificate. If the target user has not yet registered, an error will be returned. If the client wishes to use the returned enrollment certificate after retrieval, keep in mind that it must be URL-decoded. This can be accomplished with the QueryUnescape method in the "net/url" package.

The /registrar/{enrollmentID}/tcert endpoint retrieves the transaction certificates for a given user that has registered with the certificate authority. If the user has registered, a confirmation message will be returned containing an array of URL-encoded transaction certificates. Otherwise, an error will result. The desired number of transaction certificates is specified with the optional 'count' query parameter. The default number of returned transaction certificates is 1; and 500 is the maximum number of certificates that can be retrieved with a single request. If the client wishes to use the returned transaction certificates after retrieval, keep in mind that they must be URL-decoded. This can be accomplished with the QueryUnescape method in the "net/url" package.

#### Transactions

* **GET /transactions/{UUID}**

Use the /transactions/{UUID} endpoint to retrieve an individual transaction matching the UUID from the blockchain. The returned transaction message is defined inside [fabric.proto](https://github.com/hyperledger/fabric/blob/master/protos/fabric.proto#L28).

```
message Transaction {
    enum Type {
        UNDEFINED = 0;
        CHAINCODE_DEPLOY = 1;
        CHAINCODE_INVOKE = 2;
        CHAINCODE_QUERY = 3;
        CHAINCODE_TERMINATE = 4;
    }
    Type type = 1;
    bytes chaincodeID = 2;
    bytes payload = 3;
    string uuid = 4;
    google.protobuf.Timestamp timestamp = 5;

    ConfidentialityLevel confidentialityLevel = 6;
    bytes nonce = 7;

    bytes cert = 8;
    bytes signature = 9;
}
```

For additional information on the REST endpoints and more detailed examples, please see the [protocol specification](https://github.com/hyperledger/fabric/blob/master/docs/protocol-spec.md) section 6.2 on the REST API.

### To set up Swagger-UI

[Swagger](http://swagger.io/) is a convenient package that allows you to describe and document your REST API in a single file. The REST API is described in [rest_api.json](https://github.com/hyperledger/fabric/blob/master/core/rest/rest_api.json). To interact with the peer node directly through the Swagger-UI, you can upload the available Swagger definition to the [Swagger service](http://swagger.io/). Alternatively, you may set up a Swagger installation on your machine by following the instructions below.

1. You can use Node.js to serve up the rest_api.json locally. To do so, make sure you have Node.js installed on your local machine. If it is not installed, please download the [Node.js](https://nodejs.org/en/download/) package and install it.

2. Install the Node.js http-server package with the command below:

    `npm install http-server -g`

3. Start up an http-server on your local machine to serve up the rest_api.json.

    ```
    cd /opt/gopath/src/github.com/hyperledger/fabric/core/rest
    http-server -a 0.0.0.0 -p 5554 --cors
    ```

4. Make sure that you are successfully able to access the API description document within your browser at this link:

    `http://localhost:5554/rest_api.json`

5. Download the Swagger-UI package with the following command:

    `git clone https://github.com/swagger-api/swagger-ui.git`

6. Navigate to the /swagger-ui/dist directory and click on the index.html file to bring up the Swagger-UI interface inside your browser.

7. Start up the peer node with no connections to a leader or validator as follows.

    ```
    cd /opt/gopath/src/github.com/hyperledger/fabric
    build/bin/peer node start
    ```

8. If you need to construct a test blockchain on the local peer node, run the the TestServerOpenchain_API_GetBlockCount test implemented inside [api_test.go](https://github.com/hyperledger/fabric/blob/master/core/rest/api_test.go). This test will create a blockchain with 5 blocks. Subsequently restart the peer process.

    ```
    cd /opt/gopath/src/github.com/hyperledger/fabric/core/rest
    go test -v -run TestServerOpenchain_API_GetBlockCount
    ```

9. Go back to the Swagger-UI interface inside your browser and load the API description. You should now be able to issue queries against the pre-built blockchain directly from Swagger.

## Node.js Application

You can interface with the peer process from a Node.js application. One way to accomplish that is by relying on the Swagger API description document, [rest_api.json](https://github.com/hyperledger/fabric/blob/master/core/rest/rest_api.json ) and the [swagger-js plugin](https://github.com/swagger-api/swagger-js). Another way to accomplish that relies upon the IBM Blockchain [JS SDK](https://github.com/IBM-Blockchain/ibm-blockchain-js). Use the approach that you find the most convenient.

### [Using Swagger JS Plugin](https://github.com/hyperledger/fabric/blob/master/docs/API/Samples/Sample_1.js)

* Demonstrates interfacing with a peer node from a Node.js application.
* Utilizes the Node.js swagger-js plugin: https://github.com/swagger-api/swagger-js

**To run:**

1. Build and install the [fabric core](https://github.com/hyperledger/fabric/blob/master/README.md#building-the-fabric-core-).

    ```
    cd /opt/gopath/src/github.com/hyperledger/fabric
    make peer
    ```

2. Run a local peer node only (not a complete network) with:

    `build/bin/peer node start`

3. Set up a test blockchain data structure (with 5 blocks only) by running a test from within Vagrant as follows. Subsequently restart the peer process.

    ```
    cd /opt/gopath/src/github.com/hyperledger/fabric/core/rest
    go test -v -run TestServerOpenchain_API_GetBlockCount
    ```

4. Start up an http-server on your local machine to serve up the rest_api.json.

    ```
    npm install http-server -g
    cd /opt/gopath/src/github.com/hyperledger/fabric/core/rest
    http-server -a 0.0.0.0 -p 5554 --cors
    ```

5. Download and unzip [Sample_1.zip](https://github.com/hyperledger/fabric/blob/master/docs/API/Samples/Sample_1.zip)

    ```
    unzip Sample_1.zip -d Sample_1
    cd Sample_1
    ```

6. Update the api_url variable within [openchain.js](https://github.com/hyperledger/fabric/blob/master/docs/API/Samples/Sample_1.js) to the appropriate URL if it is not already the default

    `var api_url = 'http://localhost:5554/rest_api.json';`

7. Run the Node.js app

    `node ./openchain.js`

You will observe several responses on the console and the program will appear to hang for a few moments at the end. This is expected, as is it waiting for the invocation transaction to complete in order to then execute a query. You can take a look at the sample output of the program inside the 'openchain_test' file located in the Sample_1 directory.

### [Marbles Demo Application](https://github.com/IBM-Blockchain/marbles)

* Demonstrates an alternative way of interfacing with a peer node from a Node.js app.
* Demonstrates deploying a Blockchain application as a Bluemix service.

Hold on to your hats everyone, this application is going to demonstrate transferring marbles between two users leveraging IBM Blockchain. We are going to do this in Node.js and a bit of GoLang. The backend of this application will be the GoLang code running in our blockchain network. The chaincode itself will create a marble by storing it to the chaincode state. The chaincode itself is able to store data as a string in a key/value pair setup. Thus we will stringify JSON objects to store more complex structures.

For more inforation on the IBM Blockchain marbles demo, set-up, and instructions, please visit [this page](https://github.com/IBM-Blockchain/marbles).

### [Commercial Paper Demo Application](https://github.com/IBM-Blockchain/cp-web)

* Demonstrates an alternative way of interfacing with a peer node from a Node.js app.
* Demonstrates deploying a Blockchain application as a Bluemix service.

This application is a demonstration of how a commercial paper trading network might be implemented on IBM Blockchain. The components of the demo are:

* An interface for creating new users on the network.
* An interface for creating new commercial papers to trade.
* A Trade Center for buying and selling existing trades.
* A special interface just for auditors of the network to examine trades.

For more inforation on the IBM Blockchain commercial paper demo, set-up, and instructions, please visit [this page](https://github.com/IBM-Blockchain/cp-web).
