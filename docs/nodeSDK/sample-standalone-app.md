# Node.js Standalone Application in Vagrant

This section describes how to run a sample standalone Node.js application which interacts with a Hyperledger fabric blockchain.

1. If you haven't already done so, see [Setting Up The Application Development Environment](app-developer-env-setup.md) to get your environment set up.  The remaining steps assume that you are running **inside the vagrant environment**.

2. Issue the following commands to build the Node.js Client SDK:

```
   cd /opt/gopath/src/github.com/hyperledger/fabric
   make node-sdk
```

3. Start the membership services and peer processes.  We run the peer in dev mode for simplicity.

```
   cd /opt/gopath/src/github.com/hyperledger/fabric/build/bin
   membersrvc > membersrvc.log 2>&1&
   peer node start --peer-chaincodedev > peer.log 2>&1&
```

4. Build and run chaincode example 2:

```
   cd /opt/gopath/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02
   go build
   CORE_CHAINCODE_ID_NAME=mycc CORE_PEER_ADDRESS=0.0.0.0:30303 ./chaincode_example02 > log 2>&1&
```

5. Put the following sample app in a file named **app.js** in the */tmp* directory.  Take a moment (now or later) to read the comments and code to begin to learn the Node.js Client SDK APIs.

   You may retrieve the sample application file:
```
    curl -o webapp.js https://raw.githubusercontent.com/hyperledger/fabric/master/examples/sdk/node/standalone-app.js
```
   The sample app:

```javascript
   /*
    * A simple application utilizing the Node.js Client SDK to:
    * 1) Enroll a user
    * 2) User deploys chaincode
    * 3) User queries chaincode
    */
   // "HFC" stands for "Hyperledger fabric Client"
   var hfc = require("hfc");

   var chain, user, chaincodeID;

   // Create a chain object used to interact with the chain.
   // You can name it anything you want as it is only used by client.
   chain = hfc.newChain("mychain");
   // Initialize the place to store sensitive private key information
   chain.setKeyValStore( hfc.newFileKeyValStore('/tmp/keyValStore') );
   // Set the URL to membership services and to the peer
   chain.setMemberServicesUrl("grpc://localhost:7054");
   chain.addPeer("grpc://localhost:7051");
   // The following is required when the peer is started in dev mode
   // (i.e. with the '--peer-chaincodedev' option)
   chain.setDevMode(true);

   // Begin by enrolling the user
   enroll();

   // Enroll a user.
   function enroll() {
      console.log("enrolling user admin ...");
      // Enroll "admin" which is preregistered in the membersrvc.yaml
      chain.enroll("admin", "Xurw3yU9zI0l", function(err, admin) {
         if (err) {
            console.log("ERROR: failed to register admin: %s",err);
            process.exit(1);
         }
         user = admin;
         deploy();
      });
   }

   // Deploy chaincode
   function deploy() {
      console.log("deploying chaincode; please wait ...");
      // Construct the deploy request
      var req = {
          chaincodeName: 'mycc',
          fcn: "init",
          args: ["a", "100", "b", "200"]
      };
      // Issue the deploy request and listen for events
      var tx = user.deploy(req);
      tx.on('complete', function(results) {
          // Deploy request completed successfully
          console.log("deploy complete; results: %j",results);
          // Set the testChaincodeID for subsequent tests
          chaincodeID = results.chaincodeID;
          query();
      });
      tx.on('error', function(err) {
          console.log("Failed to deploy chaincode: request=%j, error=%k",req,err);
          process.exit(1);
      });

   }

   // Query chaincode
   function query() {
      console.log("querying chaincode ...");
      // Construct a query request
      var req = {
         chaincodeID: chaincodeID,
         fcn: "query",
         args: ["a"]
      };
      // Issue the query request and listen for events
      var tx = user.query(req);
      tx.on('complete', function (results) {
         console.log("query completed successfully; results=%j",results);
         process.exit(0);
      });
      tx.on('error', function (err) {
         console.log("Failed to query chaincode: request=%j, error=%k",req,err);
         process.exit(1);
      });
   }
```

6. Run the application as follows:

```
   cd /tmp
   node app
```

7. Congratulations!  You've successfully run your first Hyperledger fabric application.

