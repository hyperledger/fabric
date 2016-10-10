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
var fs = require('fs');
var tutil = require('test/unit/test-util.js');

var keyValStorePath = "/tmp/keyValStore"
var keyValStorePath2 = keyValStorePath + "2";
var keyValStorePath3 = keyValStorePath + "3";

//
// Run the registrar test
//
test('registrar test', function (t) {
    registrarTest(function(err) {
        if (err) {
          fail(t, "registrarTest", err);
          // Exit the test script after a failure
          process.exit(1);
        } else {
          pass(t, "registrarTest");
        }
    });
});

//
// Run the enroll again test
//
test('enroll again', function (t) {
    enrollAgain(function(err) {
        if (err) {
          fail(t, "enrollAgain", err);
          // Exit the test script after a failure
          process.exit(1);
        } else {
          pass(t, "enrollAgain");
        }
    });
});

var chain,admin,webAdmin,webUser;

// STEP1: Init the chain and enroll admin
function registrarTest(cb) {
   console.log("registrarTest: STEP 1");
   //
   // Create and configure the test chain
   //
   chain = tutil.getTestChain("testChain");

   chain.enroll("admin", "Xurw3yU9zI0l", function (err, user) {
      if (err) return cb(err);
      admin = user;
      chain.setRegistrar(admin);
      registrarTestStep2(cb);
   });
}

// STEP 2: Register and enroll webAdmin and set as register
function registrarTestStep2(cb) {
   console.log("registrarTest: STEP 2");
   registerAndEnroll("webAdmin", "client", null, {roles:['client']}, chain, function(err,user) {
      if (err) return cb(err);
      webAdmin = user;
      chain.setRegistrar(webAdmin);
      registrarTestStep3(cb);
   });
}

// STEP 3: Register webUser with an attribute"
function registrarTestStep3(cb) {
   console.log("registrarTest: STEP 3");
   var attrs;
   attrs = [{name:'foo',value:'bar'}];
   registerThenEnroll("webUser", "client", attrs, webAdmin, chain, function(err, user) {
      if (err) return cb(err);
      webUser = user;
      registrarTestStep4(cb);
   });
}

// STEP 4: Negative test attempting to register an auditor type by webAdmin
function registrarTestStep4(cb) {
   console.log("registrarTest: STEP 4");
   registerAndEnroll("auditorUser", "auditor", null, null, chain, function(err) {
      if (!err) return cb(Error("webAdmin should not have been allowed to register member of type auditor"));
      err = checkErr(err,"webAdmin may not register member of type auditor");
      if (err) return cb(err);
      registrarTestStep5(cb);
   });
}

// STEP 5: Negative test attempting to register a validator type by webAdmin
function registrarTestStep5(cb) {
   console.log("registrarTest: STEP 5");
   registerAndEnroll("validatorUser", "validator", null, null, chain, function(err) {
      if (!err) return cb(Error("webAdmin should not have been allowed to register member of type validator"));
      err = checkErr(err,"webAdmin may not register member of type validator");
      if (err) return cb(err);
      registrarTestStep6(cb);
   });
}

// STEP 6: Negative test attempting to register a client type by webUser
function registrarTestStep6(cb) {
   console.log("registrarTest: STEP 6");
   chain.setRegistrar(webUser);
   registerAndEnroll("webUser2", "client", null, webUser, chain, function(err) {
      if (!err) return cb(Error("webUser should not be allowed to register a member of type client"));
      err = checkErr(err,"webUser may not register member of type client");
      if (err) return cb(err);
      registrarTestDone(cb);
   });
}

// Final step: success
function registrarTestDone(cb) {
   console.log("registrarTest: Done");
   return cb();
}

// Register and enroll user 'name' with role 'role' with registrar info 'registrar' for chain 'chain'
function registerAndEnroll(name, role, attributes, registrar, chain, cb) {
    console.log("registerAndEnroll %s",name);
    // User is not enrolled yet, so perform both registration and enrollment
    var registrationRequest = {
         roles: [ role ],
         enrollmentID: name,
         affiliation: "bank_a",
         attributes: attributes,
         registrar: registrar
    };
    chain.registerAndEnroll(registrationRequest,cb);
}

// Register, then enroll user 'name' with role 'role' with registrar info 'registrar' for chain 'chain'
// This drives standalone chain.register() and chain.enroll() functions
function registerThenEnroll(name, role, attributes, registrar, chain, cb) {
    console.log("registerThenEnroll %s",name);
    // User is not enrolled yet, so perform both registration and enrollment
    var registrationRequest = {
         roles: [ role ],
         enrollmentID: name,
         affiliation: "bank_a",
         attributes: attributes,
         registrar: registrar
    };
    // Test chain.register()
    chain.register(registrationRequest, function(err, enrollmentPassword) {
        if (err) {
            console.log("registerThenEnroll: couldn't register name ", err)
            return cb(err);
        }
        // Fetch name's member so we can set the Registrar
        chain.getMember(registrar, function(err, member) {
            if (err) {
                console.log("could not get member for ", name, err);
                return cb(err);
            }
            //console.log("I did find this member", member)
            chain.setRegistrar(member);
            });

        // Test chain.enroll using password returned by chain.register()
        chain.enroll(name, enrollmentPassword, function(err, member) {
        //console.log("am I defined?", cb)  /* yes, defined */
            if (err) {
                console.log("registerThenEnroll: enroll failed", err);
                return cb(err);
            }
            //console.log("registerThenEnroll: enroll succeeded for registration request =", registrationRequest);
            return cb(err, member);
        });
   });   /* end chain.register */
}  /* registerThenEnroll */

// Force the client to try to enroll admin again by creating a different chain
// This should fail.
function enrollAgain(cb) {
   console.log("enrollAgain");
   //
   // Remove the file-based keyValStore
   // Create and configure testChain2 so there is no shared state with testChain
   // This is necessary to start without a local cache.
   //
   fs.renameSync(keyValStorePath,keyValStorePath2);
   var chain = tutil.getTestChain("testChain2");

   //console.log("enrollAgain: chain.getDeployWaitTime() = ", x)
   //var chain2 = hfc.newChain(42)
   //console.log("enrollAgain: here's my chain: %v", chain)

   chain.enroll("admin", "Xurw3yU9zI0l", function (err, admin) {
      rmdir(keyValStorePath);
      fs.renameSync(keyValStorePath2,keyValStorePath);
      if (!err) return cb(Error("admin should not be allowed to re-enroll"));
      return cb();
   });
}

function rmdir(path) {
  if( fs.existsSync(path) ) {
    fs.readdirSync(path).forEach(function(file,index){
      var curPath = path + "/" + file;
      if(fs.lstatSync(curPath).isDirectory()) { // recurse
        rmdir(curPath);
      } else { // delete file
        fs.unlinkSync(curPath);
      }
    });
    fs.rmdirSync(path);
  }
}

function checkErr(err,expect) {
   err = err.toString();
   var found = err.match(expect);
   if (!found || found.length === 0) {
      err = new Error(util.format("Incorrect error: expecting '%s' but found '%s'",expect,err));
      return err;
   }
   return null;
}

function pass(t, msg) {
    t.pass("Success: [" + msg + "]");
    t.end();
}

function fail(t, msg, err) {
    t.fail("Failure: [" + msg + "]: [" + err + "]");
    t.end(err);
}
