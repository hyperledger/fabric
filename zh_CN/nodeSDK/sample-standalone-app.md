# Node.js Standalone Application in Vagrant

This section describes how to run a sample standalone Node.js application which interacts with a Hyperledger fabric blockchain.

* If you haven't already done so, see [Setting Up The Application Development Environment](app-developer-env-setup.md) to get your environment set up.  The remaining steps assume that you are running **inside the vagrant environment**.

* Issue the following commands to build the Node.js Client SDK:

```
   cd /opt/gopath/src/github.com/hyperledger/fabric
   make node-sdk
```

* Start the membership services and peer processes.  We run the peer in dev mode for simplicity.

```
   cd /opt/gopath/src/github.com/hyperledger/fabric/build/bin
   membersrvc > membersrvc.log 2>&1&
   peer node start --peer-chaincodedev > peer.log 2>&1&
```

* Build and run chaincode example 2:

```
   cd /opt/gopath/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02
   go build
   CORE_CHAINCODE_ID_NAME=mycc CORE_PEER_ADDRESS=0.0.0.0:7051 ./chaincode_example02 > log 2>&1&
```

* Put the following sample app in a file named **app.js** in the */tmp* directory.  Take a moment (now or later) to read the comments and code to begin to learn the Node.js Client SDK APIs.

   You may retrieve the [sample application](https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/app.js) file:
```
    cd /tmp
    curl -o app.js https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/app.js
```
* Run **npm** to install Hyperledger Fabric Node.js SDK in the `/tmp` directory
```
     npm install /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
```

* To run the application :

```
     CORE_CHAINCODE_ID_NAME=mycc CORE_PEER_ADDRESS=0.0.0.0:7051 MEMBERSRVC_ADDRESS=0.0.0.0:7054 DEPLOY_MODE=dev node app
```

Congratulations!  You've successfully run your first Hyperledger fabric application.

