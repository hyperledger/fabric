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

//
// Load required packages.
//

var fs = require('fs');
var tar = require("tar-fs");
var grpc = require('grpc');
var uuid = require('node-uuid');
var path = require("path");
var zlib = require("zlib");

var debugModule = require('debug');
let debug = debugModule('hfc'); // 'hfc' stands for 'HyperLedger Fabric Client'

//
// Load required crypto stuff.
//

var sha3_256 = require('js-sha3').sha3_256;

//
// Load required protobufs.
//

var _timeStampProto = grpc.load(__dirname + "/protos/google/protobuf/timestamp.proto").google.protobuf.Timestamp;

//
// GenerateUUID returns an RFC4122 compliant UUID.
// http://www.ietf.org/rfc/rfc4122.txt
//

export function GenerateUUID() {
  return uuid.v4();
};

//
// GenerateTimestamp returns the current time in the google/protobuf/timestamp.proto
// structure.
//

export function GenerateTimestamp() {
  var timestamp = new _timeStampProto({ seconds: Date.now() / 1000, nanos: 0 });
  return timestamp;
}

//
// GenerateParameterHash generates a hash from the chaincode deployment parameters.
// Specifically, it hashes together the code path of the chaincode (under $GOPATH/src/),
// the initializing function name, and all of the initializing parameter values.
//

export function GenerateParameterHash(path, func, args) {
  debug("GenerateParameterHash...");

  debug("path: " + path);
  debug("func: " + func);
  debug("args: " + args);

  // Append the arguments
  var argLength = args.length;
  var argStr = "";
  for (var i = 0; i < argLength; i++) {
    argStr = argStr + args[i];
  }

  // Append the path + function + arguments
  var str = path + func + argStr;
  debug("str: " + str);

  // Compute the hash
  var strHash = sha3_256(str);
  debug("strHash: " + strHash);

  return strHash;
}

//
// GenerateDirectoryHash generates a hash of the chaincode directory contents
// and hashes that together with the chaincode parameter hash, generated above.
//

export function GenerateDirectoryHash(rootDir, chaincodeDir, hash) {
  var self = this;

  // Generate the project directory
  var projectDir = rootDir + "/" + chaincodeDir;

  // Read in the contents of the current directory
  var dirContents = fs.readdirSync(projectDir);
  var dirContentsLen = dirContents.length;

  // Go through all entries in the projet directory
  for (var i = 0; i < dirContentsLen; i++) {
    var current = projectDir + "/" + dirContents[i];

    // Check whether the entry is a file or a directory
    if (fs.statSync(current).isDirectory()) {
      // If the entry is a directory, call the function recursively.

      hash = self.GenerateDirectoryHash(rootDir, chaincodeDir + "/" + dirContents[i], hash);
    } else {
      // If the entry is a file, read it in and add the contents to the hash string

      // Read in the file as buffer
      var buf = fs.readFileSync(current);
      // Update the value to be hashed with the file content
      var toHash = buf + hash;
      // Update the value of the hash
      hash = sha3_256(toHash);
    }
  }

  return hash;
}

//
// GenerateTarGz creates a .tar.gz file from contents in the src directory and
// saves them in a dest file.
//

export function GenerateTarGz(src, dest, cb) {
  debug("GenerateTarGz");

  // A list of file extensions that should be packaged into the .tar.gz.
  // Files with all other file extenstions will be excluded to minimize the size
  // of the deployment transaction payload.
  var keep = [
    ".go",
    ".yaml",
    ".json",
    ".c",
    ".h",
    ".pem"
  ];

  // Create the pack stream specifying the ignore/filtering function
  var pack = tar.pack(src, {
    ignore: function(name) {
      // Check whether the entry is a file or a directory
      if (fs.statSync(name).isDirectory()) {
        // If the entry is a directory, keep it in order to examine it further
        return false
      } else {
        // If the entry is a file, check to see if it's the Dockerfile
        if (name.indexOf("Dockerfile") > -1) {
          return false
        }

        // If it is not the Dockerfile, check its extension
        var ext = path.extname(name);

        // Ignore any file who's extension is not in the keep list
        if (keep.indexOf(ext) === -1) {
          return true
        } else {
          return false
        }
      }
    }
  })
  .pipe(zlib.Gzip())
  .pipe(fs.createWriteStream(dest));

  pack.on("close", function() {
    return cb(null);
  });
  pack.on("error", function() {
    return cb(Error("Error on fs.createWriteStream"));
  });
}
