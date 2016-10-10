//
// Run the getChain tests
//
var hfc = require('hfc');
var test = require('tape');
var util = require('util');
var fs = require('fs');
var tutil = require('test/unit/test-util.js');

function pass(t, msg) {
    t.pass("Success: [" + msg + "]");
    t.end();
}

function fail(t, msg, err) {
    t.fail("Failure: [" + msg + "]: [" + err + "]");
    t.end(err);
}

function getChainTests(cb) {
   console.log("getChainTests");
   var absentChain = "pizza crust";
   var existingChain = "testChain";

   var newChainNames = ["Rottweiler", "Dachshund", "Beagle", "Collie", "Wire Haired Fox Terrier",
                        "Norwegian Elk Hound", "Blue Tick Hound", "Golden Retriever", "Greyhound"];
   var name = "____testChain"
   var create0 = 0;
   var create1 = 1;
   var createF = false;
   var createT = true;
   var flags = [ 0, 1, false, true, create0, create1, createF, createT, 12]; // Define one newChainNames[] element per flag

   // Verify that fetch only existing chains when create flag is not specified
   var ch = hfc.getChain(absentChain);

   if (ch) {
      console.log("chain %s unexpectedly found", absentChain);
      return cb(Error("Chain pizza crust unexpectedly found"));
   }

   var T = Boolean(true)
   hfc.getChain(existingChain, T);
   ch = hfc.getChain(existingChain);

   if (!ch) {
      console.log("chain " + existingChain + "expected but not found");
      return cb(Error("Chain expected but not found", existingChain));
   }


   if (newChainNames.length != flags.length) {
      return cb(Error("Test case flaw: flags.length != newChainNames.length "))
   }

   //
   // Test getting existing chain using the different flags
   var results = [];
   for (var i = 0; i < flags.length; i++) {
      results[i] = hfc.getChain(existingChain, flags[i]);
      //console.log("results for " + i + " " + results[i]);
      if (i > 0) {
         if (results[i] != results[i-1]) {
            return cb(Error("unexpected result[" + i + "] for existing chain " + existingChain + " and flag " + flags[i]));
         }
      }
   }

   //
   // Test non existing chain with various create flags
   for (var i = 0; i < flags.length; i++) {
      results[i] = hfc.getChain(newChainNames[i], flags[i]);

      if (flags[i] && results[i]) { /* Create flag specified and a chain is returned */
         continue;
      }
      if (!flags[i] && !results[i]) { /* create flag not specified and no chain returned */
         continue;
      }
      var s = "getChainTests: unexpected result for flag " + flags[i] + " name " + newChainNames[i];
      return cb(s);
   }
   return cb();
} /* getChainTests() */

test('getChain tests', function (t) {
   getChainTests(function(err) {
      if (err) {
         fail(t, "getChain tests", err);
         // Exit the test script after a failure
      } else {
         pass(t, "getChain tests");
      }
   });
}); /* getChain tests */
