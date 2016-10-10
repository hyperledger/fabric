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
/**
 * Licensed Materials - Property of IBM
 * © Copyright IBM Corp. 2016
 */

/**
 * Simple asset management use case where authentication is performed
 * with the help of TCerts only (use-case 1) or attributes only (use-case 2).*/

var hfc = require('hfc');
var test = require('tape');
var util = require('util');
var crypto = require('lib/crypto');
var tutil = require('test/unit/test-util.js');
var fs = require('fs');

var chain, chaincodeID;
var chaincodeName = "mycc4";
var deployer, alice, bob, assigner;
var aliceAccount = "12345-56789";
var bobAccount = "23456-67890";

var goPath = process.env.GOPATH
var testChaincodePath = process.env.SDK_CHAINCODE_PATH
 ? process.env.SDK_CHAINCODE_PATH
 : "github.com/asset_management_with_roles/"
var absoluteTestChaincodePath = goPath + "/src/" + testChaincodePath;

function pass(t, msg) {
    t.pass("Success: [" + msg + "]");
    t.end();
}

function fail(t, msg, err) {
    t.fail("Failure: [" + msg + "]: [" + err + "]");
    t.end(err);
}

// Create the chain and enroll users as deployer, assigner, and nonAssigner (who doesn't have privilege to assign.
function setup(cb) {
   console.log("initializing ...");
   chain = tutil.getTestChain("testChain");
   console.log("enrolling deployer ...");
   chain.enroll("WebAppAdmin", "DJY27pEnl16d", function (err, user) {
      if (err) return cb(err);
      deployer = user;
      chain.setRegistrar(user);
      console.log("enrolling assigner2 ...");
      registerAndEnroll("assigner2",[{name:'role',value:'assigner'}], function(err,user) {
         if (err) return cb(err);
         assigner = user;
         console.log("enrolling alice2 ...");
         registerAndEnroll("alice2",[{name:'role',value:'client'},{name:'account',value:aliceAccount}], function(err,user) {
            if (err) return cb(err);
            alice = user;
            console.log("enrolling bob2 ...");
            registerAndEnroll("bob2",[{name:'role',value:'client'},{name:'account',value:bobAccount}], function(err,user) {
               if (err) return cb(err);
               bob = user;
               return deploy(cb);
            });
         });
      });
   });
}

// Deploy assetmgmt_with_roles with the name of the assigner role in the metadata
function deploy(cb) {
    console.log("deploying with the role name 'assigner' in metadata ...");
    if (tutil.tlsOn) {
       var deployCert = tutil.caCert
       if (tutil.peerAddr0.match(tutil.hsbnDns)) deployCert = tutil.hsbnCertPath
       else if (tutil.peerAddr0.match(tutil.bluemixDns)) deployCert = tutil.bluemixCertPath
       fs.createReadStream(tutil.caCert).pipe(fs.createWriteStream(absoluteTestChaincodePath + '/certificate.pem'));
    }
    var req = {
        fcn: "init",
        args: [],
        certificatePath: deployCert,
        metadata: new Buffer("assigner")
    };

    if (tutil.deployMode === 'dev' ) {
       req.chaincodeName = chaincodeName;
    } else {
       req.chaincodePath = testChaincodePath;
    }
    var tx = deployer.deploy(req);
    tx.on('submitted', function (results) {
        console.log("deploy submitted: %j", results);
    });
    tx.on('complete', function (results) {
        console.log("deploy complete: %j", results);
        chaincodeID = results.chaincodeID;
        console.log("chaincodeID:" + chaincodeID);
        return cb();
    });
    tx.on('error', function (err) {
        console.log("deploy error: %j", err.toString());
        return cb(err);
    });
}

function assignOwner(user,owner,cb) {
    var req = {
        chaincodeID: chaincodeID,
        fcn: "assign",
        args: ["MyAsset",owner],
        attrs: ['role']
    };
    console.log("assign: invoking %j",req);
    var tx = user.invoke(req);
    tx.on('submitted', function (results) {
        console.log("assign transaction ID: %j", results);
    });
    tx.on('complete', function (results) {
        console.log("assign invoke complete: %j", results);
        return cb();
    });
    tx.on('error', function (err) {
        console.log("assign invoke error: %j", err);
        return cb(err);
    });
}

// Check to see if the owner of the asset is
function checkOwner(user,ownerAccount,cb) {
    var req = {
        chaincodeID: chaincodeID,
        fcn: "query",
        args: ["MyAsset"]
    };
    console.log("query: querying %j",req);
    var tx = user.query(req);
    tx.on('complete', function (results) {
       var realOwner = results.result;
       //console.log("realOwner: " + realOwner);

       if (ownerAccount == results.result) {
          console.log("correct owner: %s",ownerAccount);
          return cb();
       } else {
          return cb(new Error(util.format("incorrect owner: expected=%s, real=%s",ownerAccount,realOwner)));
       }
    });
    tx.on('error', function (err) {
        console.log("assign invoke error: %j", err);
        return cb(err);
    });
}

test('setup asset management with roles', function (t) {
    t.plan(1);

    console.log("setup asset management with roles");

    setup(function(err) {
        if (err) {
            t.fail("error: "+err.toString());
            // Exit the test script after a failure
            process.exit(1);
        } else {
            t.pass("setup successful");
        }
    });
});

test('assign asset management with roles', function (t) {
    t.plan(1);
    alice.getUserCert(["role", "account"], function (err, aliceCert) {
        if (err) {
          fail(t, "Failed getting Application certificate for Alice.");
          // Exit the test script after a failure
          process.exit(1);
        }
        assignOwner(assigner, aliceCert.encode().toString('base64'), function(err) {
            if (err) {
                t.fail("error: "+err.toString());
                // Exit the test script after a failure
                process.exit(1);
            } else {
                checkOwner(assigner, aliceAccount, function(err) {
                    if(err){
                        t.fail("error: "+err.toString());
                        // Exit the test script after a failure
                        process.exit(1);
                    } else {
                        t.pass("assign successful");
                    }
                });
            }
        });
    });
});

test('not assign asset management with roles', function (t) {
    t.plan(1);

    bob.getUserCert(["role", "account"], function (err, bobCert) {
        if (err) {
          fail(t, "Failed getting Application certificate for Alice.");
          // Exit the test script after a failure
          process.exit(1);
        }
        assignOwner(alice, bobCert.encode().toString('base64'), function(err) {
            if (err) {
                t.fail("error: "+err.toString());
                // Exit the test script after a failure
                process.exit(1);
            } else {
                checkOwner(alice, bobAccount, function(err) {
                    if(err){
                        t.pass("assign successful");
                    } else {
                        err = new Error ("this user should not have been allowed to assign");
                        t.fail("error: "+err.toString());
                        // Exit the test script after a failure
                        process.exit(1);
                    }
                });
            }
        });
    });
});

function registerAndEnroll(name, attrs, cb) {
    console.log("registerAndEnroll name=%s attrs=%j",name,attrs);
    var registrationRequest = {
         roles: [ 'client' ],
         enrollmentID: name,
         affiliation: "bank_a",
         attributes: attrs
    };
    chain.registerAndEnroll(registrationRequest,cb);
}

