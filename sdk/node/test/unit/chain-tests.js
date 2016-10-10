/**
 * Copyright 2016 IBM
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

var hfc = require('hfc');
var test = require('tape');
var util = require('util');
var tutil = require('test/unit/test-util.js');
var fs = require('fs');

// Constant to test for URLs
var urlpattern = new RegExp( "(http|ftp|grpc)s*:\/\/?");

//
//  Create a test chain
//
var chain = tutil.getTestChain("testChain");

//
// Configure the test chain
//
if (tutil.tlsOn) {
    if (tutil.caCert) {
       var pem = fs.readFileSync(tutil.caCert);
       console.log("tutil.caCertHost:", tutil.caCertHost);
       if (tutil.caCertHost) { var eventGrpcOpts={ pem:pem, hostnameOverride: tutil.caCertHost} }
       else { var eventGrpcOpts={ pem:pem } };
    }
    console.log("Setting eventHubAddr address to grpcs://" + tutil.eventHubAddr);
    chain.eventHubConnect("grpcs://" + tutil.eventHubAddr, eventGrpcOpts);
}
else {
    console.log("Setting eventHubAddr address to grpc://" + tutil.eventHubAddr);
    chain.eventHubConnect("grpc://" + tutil.eventHubAddr);
}

process.on('exit', function (){
  chain.eventHubDisconnect();
});

//
// Configure test users
//
// Set the values required to register a user with the certificate authority.
//

test_user1 = {
    name: "WebApp_user1",
    role: 1, // Client
    affiliation: "bank_a"
};

//
// Declare variables to store the test user Member objects returned after
// registration and enrollment as they will be used across multiple tests.
//

var test_user_Member1;

//
// Declare test variables that will be used to store chaincode values used
// across multiple tests.
//
var goPath = process.env.GOPATH
var testChaincodePath = process.env.SDK_CHAINCODE_PATH
 ? process.env.SDK_CHAINCODE_PATH
 : "github.com/chaincode_example02/"
var absoluteTestChaincodePath = goPath + "/src/" + testChaincodePath;

// Chaincode hash that will be filled in by the deployment operation or
// chaincode name that will be referenced in development mode.
var testChaincodeName = "mycc1";

// testChaincodeID will store the chaincode ID value after deployment.
var testChaincodeID;

// var deployCert = '/var/hyperledger/production/.membersrvc/tlsca.cert'
if (tutil.tlsOn) {
   var deployCert = tutil.caCert
   if (tutil.peerAddr0.match(tutil.hsbnDns)) deployCert = tutil.hsbnCertPath
   else if (tutil.peerAddr0.match(tutil.bluemixDns)) deployCert = tutil.bluemixCertPath
   fs.createReadStream(tutil.caCert).pipe(fs.createWriteStream(absoluteTestChaincodePath + '/certificate.pem'));
}

// Initializing values for chaincode parameters
var initA = "100";
var initB = "200";
var deltaAB = "1";

function getUser(name, cb) {
    chain.getUser(name, function (err, user) {
        if (err) return cb(err);
        if (user.isEnrolled()) return cb(null, user);
        // User is not enrolled yet, so perform both registration and enrollment
        // The chain registrar is already set inside 'Set chain registrar' test
        var registrationRequest = {
            enrollmentID: name,
            affiliation: "bank_a"
        };
        user.registerAndEnroll(registrationRequest, function (err) {
            if (err) cb(err, null)
            cb(null, user)
        });
    });
}

function pass(t, msg) {
    t.pass("Success: [" + msg + "]");
    t.end();
}

function fail(t, msg, err) {
    t.fail("Failure: [" + msg + "]: [" + err + "]");
    t.end(err);
}

//
//  Test boolean function
function devMode(c) {
   var initialDevMode = c.isDevMode();


   c.setDevMode(c.isDevMode() ? false : true);  /* toggle boolean */

   if (c.isDevMode == initialDevMode) {
      console.log("setDevMode did not change devMode");
      return "unexpected result from setDevMode() and isDevMode()";
   }
   c.setDevMode(c.isDevMode() ? false : true);  /* toggle boolean */

   if (c.isDevMode() != initialDevMode)  {
      console.log("oops DevMode: c.isDevMode() =", c.isDevMode())
      return "unexpected result from setDevMode() and isDevMode()";
   }
   return "";
} /* DevMode() */

