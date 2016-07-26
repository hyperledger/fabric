#!/usr/bin/env node
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

var hfc = require('..');
var fs = require('fs');
var util = require('util');

function usage(msg) {
    if (msg) console.log("ERROR: %s",msg);
    console.log("Usage: hfc test <config-file>");
    process.exit(0);
}

function main() {
   var argv = process.argv.splice(2);
   if (argv.length < 1) usage();
   var cmd = argv[0];
   argv = argv.splice(1);
   if (cmd == 'test') {
      if (argv.length != 1) usage("missiing <test.json> argument");
      var cfgFile = argv[0];
      if ((!cfgFile.indexOf(".")) || (!cfgFile.indexOf("/"))) {
         cfgFile = "./"+cfgFile;
      }
      runTests(cfgFile);
   } else {
      usage("invalid command: "+cmd);
   }

}

function runTests(cfgFile) {

    console.log("loading config file: %s",cfgFile);
    var cfg = require('./'+cfgFile);

    console.log("configuring chain ...");
    var chain = hfc.newChain("testChain");
    // Set member services URL
    chain.setMemberServicesUrl(getVal(cfg,"memberServicesUrl"));
    // Set keyValStore
    chain.setKeyValStore(hfc.newFileKeyValStore('/tmp/keyValStore'));
    // Add peeers
    var peers = getVal(cfg,"peers");
    for (var i = 0; i < peers.length; i++) {
        chain.addPeer(peers[i]);
    }

    // Enroll the registrar
    console.log("enrolling registrar ...");
    var registrar = getVal(cfg,"registrar");
    chain.enroll(registrar.name,registrar.secret,function(err,user) {
        if (err) throw Error("Failed enrolling registrar: "+err);
        chain.setRegistrar(user);
        // Initialization complete, start tests
        console.log("beginning tests");
        var numUsers = getVal(cfg,"numUsers");
        var iterations = getVal(cfg,"iterations");
        var tests = getVal(cfg,"tests");
        // Register and enroll all users and start tests in parallel
        for (var userNum = 0; userNum < numUsers; userNum++) {
            var userName = "user." + userNum + "-" + new Date().getTime();
            console.log("registering %s",userName);
            var registration = {
                enrollmentID: userName,
                affiliation: "bank_a"
            };
            chain.registerAndEnroll(registration,function(err,user) {
               if (err) throw Error("failed to register and enroll "+userName+": "+err);
               runUser(user,0,tests,0,iterations,chain);
            });
        }
    });
}

function runUser(user,testIdx,tests,curCount,maxCount,chain) {
    if (curCount >= maxCount) {
        console.log("completed tests for user %s",user.getName());
        return;
    }
    if (testIdx >= tests.length) {
        curCount++;
        console.log("completed iteration %d for user %s",curCount,user.getName());
        runUser(user,0,tests,curCount,maxCount,chain);
        return;
    }
    var test = tests[testIdx];
    var type = test.type.toLowerCase();
    var tx;
    if (type == 'invoke') {
       tx = user.invoke(test);
    } else if (type == 'query') {
       tx = user.query(test);
    } else if (type == 'deploy') {
       tx = user.deploy(test);
    } else {
       throw Error("invalid test type: "+type);
    }
    tx.on("submitted",function(results) {
        console.log("submitted: results=%s, user=%s, test=%d, iteration",results,user.getName(),testIdx,curCount);
        // Run the next test for this user
        // TODO: Move this to complete event once the complete event works correctly in SDK
        runUser(user,testIdx+1,tests,curCount,maxCount,chain);
    });
    tx.on("complete",function(results) {
        console.log("complete: results=%j, user=%s, test=%d, iteration",results,user.getName(),testIdx,curCount);
    });
    tx.on("error",function(err) {
        console.log("error: %j, user=%s, test=%d, iteration",err,user.getName(),testIdx,curCount);
    });
}

function runUserTest(userName,tests,curCount,maxCount,chain) {
    if (curCount >= maxCount) {
        return console.log("completed tests for user %s",userName);
    }
}

function getVal(cfg,name) {
    var val = cfg[name];
    if (!val) throw Error(util.format("config is missing '%s' field",name));
    return val;
}

main();
