/**
 * Copyright London Stock Exchange 2016 All Rights Reserved.
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

var hfc = require('../..');
var test = require('tape');
var util = require('util');
var fs = require('fs');
var tutil = require('test/unit/test-util.js');

//
//  Create a test chain
//

var chain = tutil.getTestChain("testChain");
chain.setDeployWaitTime(120);

var goPath = process.env.GOPATH;
// Path to the local directory containing the chaincode project under $GOPATH
var testChaincodePath = "github.com/eventsender/";
var absoluteTestChaincodePath = goPath + "/src/" + testChaincodePath;


//
// Configure the test chain
//
if (tutil.tlsOn) {
    var deployCert = tutil.caCert;
    fs.createReadStream(tutil.caCert).pipe(fs.createWriteStream(absoluteTestChaincodePath + '/certificate.pem'));
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

// Chaincode hash that will be filled in by the deployment operation or
// chaincode name that will be referenced in development mode.
var testChaincodeName = "mycc5";

// testChaincodeID will store the chaincode ID value after deployment.
var testChaincodeID;

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

//
// Enroll the WebAppAdmin member and set as registrar then register
// and enroll new user with certificate authority
// This test is a prerequisite to further tests
//

test('Set up chain prerequisites', function (t) {
    t.plan(5);

    // Get the WebAppAdmin member
    chain.getMember("WebAppAdmin", function (err, WebAppAdmin) {
        if (err) {
            t.fail("Failed to get WebAppAdmin member " + " ---> " + err);
            t.end(err);
            process.exit(1);
        } else {
            t.pass("Successfully got WebAppAdmin member" /*+ " ---> " + JSON.stringify(crypto)*/);

            // Enroll the WebAppAdmin member with the certificate authority using
            // the one time password hard coded inside the membersrvc.yaml.
            pw = "DJY27pEnl16d";
            WebAppAdmin.enroll(pw, function (err, crypto) {
                if (err) {
                    t.fail("Failed to enroll WebAppAdmin member " + " ---> " + err);
                    t.end(err);
                    process.exit(1);
                } else {
                    t.pass("Successfully enrolled WebAppAdmin member" /*+ " ---> " + JSON.stringify(crypto)*/);

                    // Confirm that the WebAppAdmin token has been created in the key value store
                    path = chain.getKeyValStore().dir + "/member." + WebAppAdmin.getName();

                    fs.exists(path, function (exists) {
                        if (exists) {
                            t.pass("Successfully stored client token" /*+ " ---> " + WebAppAdmin.getName()*/);
                        } else {
                            t.fail("Failed to store client token for " + WebAppAdmin.getName() + " ---> " + err);
                            t.end(err);
                            // Exit the test script after a failure
                            process.exit(1);
                        }
                        chain.setRegistrar(WebAppAdmin);
                        // Register and enroll test_user
                        getUser(test_user1.name, function (err, user) {
                            if (err) {
                                t.fail("Failed to get " + test_user1.name + " ---> ", err);
                                t.end(err);
                                // Exit the test script after a failure
                                process.exit(1);
                            } else {
                                test_user_Member1 = user;

                                t.pass("Successfully registered and enrolled " + test_user_Member1.getName());

                                // Confirm that the user token has been created in the key value store
                                path = chain.getKeyValStore().dir + "/member." + test_user1.name;
                                fs.exists(path, function (exists) {
                                    if (exists) {
                                        t.pass("Successfully stored client token" /*+ " ---> " + test_user1.name*/);
                                    } else {
                                        t.fail("Failed to store client token for " + test_user1.name + " ---> " + err);
                                        t.end(err);
                                        // Exit the test script after a failure
                                        process.exit(1);
                                    }
                                });
                            }
                        });
                    });
                }
            });
        }
    });
});

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
        args: [],
        certificatePath: deployCert
    };

    if (tutil.deployMode === 'dev') {
        // Name required for deploy in development mode
        deployRequest.chaincodeName = testChaincodeName;
    } else {
        // Path (under $GOPATH) required for deploy in network mode
        deployRequest.chaincodePath = testChaincodePath;
    }

    // Trigger the deploy transaction
    var deployTx = test_user_Member1.deploy(deployRequest);

    // the deploy complete is triggered as a result of a fabric deploy
    // complete event automatically when a event source is connected
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
        // Exit the test script after a failure
        process.exit(1);
    });
});

//
// Issue a chaincode invoke to generate event and listen for the event
// by registering with chaincode id and event name
//