//
// Test chain.deployWaitTime() and chain.setDeployWaitTime()
function deployWaitChallenge(cb) {
   var initDeployWaitTime = chain.getDeployWaitTime();
   var expectDeployWaitTime = 2*initDeployWaitTime;

   console.log("initDeployWaitTime", initDeployWaitTime);

   // I wrote an issue for these values which shouldn't work.  When issue is resolved, I'll
   // enhance this test
   chain.setDeployWaitTime(0);
   console.log("deployWaitTime", chain.getDeployWaitTime());
   chain.setDeployWaitTime(6000);
   console.log("deployWaitTime", chain.getDeployWaitTime());
   chain.setDeployWaitTime(-1);
   console.log("deployWaitTime", chain.getDeployWaitTime());
   chain.setDeployWaitTime("as long as it takes");
   console.log("deployWaitTime", chain.getDeployWaitTime());

   // conventional testing
   chain.setDeployWaitTime(initDeployWaitTime*2);
   var x = chain.getDeployWaitTime();
   if (x != expectDeployWaitTime) {
      console.log("deployWaitChallenge: expected " + expectDeployWaitTime + " got " + x);
      return cb(new Error("deployWaitChallenge: unexpected value from getDeployWaitTime()"));
   }
   return cb();
}; /* deployWaitChallenge() */

//
// Test chain.setInvokeWaitTime() and chain.getInvokeWaitTime()
//
function invokeWaitChallenge(cb) {
   var initInvokeWaitTime = chain.getInvokeWaitTime();
   var expectInvokeWaitTime = 2*initInvokeWaitTime;

   //console.log("initInvokeWaitTime", initInvokeWaitTime);

   chain.setInvokeWaitTime(0);
   chain.setInvokeWaitTime(6000);
   chain.setInvokeWaitTime(-1);
   chain.setInvokeWaitTime("as long as it takes");
   chain.setInvokeWaitTime(expectInvokeWaitTime);
   var x = chain.getInvokeWaitTime();
   if (x != expectInvokeWaitTime) {
      console.log("InvokeWaitChallenge: expected " + expectInvokeWaitTime + " got " + x);
      return cb(new Error("InvokeWaitChallenge: unexpected value from getInvokeWaitTime()"));
   }
   return cb();
}; /* invokeWaitChallenge() */

//
// Test adding an invalid peer (omit protocol or use invalid protocol in URL)
//
test('Add invalid peer URLs to the chain', function (t) {

    //list of valid protocol prefixes to test
    var prefixes = [
        "",
        "grpcio://",
        "http://",
        " "
    ];

    t.plan(prefixes.length);

    var chain_test;

    //loop through the prefixes
    prefixes.forEach(function (prefix, index) {

        chain_test = hfc.newChain("chain_test" + index);

        try {
            chain_test.addPeer(prefix + "://localhost:7051", new Buffer(32));
            t.fail("Should not be able to add peer with URL starting with " + prefix);
        }
        catch (err) {
            if (err.name === 'InvalidProtocol') {
                t.pass("Returned 'InvalidProtocol' error for URL starting with " + prefix);
            }
            else {
                t.fail("Failed to return 'InvalidProtocol' error  for URL starting with " + prefix);
            }
        }
    })

});


//
// Test adding a valid peer (URL must start with grpc or grpcs)
//
test('Add valid peer URLs to the chain', function (t) {

    //list of valid protocol prefixes to test
    var prefixes = [
        "grpc",
        "GRPC",
        "grpcs",
        "GRPCS"
    ];

    t.plan(prefixes.length);

    var chain_test2;

    //loop through the prefixes
    prefixes.forEach(function (prefix, index) {

        chain_test2 = hfc.newChain("chain_test2" + index);

        try {
            chain_test2.addPeer(prefix + "://localhost:7051",
                fs.readFileSync(__dirname + "/../fixtures/tlsca.cert", {encoding: "ascii"}));
            t.pass("Successfully added peer with URL starting with " + prefix + "://");
        }
        catch (err) {
            t.fail("Could not add peer to the chain with URL starting with " + prefix + "://");
        }
    })

});

