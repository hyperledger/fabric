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