test('Invoke chaincode and have it generate an event', function (t) {
    t.plan(2);

    var evtstring = "event-test";
    // Construct the invoke request
    var invokeRequest = {
        // Name (hash) required for invoke
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "invoke",
        // Parameters for the invoke function
        args: [evtstring]
    };
    var eh = chain.getEventHub();
    var duration = chain.getInvokeWaitTime() * 1000;
    var timedout = true;
    var timeoutId = null;

    // register for chaincode event
    var regid = eh.registerChaincodeEvent(testChaincodeID, "^evtsender$", function(event) {
        timedout = false;
        if (timeoutId) {
            clearTimeout(timeoutId);
        }
        t.equal(event.payload.toString(), "Event 0," + evtstring, "Successfully received expected chaincode event payload");
        eh.unregisterChaincodeEvent(regid);
    });
    // Trigger the invoke transaction
    var invokeTx = test_user_Member1.invoke(invokeRequest);
    // set timout on event sent by chaincode invoke
    timeoutId = setTimeout(function() {
                                 if(timedout) {
                                     eh.unregisterChaincodeEvent(regid);
                                     t.fail("Failed to receive chaincode event");
                                     process.exit(1);
                                 }
                             },
                             duration);

    // Print the invoke results
    invokeTx.on('complete', function (results) {
        // Invoke transaction submitted successfully
        t.pass(util.format("Successfully completed chaincode invoke transaction: request=%j, response=%j", invokeRequest, results));
    });
    invokeTx.on('error', function (err) {
        // Invoke transaction submission failed
        t.fail(util.format("Failed to submit chaincode invoke transaction: request=%j, error=%j", invokeRequest, err));
        // Exit the test script after a failure
        process.exit(1);
    });
});
//
// Issue a chaincode invoke to generate event and listen for the event
// on 2 registrations
//

test('Invoke chaincode, have it generate an event, and receive event on 2 registrations', function (t) {
    t.plan(3);

    var evtstring = "event-test";
    // Construct the invoke request
    var invokeRequest = {
        // Name (hash) required for invoke
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "invoke",
        // Parameters for the invoke function
        args: [evtstring]
    };
    var eh = chain.getEventHub();
    var duration = chain.getInvokeWaitTime() * 1000;
    var timedout = true;
    var timeoutId = null;
    var eventcount = 0;

    // register for chaincode event
    var regid1 = eh.registerChaincodeEvent(testChaincodeID, "^evtsender$", function(event) {
        eventcount++;
        if (eventcount > 1) {
            if (timeoutId) {
                clearTimeout(timeoutId);
            }
        }
        t.equal(event.payload.toString(), "Event 1," + evtstring, "Successfully received expected chaincode event payload");
        eh.unregisterChaincodeEvent(regid1);
    });
    // register for chaincode event
    var regid2 = eh.registerChaincodeEvent(testChaincodeID, "^evtsender$", function(event) {
        eventcount++;
        if (eventcount > 1) {
            if (timeoutId) {
                clearTimeout(timeoutId);
            }
        }
        t.equal(event.payload.toString(), "Event 1," + evtstring, "Successfully received expected chaincode event payload");
        eh.unregisterChaincodeEvent(regid2);
    });
    // Trigger the invoke transaction
    var invokeTx = test_user_Member1.invoke(invokeRequest);
    // set timout on event sent by chaincode invoke
    timeoutId = setTimeout(function() {
                                 if(eventcount > 1) {
                                     eh.unregisterChaincodeEvent(regid1);
                                     eh.unregisterChaincodeEvent(regid2);
                                     t.fail("Failed to receive chaincode event");
                                     process.exit(1);
                                 }
                             },
                             duration);

    // Print the invoke results
    invokeTx.on('complete', function (results) {
        // Invoke transaction submitted successfully
        t.pass(util.format("Successfully completed chaincode invoke transaction: request=%j, response=%j", invokeRequest, results));
    });
    invokeTx.on('error', function (err) {
        // Invoke transaction submission failed
        t.fail(util.format("Failed to submit chaincode invoke transaction: request=%j, error=%j", invokeRequest, err));
        // Exit the test script after a failure
        process.exit(1);
    });
});

//
// Issue a chaincode invoke to generate event and listen for the event
// by registering with chaincode id and wildcarded event name
//