//
// Test adding multiple peers to the chain
//
test('Add multiple peers to the chain', function (t) {

    t.plan(1);

    var chain_multiple = hfc.newChain("chain_multiple");

    var peers = [
        "grpc://localhost:7051",
        "grpc://localhost:7052",
        "grpc://localhost:7053",
        "grpc://localhost:7054"
    ];

    peers.forEach(function (peer) {

        try {
            chain_multiple.addPeer(peer);
        }
        catch (err) {
            t.fail("Failed to add multiple peers to the chain");
        }

    })

    //check to see we have the correct number of peers
    if (chain_multiple.getPeers().length == peers.length) {
        t.pass("Successfully added multiple peers to the chain(" + peers.length +
            " expected | " + chain_multiple.getPeers().length + " found)");

    }
    else {
        t.fail("Failed to add multiple peers to the chain(" + peers.length +
            " expected | " + chain_multiple.getPeers().length + " found)");
    }
});

//
// Test adding duplicate peers to the chain
//
test('Catch duplicate peer added to chain', function (t) {

    t.plan(2);

    var chain_duplicate = hfc.newChain("chain_duplicate");

    var peers = [
        "grpc://localhost:7051",
        "grpc://localhost:7052",
        "grpc://localhost:7053",
        "grpc://localhost:7051"
    ];

    //we have one duplicate to set the expected value
    var expected = peers.length - 1;

    peers.forEach(function (peer) {

        try {
            chain_duplicate.addPeer(peer);
        }
        catch (err) {
            if (err.name != "DuplicatePeer"){
                t.fail("Unexpected error " + err.toString());
            }
            else {
                t.pass("Expected error message 'DuplicatePeer' thrown");
            }
        }

    })

    //check to see we have the correct number of peers
    if (chain_duplicate.getPeers().length == expected) {
        t.pass("Duplicate peer not added to the chain(" + expected +
            " expected | " + chain_duplicate.getPeers().length + " found)");

    }
    else {
        t.fail("Failed to detect duplicate peer (" + expected +
            " expected | " + chain_duplicate.getPeers().length + " found)");
    }
});

//
// Set Invalid security level and hash algorithm.
//

test('Set Invalid security level and hash algorithm.', function (t) {
    t.plan(2);

    var securityLevel = chain.getMemberServices().getSecurityLevel();
    try {
        chain.getMemberServices().setSecurityLevel(128);
        t.fail("Setting an invalid security level should fail. Allowed security levels are '256' and '384'.")
    } catch (err) {
        if (securityLevel != chain.getMemberServices().getSecurityLevel()) {
            t.fail("Chain is using an invalid security level.")
        }

        t.pass("Setting an invalid security level failed as expected.")
    }

    var hashAlgorithm = chain.getMemberServices().getHashAlgorithm();
    try {
        chain.getMemberServices().setHashAlgorithm('SHA');
        t.fail("Setting an invalid hash algorithm should fail. Allowed hash algorithm are 'SHA2' and 'SHA3'.")
    } catch (err) {
        if (hashAlgorithm != chain.getMemberServices().getHashAlgorithm()) {
            t.fail("Chain is using an invalid hash algorithm.")
        }

        t.pass("Setting an invalid hash algorithm failed as expected.")
    }

});  /* Set Invalid security level and hash algorithm */


//
// Enroll the WebAppAdmin member. WebAppAdmin member is already registered
// manually by being included inside the membersrvc.yaml file.
//

test('Enroll WebAppAdmin', function (t) {
    t.plan(3);

    // Get the WebAppAdmin member
    chain.getMember("WebAppAdmin", function (err, WebAppAdmin) {
        if (err) {
            t.fail("Failed to get WebAppAdmin member " + " ---> " + err);
            t.end(err);
        } else {
            t.pass("Successfully got WebAppAdmin member" /*+ " ---> " + JSON.stringify(crypto)*/);

            // Enroll the WebAppAdmin member with the certificate authority using
            // the one time password hard coded inside the membersrvc.yaml.
            pw = "DJY27pEnl16d";
            WebAppAdmin.enroll(pw, function (err, crypto) {
                if (err) {
                    t.fail("Failed to enroll WebAppAdmin member " + " ---> " + err);
                    t.end(err);
                } else {
                    t.pass("Successfully enrolled WebAppAdmin member" /*+ " ---> " + JSON.stringify(crypto)*/);

                    // Confirm that the WebAppAdmin token has been created in the key value store
                    path = chain.getKeyValStore().dir + "/member." + WebAppAdmin.getName();

                    fs.exists(path, function (exists) {
                        if (exists) {
                            t.pass("Successfully stored client token" /*+ " ---> " + WebAppAdmin.getName()*/);
                        } else {
                            t.fail("Failed to store client token for " + WebAppAdmin.getName() + " ---> " + err);
                        }
                    });
                }
            });
        }
    });
});  /* Enroll WebAppAdmin */

