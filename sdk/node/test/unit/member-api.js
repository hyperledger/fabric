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
var tutil = require('test/unit/test-util.js');
var fs = require('fs');

// constants
var registrar = {
    name: 'WebAppAdmin',
    secret: 'DJY27pEnl16d'
    //name: 'alice',
    //secret: 'CMS10pEQlB16'
};

var badRegistrar = {
    name : 'test_user0',
    secret: 'MS9qrN8hFjlE'
};


//
//  Create and configure a test chain
//
var chain = tutil.getTestChain("testChain");

function arraysEqual(a, b) {
  if (a === b) return true;
  if (a == null || b == null) return false;
  if (a.length != b.length) return false;

  // If you don't care about the order of the elements inside
  // the array, you should sort both arrays here.

  for (var i = 0; i < a.length; ++i) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}

/**
 * Get the user and if not enrolled, register and enroll the user.
 */
function getUser(name, cb) {
    // setRegistrar is presumed to have been done previously
    // the second test in the below sequence sets the registrar
    chain.getUser(name, function (err, user) {
        //console.log(name + ".getAffiliation() = ", user.getAffiliation());
        //console.log(name + "user.getRoles()", user.getRoles());
        if (err) return cb(err);
        if (user.isEnrolled()) return cb(null, user);
        // User is not enrolled yet, so perform both registration and enrollment
        var registrationRequest = {
            enrollmentID: name,
            affiliation: "bank_c"
        };
        user.registerAndEnroll(registrationRequest, function (err) {
            if (err) cb(err, null)
            cb(null, user)
                console.log("getName enrolled " + user.getName());
                console.log("getChain.getName " + user.getChain().getName);
                console.log("getAffiliation " + user.getAffiliation());
                console.log("getChain " + user.getChain());
                console.log("getEnrollment" + user.getEnrollment());
        console.log("getMemberServices ", user.getMemberServices());
        console.log("getRoles", user.getRoles());
        console.log("getTCertBatchSize", user.getTCertBatchSize());
        //console.log("user.getRoles()", user.getRoles());
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


//test if a non-register role user can be enrolled and set as registrar successfully
// Omitted this test due to FAB-152 fixed as "can't/won't fix"
test.skip('-Ve Test Enroll the registrar', function (t) {

    // Get the non-webappadmin member details
    chain.getUser(badRegistrar.name, function (err, user) {
        if (err) {
            fail(t, "-ve Test get registrar", err);
            // Exit the test script after a failure
            //process.exit(1);
        }
        user.enroll(badRegistrar.secret, function (err) {
            if (err) {
                fail(t, "-ve Test enroll registrar", err);
                // Exit the test script after a failure
                //process.exit(1);
            }
            try {
              //chain.setRegistrar(user);
              console.log(user)
              fail(t, "How can I set test_user0 as registrar ");
            } catch (e) {
              console.log(e);
            }
        });
    });
});



test('Enroll the registrar', function (t) {
    // Get the WebAppAdmin member
    chain.getUser(registrar.name, function (err, user) {
        if (err) {
            fail(t, "get registrar", err);
            // Exit the test script after a failure
            process.exit(1);
        }
        // Enroll the WebAppAdmin user with the certificate authority using
        // the one time password hard coded inside the membersrvc.yaml.
        user.enroll(registrar.secret, function (err) {
            if (err) {
                fail(t, "enroll registrar", err);
                // Exit the test script after a failure
                process.exit(1);
            }
            chain.setRegistrar(user);
            //console.log(user)
            pass(t, "enrolled " + registrar.name);
        });
    });
});

//test enroll Alice
test('Enroll Alice', function (t) {
    getUser('Alice', function (err, user) {
        if (err) {
            fail(t, "enroll Alice", err);
            // Exit the test script after a failure
            process.exit(1);
        }
        alice = user;
        alice.getUserCert(null, function (err, userCert) {
            if (err) {
                fail(t, "Failed getting Application certificate.");
                // Exit the test script after a failure
                process.exit(1);
            }
            alicesCert = userCert;
            pass(t, "enroll Alice");
        })
    });
});

//test isRegistered() API
test('isRegistered', function (t) {
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("After getUser ", user)
    alice = user;
    //console.log("from isRegistered: 2 %s", alice)
    if (alice.isRegistered()) {
      //console.log(" Alice is isRegistered true ")
      pass(t, "isRegistered ");
    } else {
      console.log("Alice isRegistered false")
      fail(t, "isRegistered ");
    }
  })
});

//isRegisterd() API -ve test
test('isRegistered -Ve', function (t) {
  //var XYZ123 = user;
  //console.log("from isRegistered: 2 %s", alice)
  chain.getUser("AAA12345", function (err, user) {
      if (err)  {
        //console.log("AAA1234 user not found ")
        fail(t, "isRegistered -Ve");
      }
      //console.log("XYZ1234 user found ", user)
      var XYZ1234 = user
      if (XYZ1234.isRegistered()) {
        console.log("How can this user be registered??? ", user)
        fail(t, "isRegistered -Ve");
      } else {
        //console.log("XYZ1234 isRegistered false ")
        pass(t, "isRegistered -Ve");
      }
  });
});

//test chain.getRegistrar() API
test('getRegistrar', function (t) {
    mem = chain.getRegistrar()
    if (mem.getName() == "WebAppAdmin") {
      console.log("from getRegistrar: ", mem)
      pass(t, "getRegistrar");
    } else {
      fail(t, "getRegistrar");
    }

});

//test getAffiliation API
test.skip('getAffiliation', function (t) {

    console.log("getAffiliation: blocked by issue FAB-218 ");

    getUser('Alice1', function (err, user) {
        if (err) return cb(err);
        //console.log("got user Alice ");
        alice = user;
        aff = alice.getAffiliation()
        if (aff != null) {
            console.log("result from getAffiliation ", aff)
            pass(t, "getAffiliation");
        } else {
        fail(t, "getAffiliaton", aff);
        }
    })
});


//test setAffiliation() API
test('setAffiliation', function (t) {
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice")
    alice = user;
    console.log("Setting affiliation to bank_b")
    alice.setAffiliation("bank_b")
    aff = alice.getAffiliation()
    if ( aff.toString().trim() === "bank_b") {
      pass(t, "setAffiliation");
    } else {
      fail(t, "setAffiliation");
    }
  })
});


//test getEnrollment() API
test('getEnrollment', function (t) {
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice  ")
    alice = user;
    enr = alice.getEnrollment()
    if ( enr != null) {
        pass(t, "getEnrollment data: ", enr);
    } else {
      fail(t, "getEnrollment");
    }
  })
});


