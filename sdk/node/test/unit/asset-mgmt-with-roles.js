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
var crypto = require('crypto');
var fs  = require('fs');
var tutil = require('test/unit/test-util.js');
var x509 = require('x509');

var chain, chaincodeID;
var chaincodeName = "mycc3";
var deployer, alice, bob, assigner;

var aliceAccount = "12345-56789";
var aliceAttributeList=["role", "account" ]
var aliceAttributeValueList=["client", aliceAccount ]

var bobAccount = "23456-67890";
var bobAttributeList=["role", "account" ]
var bobAttributeValueList=["client", bobAccount ]

var username = process.env.SDK_DEFAULT_USER
var usersecret = process.env.SDK_DEFAULT_SECRET
var goPath = process.env.GOPATH
var testChaincodePath = process.env.SDK_CHAINCODE_PATH
 ? process.env.SDK_CHAINCODE_PATH
 : "github.com/asset_management_with_roles/" ;
var absoluteTestChaincodePath = goPath + "/src/" + testChaincodePath;

var devMode = ( process.env.SDK_DEPLOY_MODE == 'dev')
 ? true
 : false;
var peerAddr0 = process.env.SDK_PEER_ADDRESS
 ? process.env.SDK_PEER_ADDRESS
 : "localhost:7054" ;
var caCert   = process.env.SDK_CA_CERT_FILE
 ? process.env.SDK_CA_CERT_FILE
 : "tlsca.cert" ;

var chainDeployer = {
    name: username ? username : 'WebAppAdmin',
    secret: usersecret ? usersecret : 'DJY27pEnl16d'
};

var chainAssigner = {
    name: 'assigner',
    secret: 'Tc43PeqBl11'
};

var chainUser1 = {
    name: 'alice',
    secret: 'CMS10pEQlB16'
};

var chainUser2 = {
    name: 'bob',
    secret: 'NOE63pEQbL25'
};

// Given a certificate byte buffer of the DER-encoded certificate, return
// a PEM-encoded (64 chars/line) string with the appropriate header/footer
function certToPEM(cert, cb) {
    var pem = cert.encode().toString('base64');
    certStr = "-----BEGIN CERTIFICATE-----\n"
    for (var i = 0; i < pem.length; i++) {
       if ((i>0) && i%64 == 0) certStr += "\n";
       certStr += pem[i]
    }
    certStr += "\n-----END CERTIFICATE-----\n"
    cb(certStr)
}

// Validate that the correct x509 v3 extensions containing
// attribute-entries were added to the certificate
function checkCertExtensions(certbuf, attV, cb) {
    var pem = certbuf.encode().toString('base64');
    certToPEM(certbuf, function(certPem) {
       var c = x509.parseCert(certPem)
       console.log("CN: ", c.subject.commonName);
       console.log("extensions: ");
       var extArrVal  = [];
       Object.keys(c.extensions).forEach(function(key) {
           var attrOid = key;
           var attrVal = c.extensions[key];
           extArrVal.push(attrVal);
           console.log("   --->", attrOid, ":", attrVal);
       });
       for (var a in attV) {
          if ( extArrVal.indexOf(attV[a]) == -1) {
             console.log( attV[a], "not in extentions");
             return cb(new Error(util.format("Failed to find %s in certificate extentions", attV[a])));
          }
       }
    });
    return cb();
}

// Create the chain and enroll users as deployer, assigner, and nonAssigner (who doesn't have privilege to assign.
function setup(cb) {
   console.log("initializing ...");
   var chain = tutil.getTestChain();
   if (devMode) chain.setDevMode(true);
   console.log("enrolling " + chainDeployer.name + "...");
   chain.enroll(chainDeployer.name, chainDeployer.secret, function (err, user) {
      if (err) return cb(err);
      deployer = user;
      console.log("enrolling " + chainAssigner.name + "...");
      chain.enroll(chainAssigner.name, chainAssigner.secret, function (err, user) {
         if (err) return cb(err);
         assigner = user;
         console.log("enrolling " + chainUser1.name + "...");
         chain.enroll(chainUser1.name, chainUser1.secret, function (err, user) {
            if (err) return cb(err);
            alice = user;
            console.log("enrolling " + chainUser2.name + "...");
            chain.enroll(chainUser2.name, chainUser2.secret, function (err, user) {
               if (err) return cb(err);
               bob = user;
               return deploy(cb);
            });
         });
      });
   });
}

// Deploy asset_management_with_roles with the name of the assigner role in the metadata
function deploy(cb) {
    if (tutil.tlsOn) {
       var deployCert = tutil.caCert
       if (peerAddr0.match(tutil.hsbnDns)) deployCert = tutil.hsbnCertPath
       else if (peerAddr0.match(tutil.bluemixDns)) deployCert = tutil.bluemixCertPath
       // Path (under $GOPATH) required for deploy in network mode
       fs.createReadStream(caCert).pipe(fs.createWriteStream(absoluteTestChaincodePath + '/certificate.pem'));
    }

    console.log("deploying with the role name 'assigner' in metadata ...");
    var req = {
        fcn: "init",
        args: [],
        certificatePath: deployCert,
        metadata: new Buffer("assigner")
    };
    if (devMode) {
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
        userCert: user.cert,
        attrs: aliceAttributeList
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
    alice.getUserCert(aliceAttributeList, function (err, aliceCert) {
        if (err) t.fail(t, "Failed getting Application certificate for Alice.");
        checkCertExtensions(aliceCert, aliceAttributeValueList, function(err) {
            if(err){
                t.fail("error: "+err.toString());
            }
        });
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
    })
});

test('not assign asset management with roles', function (t) {
    t.plan(1);

    bob.getUserCert(["role", "account"], function (err, bobCert) {
        if (err) t.fail(t, "Failed getting Application certificate for Bob.");
        checkCertExtensions(bobCert, bobAttributeValueList, function(err) {
            if(err){
                t.fail("error: "+err.toString());
            }
        });
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