//
// Set the WebAppAdmin as the designated chain 'registrar' member who will
// subsequently register/enroll other new members. WebAppAdmin member is already
// registered manually by being included inside the membersrvc.yaml file and
// enrolled in the UT above.
//

test('Set chain registrar', function (t) {
    t.plan(2);

    // Get the WebAppAdmin member
    chain.getMember("WebAppAdmin", function (err, WebAppAdmin) {
        if (err) {
            t.fail("Failed to get WebAppAdmin member " + " ---> " + err);
            t.end(err);
        } else {
            t.pass("Successfully got WebAppAdmin member");

            // Set the WebAppAdmin as the designated chain registrar
            chain.setRegistrar(WebAppAdmin);

            // Confirm that the chain registrar is now WebAppAdmin
            t.equal(chain.getRegistrar().getName(), "WebAppAdmin", "Successfully set chain registrar");
        }
    });
}); /* Set Chain Registrar */

//
// Register and enroll a new user with the certificate authority.
// This will be performed by the registrar member, WebAppAdmin.
//

test('Register and enroll a new user', function (t) {
    t.plan(2);

    // Register and enroll test_user
    getUser(test_user1.name, function (err, user) {
        if (err) {
            fail(t, "Failed to get " + test_user1.name + " ---> ", err);
        } else {
            test_user_Member1 = user;

            t.pass("Successfully registered and enrolled " + test_user_Member1.getName());

            // Confirm that the user token has been created in the key value store
            path = chain.getKeyValStore().dir + "/member." + test_user1.name;
            fs.exists(path, function (exists) {
                if (exists) {
                    t.pass("Successfully stored client token" /*+ " ---> " + test_user1.name*/);
                    t.end();
                } else {
                    t.fail("Failed to store client token for " + test_user1.name + " ---> " + err);
                    t.end(err);
                }
            });
        }
    });
}); /* Register and enroll a new user */

//
// Create and issue a chaincode deploy request with a missing chaincodeName
// parameter (in development mode) and a missing chaincodePath parameter (in
// network mode). The request is expected to fail with an error specifying
// the missing parameter.
//

test('Deploy with missing chaincodeName or chaincodePath', function (t) {
    t.plan(1);

    // Construct the deploy request with a missing chaincodeName/chaincodePath
    var deployRequest = {
        // Function to trigger
        fcn: "init",
        // Arguments to the initializing function
        args: ["a", initA, "b", initB],
        certificatePath: deployCert
    };

    // Trigger the deploy transaction
    var deployTx = test_user_Member1.deploy(deployRequest);

    // Print the deploy results
    deployTx.on('complete', function (results) {
        // Deploy request completed successfully
        console.log(util.format("deploy results: %j", results));
        // Set the testChaincodeID for subsequent tests
        testChaincodeID = results.chaincodeID;
        console.log("testChaincodeID:" + testChaincodeID);
        t.fail(util.format("Successfully deployed chaincode: request=%j, response=%j", deployRequest, results));
    });
    deployTx.on('error', function (err) {
        // Deploy request failed
        t.pass(util.format("Failed to deploy chaincode: request=%j, error=%j", deployRequest, err));
    });
}); /* Deploy with missing chaincodeName or chaincodePath */

//
// Create and issue a chaincode deploy request by the test user, who was
// registered and enrolled in the UT above. Deploy a testing chaincode from
// a local directory in the user's $GOPATH.
//

test('Deploy a chaincode by enrolled user', function (t) {
    t.plan(1);

    // Construct the deploy request
    var deployRequest = {
        // Function to trigger
        fcn: "init",
        // Arguments to the initializing function
        args: ["a", initA, "b", initB],
        certificatePath: deployCert
    };

  //chain.setDeployWaitTime(-1);
  //console.log("setting bogus wait time prior to deploy")
 if (tutil.deployMode === 'dev') {
        // Name required for deploy in development mode
        deployRequest.chaincodeName = testChaincodeName;
    } else {
        // Path (under $GOPATH) required for deploy in network mode
        deployRequest.chaincodePath = testChaincodePath;
    }

    // Trigger the deploy transaction
    var deployTx = test_user_Member1.deploy(deployRequest);

    // Print the deploy results
    deployTx.on('complete', function (results) {
        // Deploy request completed successfully
        console.log(util.format("deploy results: %j", results));
        // Set the testChaincodeID for subsequent tests
        testChaincodeID = results.chaincodeID;
        console.log("testChaincodeID:" + testChaincodeID);
        t.pass(util.format("Successfully deployed chaincode: request=%j, response=%j", deployRequest, results));
    });
    deployTx.on('error', function (err) {
        // Deploy request failed
        t.fail(util.format("Failed to deploy chaincode: request=%j, error=%j", deployRequest, err));
    });
}); /* Deploy a chaincode by enrolled user */