//test getName()
test('getName', function (t) {
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice ")
    alice = user;
    name = alice.getName()
    if ( name == "Alice") {
      console.log("result from getName ", name)
      pass(t, "getName");
    } else {
      fail(t, "getName", name);
    }
  })
});

//test getNextTCert() API
test.skip('getNextTCert', function (t) {
    getUser('Alice', function (err, user) {
        if (err) return cb(err);
        //console.log("got user Alice")
        alice = user;
        alice.getNextTCert(['role'],function(err, tcert) {
          if (err) {
             fail(t, "getNextTCert", tcert);
              //continues to next test case after throwing error
             throw err;
          } else {
            //console.log("getNextTcert passed", tcert)
            pass(t, "getNextTCert");

          }
        })
   })
});


//test getNextTCert() API
test.skip('getNextTCertNegativeTest', function (t) {
    console.log("getNextTCertNegativeTest: blocked by issue FAB-219");
    getUser('Alice', function (err, user) {
        if (err) return cb(err);
        alice = user
        alice.getNextTCert(['fail'],function(err, tcert) {
        if (err) {
            console.log("We want this API to fail, since we are passing invalid attribute")
            pass(t, "getNextTCertNegativeTest");
            //throw err;
        } else {
            console.log("GetNextTCertNegativeTest: getting cert for invalid attribute", tcert)
            fail(t, "getNextTCertNegativeTest");
        }
     })
   })
});

// test getUserCert() API
test.skip('getUserCertTest', function (t) {
      getUser('Alice', function (err, user) {
        if (err) return cb(err);
        //console.log("got user Alice ")
        alice = user
        alice.getUserCert(['role'], function(err, ucert) {
        if (err) {
             fail(t, "UserCertTest", ucert);
             throw err;
        } else {
            pass(t, "UserCertTest");
            //console.log("UserCertTest passed", ucert)
        }
     })
   })
});


//test getRoles() API
test.skip('getRoles', function (t) {
    console.log("getRoles: blocked by issue FAB-217");
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice ")
    currRole = user.getRoles();
    if (currRole){
        pass(t, "getRoles");
    } else {
        fail(t, "getRoles");
    }
    });
});

//test get/setTCertBatchSize() API
test('getTCertBatchSize', function (t) {
  bSize = chain.getTCertBatchSize()

  chain.setTCertBatchSize(1000)

  bSize = chain.getTCertBatchSize()
  if (bSize == 1000) {
    pass(t, "getTCertBatchSize after setTCertBatchSize: ", bSize);
  } else {
    console.log("from test TCertBatchSize batchSize %d is not a expected as 1000", bSize)
    fail(t, "getTCertBatchSize after setTCertBatchSize fails: ", bSize);
  }
});


//test toString() API
test('toString', function (t) {
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice ")
    alice = user
    stateAsString = alice.toString()
    if (stateAsString.slice(0,38) === '{"name":"Alice","affiliation":"bank_b"') {
       stateKeys = JSON.parse(alice.toString())
       expectedKeys = [ 'name', 'affiliation', 'enrollmentSecret', 'enrollment' ]
       if (arraysEqual(Object.keys(stateKeys), expectedKeys)) {
          console.log("PASSED")
          pass(t, "StringToObect");
       } else {
          console.log("FALIED")
          fail(t, "StringToObject")
       }
    } else {
       fail(t, "ObjectToString");
    }

  })
});


//test newTransactionContext() API
test.skip('newTransactionContext', function (t) {
  //console.log("from newTransactionContext: 1")
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice ")
    alice = user
    alice.getNextTCert(['role'],function(err, tcert) {
    if (err) return cb(err);
    var nTxContext = alice.newTransactionContext(tcert);
    if (nTxContext != null) {
        //console.log("from newTransactionContext: ", nTxContext)
        pass(t, "newTransactionContext");
    } else {
        console.log("from newTransactionContext: ", nTxContext)
        fail(t, "newTransactionContext", nTxContext);
    }
    })
 })
});


//test saveState() API
test('saveState', function (t) {
  getUser('Alice', function (err, user) {
      if (err) return cb(err);
      //console.log("got user Alice ")
      alice = user
      alice.saveState(function (err) {
        if (err) {
          fail(t, "saveState", err);
        }
        pass(t, "saveState");
      })
    })
});

//test restoreState() API
test('restoreState', function (t) {
  getUser('Alice', function (err, user) {
    if (err) return cb(err);
    //console.log("got user Alice ")
    alice = user
    alice.restoreState( function (err) {
        if (err) {
          fail(t, "restoreState: User state could not be restored", err);
          return cb(err);
        }
        pass(t, "restoreState");
      })
    })
});