test('Generate chaincode event and receive it with wildcard', function (t) {
    t.plan(2);

    var evtstring = "event-test";
    // Construct the invoke request
    var invokeRequest = {
        // Name (hash) required for invoke
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "invoke",
        // Parameters for the invoke function
        args: [evtstring]
    };
    var eh = chain.getEventHub();
    var duration = chain.getInvokeWaitTime() * 1000;
    var timedout = true;
    var timeoutId = null;

    // register for chaincode event with wildcard event name
    var regid = eh.registerChaincodeEvent(testChaincodeID, ".*", function(event) {
        timedout = false;
        if (timeoutId) {
            clearTimeout(timeoutId);
        }
        t.equal(event.payload.toString(), "Event 2," + evtstring, "Successfully received expected chaincode event payload");
        eh.unregisterChaincodeEvent(regid);
    });
    // Trigger the invoke transaction
    var invokeTx = test_user_Member1.invoke(invokeRequest);
    // set timout on event sent by chaincode invoke
    timeoutId = setTimeout(function() {
                                 if(timedout) {
                                     eh.unregisterChaincodeEvent(regid);
                                     t.fail("Failed to receive chaincode event");
                                     process.exit(1);
                                 }
                             },
                             duration);

    // Print the invoke results
    invokeTx.on('complete', function (results) {
        // Invoke transaction submitted successfully
        t.pass(util.format("Successfully completed chaincode invoke transaction: request=%j, response=%j", invokeRequest, results));
    });
    invokeTx.on('error', function (err) {
        // Invoke transaction submission failed
        t.fail(util.format("Failed to submit chaincode invoke transaction: request=%j, error=%j", invokeRequest, err));
        // Exit the test script after a failure
        process.exit(1);
    });
});

//
// Issue a chaincode invoke to generate event and listen for the event
// by registering with chaincode id and a bogus event name
//

test('Generate an event that fails to be received', function (t) {
    t.plan(2);

    var evtstring = "event-test";
    // Construct the invoke request
    var invokeRequest = {
        // Name (hash) required for invoke
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "invoke",
        // Parameters for the invoke function
        args: [evtstring]
    };
    var eh = chain.getEventHub();
    var duration = chain.getInvokeWaitTime() * 1000;
    var timedout = true;
    var timeoutId = null;

    // register for chaincode event with bogus event name
    var regid = eh.registerChaincodeEvent(testChaincodeID, "bogus", function(event) {
        timedout = false;
        if (timeoutId) {
            clearTimeout(timeoutId);
        }
        t.fail("Received chaincode event from bogus registration");
        eh.unregisterChaincodeEvent(regid);
        process.exit(1);
    });
    // Trigger the invoke transaction
    var invokeTx = test_user_Member1.invoke(invokeRequest);
    // set timout on event sent by chaincode invoke
    timeoutId = setTimeout(function() {
                                 if(timedout) {
                                     eh.unregisterChaincodeEvent(regid);
                                     t.pass("Failed to receive chaincode event");
                                 }
                             },
                             duration);

    // Print the invoke results
    invokeTx.on('complete', function (results) {
        // Invoke transaction submitted successfully
        t.pass(util.format("Successfully completed chaincode invoke transaction: request=%j, response=%j", invokeRequest, results));
    });
    invokeTx.on('error', function (err) {
        // Invoke transaction submission failed
        t.fail(util.format("Failed to submit chaincode invoke transaction: request=%j, error=%j", invokeRequest, err));
        // Exit the test script after a failure
        process.exit(1);
    });
});

//
//
// Create and issue a chaincode query request by the test user, who was
// registered and enrolled in the UT above. Query a chaincode for
// number of events generated.
//

test('Query chaincode state for number of events sent', function (t) {
    t.plan(1);

    // Construct the query request
    var queryRequest = {
        // Name (hash) required for query
        chaincodeID: testChaincodeID,
        // Function to trigger
        fcn: "query",
        // Existing state variable to retrieve
        args: [""]
    };

    // Trigger the query transaction
    var queryTx = test_user_Member1.query(queryRequest);

    // Print the query results
    queryTx.on('complete', function (results) {
        var result = JSON.parse(results.result.toString());
        var count = parseInt(result.NoEvents);
        t.equal(count, 4, "Successfully queried correct number of events generated.");
        chain.eventHubDisconnect();
    });
    queryTx.on('error', function (err) {
        // Query failed
        t.fail(util.format("Failed to query chaincode state: request=%j, error=%j", queryRequest, err));
        t.end(err);
    });
});

