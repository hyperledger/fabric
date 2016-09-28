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
var util = require('util');
var fs = require('fs');

//Set defaults, if not set
var deployMode = process.env.SDK_DEPLOY_MODE
 ? process.env.SDK_DEPLOY_MODE
 : "net" ;
var keyStore = process.env.SDK_KEYSTORE
 ? process.env.SDK_KEYSTORE
 : "/tmp/keyValStore" ;
var caCert   = process.env.SDK_CA_CERT_FILE
 ? process.env.SDK_CA_CERT_FILE
 : "tlsca.cert" ;
var caCertHost = process.env.SDK_CA_CERT_HOST
 ? process.env.SDK_CA_CERT_HOST
 : "" ;
var caAddr   = process.env.SDK_MEMBERSRVC_ADDRESS
 ? process.env.SDK_MEMBERSRVC_ADDRESS
 : "localhost:7054" ;
var peerAddr0 = process.env.SDK_PEER_ADDRESS
 ? process.env.SDK_PEER_ADDRESS
 : "localhost:7051" ;
var tlsOn    = ( process.env.SDK_TLS == null )
 ? Boolean(false)
 : Boolean(parseInt(process.env.SDK_TLS));
var deployWait  = process.env.SDK_DEPLOYWAIT
 ? process.env.SDK_DEPLOYWAIT
 : 20;
var invokeWait  = process.env.SDK_INVOKEWAIT
 ? process.env.SDK_INVOKEWAIT
 : 5;
var ciphers = process.env.GRPC_SSL_CIPHER_SUITES

console.log("deployMode      :"+deployMode );
console.log("keyStore        :"+keyStore );
console.log("caCert          :"+caCert   );
console.log("caAddr          :"+caAddr   );
console.log("peerAddr0       :"+peerAddr0 );
console.log("tlsOn           :"+tlsOn    );
console.log("deployWait      :"+deployWait  );
console.log("invokeWait      :"+invokeWait  );
console.log("ciphers         :"+ciphers  );

function getTestChain(name) {
   name = name || "testChain";
   var chain = hfc.newChain(name);

   chain.setKeyValStore(hfc.newFileKeyValStore(keyStore));
   if (tlsOn) {
      if (fs.existsSync(caCert)) {
         var pem = fs.readFileSync(caCert);
         if (caCertHost) { var grpcOpts={ pem:pem, hostnameOverride:'tlsca' } }
         else { var grpcOpts={ pem:pem } };

         console.log("Setting membersrvc address to grpcs://" + caAddr);
         console.log("Setting peer address to grpcs://" + peerAddr0);
         console.log("Setting cert to " + caCert);

         chain.setMemberServicesUrl("grpcs://" + caAddr, { pem:pem, hostnameOverride:'tlsca' } );
         chain.addPeer("grpcs://" + peerAddr0, { pem:pem, hostnameOverride:'tlsca' } );
      } else {
         console.log("TLS was requested but " + caCert + " not found.")
         process.exit(1)
      }
   } else {
      console.log("Setting membersrvc address to grpc://" + caAddr);
      console.log("Setting peer address to grpc://" + peerAddr0);
      chain.setMemberServicesUrl("grpc://" + caAddr);
      chain.addPeer("grpc://" + peerAddr0);
   }
   //
   // Set the chaincode deployment mode to either developent mode (user runs chaincode)
   // or network mode (code package built and sent to the peer).
   console.log("$SDK_DEPLOY_MODE: " + deployMode);
   if (deployMode === 'dev') {
       chain.setDevMode(true);
   } else {
       chain.setDevMode(false);
   }
   chain.setDeployWaitTime(parseInt(deployWait));
   chain.setInvokeWaitTime(parseInt(invokeWait));
   return chain;
}

exports.getTestChain = getTestChain;
exports.deployMode  = deployMode
exports.keyStore  = keyStore
exports.caCert    = caCert
exports.caAddr    = caAddr
exports.peerAddr0  = peerAddr0
exports.tlsOn     = tlsOn
exports.deployWait   = deployWait
exports.invokeWait   = invokeWait
exports.ciphers   = ciphers