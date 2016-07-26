// Initialize the Swagger JS plugin
var client = require('swagger-client');

// Point plug-in to the location of the Swagger API description
// Update this variable accordingly, depending on where you're hosing the API description
var api_url = 'http://localhost:5554/rest_api.json';

// Operations currently exposed through IBM Blockchain Swagger description:
//
//      getChain
//      getBlock
//      getTransaction
//      chaincodeDeploy
//      chaincodeInvoke
//      chaincodeQuery
//      registerUser
//      getUserRegistration
//      deleteUserRegistration
//      getUserEnrollmentCertificate
//      getPeers
//      

// Initialize the Swagger-based client, passing in the API URL
var swagger = new client({
  url: api_url,
  success: function() {
    // If connection to the API description is established, report success and
    // proceed
    console.log("Connected to REST API server!\n");

    // Run through some of the Swagger APIs
    runSwaggerAPITest();
  },
  error: function() {
    // If connection to the API description failed, then report error and exit
    console.log("Failed to connect to REST API server.\n");
    console.log("Exiting.\n");
  }
});

// Sample script to trigger APIs exposed in Swagger through Node.js
function runSwaggerAPITest() {
    console.log("Running Swagger API test...\n");
    
    // GET /chain -- retrieve blockchain information
    swagger.Blockchain.getChain({},{responseContentType: 'application/json'}, function(Blockchain){
        console.log("----- Blockchain Retrieved: -----\n");
        console.log(Blockchain);
        console.log("----------\n\n");
    });
    
    // GET /chain/blocks/0 -- retrieve block information for block 0
    swagger.Block.getBlock({'Block': '0'},{responseContentType: 'application/json'}, function(Block){
        console.log("----- Block Retrieved: -----\n");
        console.log(Block);
        console.log("----------\n\n");
    });
    
    // GET /chain/blocks/5 -- retrieve block information for block 5
    swagger.Block.getBlock({'Block': '5'},{responseContentType: 'application/json'}, function(Block){
        console.log("----- Block Retrieved: -----\n");
        console.log(Block);
        console.log("----------\n\n");
    });
    
    // Compose the payload for chaincode deploy transaction
    var chaincodeSpec = {
        "type": "GOLANG",
        "chaincodeID":{
            "path":"github.com/openblockchain/obc-peer/openchain/example/chaincode/chaincode_example02"
        },
        "ctorMsg": {
            "function":"init",
            "args":["a", "100", "b", "200"]
        }
    };
    
    // POST /devops/deploy -- deploy the sample chaincode
    // name (hash) returned is:
    // bb540edfc1ee2ac0f5e2ec6000677f4cd1c6728046d5e32dede7fea11a42f86a6943b76a8f9154f4792032551ed320871ff7b7076047e4184292e01e3421889c
    swagger.Devops.chaincodeDeploy({'ChaincodeSpec': chaincodeSpec},{responseContentType: 'application/json'}, function(Devops){
        console.log("----- Chaincode Deployed: -----\n");
        console.log(Devops);
        console.log("----------\n\n");
        
        // Compose a payload for chaincode invocation transaction
        var chaincodeInvocationSpec = {
            "chaincodeSpec": {
                "type": "GOLANG",
                "chaincodeID":{
                    "name":"bb540edfc1ee2ac0f5e2ec6000677f4cd1c6728046d5e32dede7fea11a42f86a6943b76a8f9154f4792032551ed320871ff7b7076047e4184292e01e3421889c"
                },
                "ctorMsg": {
                    "function":"invoke",
                    "args":["a", "b", "10"]
                }
            }
        };
        
        // POST /devops/invoke -- invoke the sample chaincode
        swagger.Devops.chaincodeInvoke({'ChaincodeInvocationSpec': chaincodeInvocationSpec},{responseContentType: 'application/json'}, function(Devops){
            console.log("----- Devops Invoke Triggered: -----\n");
            console.log(Devops);
            console.log("----------\n\n");
            
            // Compose a payload for chaincode query transaction
            chaincodeInvocationSpec = {
                "chaincodeSpec": {
                    "type": "GOLANG",
                    "chaincodeID":{
                        "name":"bb540edfc1ee2ac0f5e2ec6000677f4cd1c6728046d5e32dede7fea11a42f86a6943b76a8f9154f4792032551ed320871ff7b7076047e4184292e01e3421889c"
                    },
                    "ctorMsg": {
                        "function":"query",
                        "args":["a"]
                    }
                }
            };
            
            // POST /devops/query -- query the sample chaincode for variable "a"
            // Must introduce a wait here, to insure that the invocation transaction had completed, as it returns immediately at runtime
            setTimeout(function() {
                swagger.Devops.chaincodeQuery({'ChaincodeInvocationSpec': chaincodeInvocationSpec},{responseContentType: 'application/json'}, function(Devops){
                console.log("----- Devops Query Triggered: -----\n");
                console.log(Devops);
                console.log("----------\n\n");
                })
            }, 20000);
        });
    });
}