//
// Create and issue a chaincode query request with a missing chaincodeID
// parameter. The request is expected to fail with an error specifying
// the missing parameter.
//

test('Query with missing chaincodeID', function (t) {
    t.plan(1);

    // Construct the query request with a missing chaincodeID
    var queryRequest = {
        // Function to trigger
        fcn: "query",
        // Existing state variable to retrieve
        args: ["a"]
    };

    // Trigger the query transaction
    test_user_Member1.setTCertBatchSize(1);
    var queryTx = test_user_Member1.query(queryRequest);

    // Print the query results
    queryTx.on('complete', function (results) {
        // Query completed successfully
        t.fail(util.format("Successfully queried existing chaincode state: request=%j, response=%j, value=%s", queryRequest, results, results.result.toString()));
    });
    queryTx.on('error', function (err) {
        // Query failed
        t.pass(util.format("Failed to query existing chaincode state: request=%j, error=%j", queryRequest, err));
    });
});  /* Query with missing chaincodeID */

//
// Create and issue a chaincode query request by the test user, who was
// registered and enrolled in the UT above. Query an existing chaincode
// state variable with a transaction certificate batch size of 1.
//

test('Query existing chaincode state by enrolled user with batch size of 1', function (t) {
    t.plan(1);

    // Construct the query request
    var queryRequest = {
        // Name (hash) required for query
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "query",
        // Existing state variable to retrieve
        args: ["a"]
    };

    // Trigger the query transaction
    test_user_Member1.setTCertBatchSize(1);
    var queryTx = test_user_Member1.query(queryRequest);

    // Print the query results
    queryTx.on('complete', function (results) {
        // Query completed successfully
        t.pass(util.format("Successfully queried existing chaincode state: request=%j, response=%j, value=%s", queryRequest, results, results.result.toString()));
    });
    queryTx.on('error', function (err) {
        // Query failed
        t.fail(util.format("Failed to query existing chaincode state: request=%j, error=%j", queryRequest, err));
    });
}); /* Query existing chaincode state by enrolled user with batch size of 1 */

//
// Create and issue a chaincode query request by the test user, who was
// registered and enrolled in the UT above. Query an existing chaincode
// state variable with a transaction certificate batch size of 100.
//

test('Query existing chaincode state by enrolled user with batch size of 100', function (t) {
    t.plan(1);

    // Construct the query request
    var queryRequest = {
        // Name (hash) required for query
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "query",
        // Existing state variable to retrieve
        args: ["a"]
    };

    // Trigger the query transaction
    test_user_Member1.setTCertBatchSize(100);
    var queryTx = test_user_Member1.query(queryRequest);

    // Print the query results
    queryTx.on('complete', function (results) {
        // Query completed successfully
        t.pass(util.format("Successfully queried existing chaincode state: request=%j, response=%j, value=%s", queryRequest, results, results.result.toString()));
    });
    queryTx.on('error', function (err) {
        // Query failed
        t.fail(util.format("Failed to query existing chaincode state: request=%j, error=%j", queryRequest, err));
    });
}); /* Query existing chaincode state by enrolled user with batch size of 100 */

//
// Create and issue a chaincode query request by the test user, who was
// registered and enrolled in the UT above. Query a non-existing chaincode
// state variable.
//

test('Query non-existing chaincode state by enrolled user', function (t) {
    t.plan(1);

    // Construct the query request
    var queryRequest = {
        // Name (hash) required for query
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "query",
        // Existing state variable to retrieve
        args: ["BOGUS"]
    };

    // Trigger the query transaction
    var queryTx = test_user_Member1.query(queryRequest);

    // Print the query results
    queryTx.on('complete', function (results) {
        // Query completed successfully
        t.fail(util.format("Successfully queried non-existing chaincode state: request=%j, response=%j, value=%s", queryRequest, results, results.result.toString()));
    });
    queryTx.on('error', function (err) {
        // Query failed
        t.pass(util.format("Failed to query non-existing chaincode state: request=%j, error=%j", queryRequest, err));
    });
}); /* Query non-existing chaincode state by enrolled user */

//
// Create and issue a chaincode query request by the test user, who was
// registered and enrolled in the UT above. Query a non-existing chaincode
// function.
//

test('Query non-existing chaincode function by enrolled user', function (t) {
    t.plan(1);

    // Construct the query request
    var queryRequest = {
        // Name (hash) required for query
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "BOGUS",
        // Existing state variable to retrieve
        args: ["a"]
    };

    // Trigger the query transaction
    var queryTx = test_user_Member1.query(queryRequest);

    // Print the query results
    queryTx.on('complete', function (results) {
        // Query completed successfully
        t.fail(util.format("Successfully queried non-existing chaincode function: request=%j, response=%j, value=%s", queryRequest, results, results.result.toString()));
    });
    queryTx.on('error', function (err) {
        // Query failed
        t.pass(util.format("Failed to query non-existing chaincode function: request=%j, error=%j", queryRequest, err));
    });
}); /* Query non-existing chaincode function by enrolled user */

//
// Create and issue a chaincode invoke request with a missing chaincodeID
// parameter. The request is expected to fail with an error specifying
// the missing parameter.
//

test('Invoke with missing chaincodeID', function (t) {
    t.plan(1);

    // Construct the invoke request with missing chaincodeID
    var invokeRequest = {
        // Function to trigger
        fcn: "invoke",
        // Parameters for the invoke function
        args: ["a", "b", deltaAB]
    };

    // Trigger the invoke transaction
    var invokeTx = test_user_Member1.invoke(invokeRequest);

    // Print the invoke results
    invokeTx.on('submitted', function (results) {
        // Invoke transaction submitted successfully
        t.fail(util.format("Successfully submitted chaincode invoke transaction: request=%j, response=%j", invokeRequest, results));
    });
    invokeTx.on('error', function (err) {
        // Invoke transaction submission failed
        t.pass(util.format("Failed to submit chaincode invoke transaction: request=%j, error=%j", invokeRequest, err));
    });
}); /* Invoke with missing chaincodeID */

//
// Create and issue a chaincode invoke request by the test user, who was
// registered and enrolled in the UT above.
//

test('Invoke a chaincode by enrolled user', function (t) {
    t.plan(1);

    // Construct the invoke request
    var invokeRequest = {
        // Name (hash) required for invoke
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "invoke",
        // Parameters for the invoke function
        args: ["a", "b", deltaAB]
    };

    // Trigger the invoke transaction
    var invokeTx = test_user_Member1.invoke(invokeRequest);

    // Print the invoke results
    invokeTx.on('submitted', function (results) {
        // Invoke transaction submitted successfully
        t.pass(util.format("Successfully submitted chaincode invoke transaction: request=%j, response=%j", invokeRequest, results));
        chain.eventHubDisconnect();
    });
    invokeTx.on('error', function (err) {
        // Invoke transaction submission failed
        t.fail(util.format("Failed to submit chaincode invoke transaction: request=%j, error=%j", invokeRequest, err));
    });
}); /* Invoke a chaincode by enrolled user */


//
// Test getting and setting member services
// registered and enrolled in the UT above.
//
test.skip('Get/set Member services', function (t) {
   //
   // Leave t.plan(1) for now. Deleting t.plan(1) causes all subsequent test cases to fail.
   //
   t.plan(1); // Don't even think of deleting this
   var initMemberServices = chain.getMemberServices();
   //console.log("Here is a MemberServices: ", chain.getMemberServices());

   // Make a modifiable copy of incoming member services
   var myMemberServices = { ecaaClient: initMemberServices.ecaaClient
                           ,ecapClient: initMemberServices.ecapClient
                           ,tcapClient: initMemberServices.tcapClient
                           ,tlscapClient: initMemberServices.tlscapClient
                           ,cryptoPrimitives: initMemberServices.cryptoPrimitives
                           };


   //console.log("\nCurrent MemberServices copied into myMemberServices", myMemberServices);

   // Modify my MemberServices so I can test chain.setMemberServices()
   // SecurityLevel and HashAlgorithm have only two valid values, so set each to its other value
   myMemberServices.cryptoPrimitives.securityLevel = initMemberServices.getSecurityLevel() == 256 ? 384 : 256;
   myMemberServices.cryptoPrimitives.hashAlgorithm = initMemberServices.getHashAlgorithm('SHA2') == 'SHA2' ? 'SHA3' : 'SHA2';

   console.log("\nMyMemberServices with new security level and hash algorithm: ", myMemberServices);
   chain.setMemberServices(myMemberServices);

   console.log("\nnew member services after chain.setMemberServices(myMemberServices):", chain.getMemberServices());
   t.fail(util.format("chain.getMemberServices failed waiting for FAB-249"));
   console.log("chain.getMemberServices().getSecurityLevel ", chain.getMemberServices().getSecurityLevel());
   console.log("\nchain.getMemberServices.getHashAlgorithm ", chain.getMemberServices().getHashAlgorithm());

   //var level = chain.getMemberServices().getSecurityLevel();
   var level = chain.getMemberServices().getSecurityLevel();
   if (c.securityLevel !== level) {
      console.log("get/setMemberServices Unexpected result from getSecurityLevel ", level);
      t.fail(util.format("chain.getMemberServices failed"));
   }
   var hash = chain.getMemberServices().getHashAlgorithm();
   if (c.hashAlgorithm !== hash) {
      console.log("get/setMemberServices Unexpected result from ", hash);
      t.fail(util.format("chain.getMemberServices failed"));
   }
   chain.setMemberServices(initMemberServices);
   t.pass(util.format("get/set MemberServices OK"));

}); /* Get/set Member services */

//
// Test setting and getting deployWaitTime
//
test('Deploy wait time', function(t) {
   deployWaitChallenge(function(err) {
      if (err) {
         fail(t, "deployWaitChallenge", err);
      }
      else {
         pass(t, "deployWaitChallenge");
      }
   });
});

//
// Test setting and getting invokeWaitTime
//
test('Invoke wait time', function(t) {
   invokeWaitChallenge(function(err) {
      if (err) {
         fail(t, "invokeWaitChallenge", err);
      }
      else {
         pass(t, "invokeWaitChallenge");
      }
   });
});


//
// Test chain.getPeers() and chain.addPeer()
//
test.skip('peer pem', function(t) {
   console.log("peer pem is blocked by issue FAB-433")
   var initialPeerCount = chain.getPeers().length;
   var realPem = "-----BEGIN CERTIFICATE-----" +
      "MIICSjCCAbOgAwIBAgIJAJHGGR4dGioHMA0GCSqGSIb3DQEBCwUAMFYxCzAJBgNV"+
      "BAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBX"+
      "aWRnaXRzIFB0eSBMdGQxDzANBgNVBAMTBnRlc3RjYTAeFw0xNDExMTEyMjMxMjla"+
      "Fw0yNDExMDgyMjMxMjlaMFYxCzAJBgNVBAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0"+
      "YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxDzANBgNVBAMT"+
      "BnRlc3RjYTCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAwEDfBV5MYdlHVHJ7"+
      "+L4nxrZy7mBfAVXpOc5vMYztssUI7mL2/iYujiIXM+weZYNTEpLdjyJdu7R5gGUu"+
      "Qah2PH5ACLrIIC6tRka9hcaBlIECAwEAAaMgMB4wDAYDVR0TBAUwAwEB/zAOBgNV"+
      "HQ8BAf8EBAMCAgQwDQYJKoZIhvcNAQELBQADgYEAHzC7jdYlzAVmddi/gdAeKPau"+
      "sPBG/C2HCWqHzpCUHcKuvMzDVkY/MP2o6JIW2DBbY64bO/FceExhjcykgaYtCH/m"+
      "oIU63+CFOTtR7otyQAWHqXa7q4SbCDlG7DyRFxqG0txPtGvy12lgldA2+RgcigQG"+
      "Dfcog5wrJytaQ6UA0wE=" +
      "-----END CERTIFICATE-----";

   //console.log("real pem = ", realPem);
   // Specify invalid pem parameters
   chain.addPeer("grpc://localhost:20202", realPem);
   if (chain.addPeer("grpc://localhost:30303", realPem)) {
      //console.log("real PEM accepted");
      if (chain.addPeer("grpc://localhost:303030", "")) {
         // This parameter is optional, so probably empty string should be allowed
         if (chain.addPeer("grpc://localhost:303031", -12)) {
            // not a string, so why was it accepted?
            fail(t, "peer pem: pem=-12 unexpectedly accepted");
         }
         else {
            pass(t, "peer pem");
         }
      }
      else {
         fail(t, "peer pem: null pem was rejected");
      }
   }
   else {
      fail(t, "peer pem: text string rejected");
   }

}); /*  AddPeer pem */

//
// Test chain.getTCertBatchSize()
test('Get tcert batch size', function(t) {

   var size = chain.getTCertBatchSize();
   chain.setTCertBatchSize(0);

   if (!isNaN(size) && (size)) {
      chain.setTCertBatchSize(size + 1);
      var newSize = chain.getTCertBatchSize();
      if (newSize == (size + 1)) {
         pass(t, "get tcert batch size");
      }
      else {
         fail(t, "chain.getTCertBatchSize() returned " + newSize + " expected " + size);
      }
   }
   else {
      fail(t, "chain.getTCertBatchSize() unexpectedly returned " + size);
   }
   chain.setTCertBatchSize(size);      // restore initial value
}); /* Get tcert batch size */

//
// Test chain.isPreFetchMode()
test('Get and test prefetch mode', function(t) {
   var initialPreFetchMode = chain.isPreFetchMode();

   chain.setPreFetchMode(chain.isPreFetchMode() ? false : true);  /* toggle value */
   if (chain.isPreFetchMode == initialPreFetchMode) {
      fail(t, "get and test prefetch mode", "setPreFetchMode() did not change preFetchMode()");
   }

   chain.setPreFetchMode(chain.isPreFetchMode() ? false : true);  /* toggle value */
   if (chain.isPreFetchMode() != initialPreFetchMode)  {
      fail(t, "get and test prefetch mode",
      "unexpected result from setPreFetchMode() and isPreFetchMode()");
   }

  pass(t, "get and test prefetch mode")
}); /* Get and test prefetch mode */

//
// Test chain.isDevMode()
test('Get and test Dev mode', function(t) {
   //err = DevMode(chain);
   //console.log("return from DevModeTEst", err)
   if (err = devMode(chain)) {
      console.log("DevMode returned ", err);
      fail(t, "get and test Dev mode: ", err);
   }
   else {
      pass(t, "get and set Dev mode");
   }
}); /* Get and test Dev mode */

//
// Test chain.isSecurityEnabled()
test('is Security enabled', function(t) {
   var incoming = chain.isSecurityEnabled();
   var expected = true;

   if (incoming != expected) {
      console.log("chain isSecurityEnabled=" + incoming + " Expecting "+ expected);
      fail(t, "is security enabled: unexpected result from chain.isSecurityEnabled()");
   }
   pass(t, "is security enabled");
});

//
//  Test peer class methods
test('Peer class methods', function(t) {
   var peers = chain.getPeers();

   if (!peers) {
      fail(t, "Peer class methods: chain has no peers");
   }

   for (var i = 0; peers[i]; i++) { // For each peer
      //
      // Verify peer.chain points to original chain
      var ch2 = peers[i].getChain();
      if (ch2 != chain) {
         console.log("peers[%d]", i, peers[i]);
         fail(t, "Peer class methods: Peer's chain does not point back to original chain");
      }

      var url = peers[i].getUrl();
      // console.log("peer url", url);
      // This pattern test isn't bullet proof but good enough to test whether the URL of an active peer
      // seems to be an actual URL.
      if (!urlpattern.test(url)) {
         console.log(url + " is not a valid URL");
         fail(t, "Peer class: peer.getUrl() returned invalid url: " + url);
      }
   } /* for each peer */

   pass(t, "Peer class methods");

}); /* Peer class methods */

//
// Test chain.removePeer().
// This must be the last test.
test.skip('Remove peer', function(t) {
   //  BLOCKED BY DEFECT fab-250.
   // This test has executed only on a single peer network on my laptop, but is designed to work when
   // multiple peers are in the chain.
   //
   console.log("Remove peer: rewrite this test when FAB-250 fix is available");
   fail(t, "Remove peer: blocked by issue FAB-250");

   return;
   var peers = chain.getPeers();
   if (!peers) {
      fail(t, "Remove peer: chain has no peers");
   }
   try {
      for (var i = 0; peers[i]; i++) { // Remove all the peers
         //console.log("Removing this peer: ", peers[i]);
         peers[i].remove();
      }
   } catch (err) {
      t.fail("Remove peer: exception removing peer from chain");
   }

   // Verify no peers remain
   peers = chain.getPeers();
   if (peers) {
      fail(t, "Remove peer: chain has peers after peer.remove()");
   }

   pass(t, 'Remove peer');
}); /* Remove peer */
