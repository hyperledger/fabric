/**
 * Copyright 2016 IBM All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the 'License');
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an 'AS IS' BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

var path = require('path');
var fs = require('fs-extra');
var os = require('os');
var util = require('util');

var jsrsa = require('jsrsasign');
var KEYUTIL = jsrsa.KEYUTIL;

var hfc = require('fabric-client');
var copService = require('fabric-ca-client/lib/FabricCAClientImpl.js');
var User = require('fabric-client/lib/User.js');
var CryptoSuite = require('fabric-client/lib/impl/CryptoSuite_ECDSA_AES.js');
var KeyStore = require('fabric-client/lib/impl/CryptoKeyStore.js');
var ecdsaKey = require('fabric-client/lib/impl/ecdsa/key.js');

module.exports.CHAINCODE_PATH = 'github.com/example_cc';
module.exports.CHAINCODE_UPGRADE_PATH = 'github.com/example_cc1';
module.exports.CHAINCODE_MARBLES_PATH = 'github.com/marbles_cc';
module.exports.END2END = {
	channel: 'mychannel',
	chaincodeId: 'end2end',
	chaincodeVersion: 'v0'
};

// directory for file based KeyValueStore
module.exports.KVS = '/tmp/hfc-test-kvs';
module.exports.storePathForOrg = function(org) {
	return module.exports.KVS + '_' + org;
};

// temporarily set $GOPATH to the test fixture folder
module.exports.setupChaincodeDeploy = function() {
	process.env.GOPATH = path.join(__dirname, '../fixtures');
};

// specifically set the values to defaults because they may have been overridden when
// running in the overall test bucket ('gulp test')
module.exports.resetDefaults = function() {
	global.hfc.config = undefined;
	require('nconf').reset();
};

module.exports.cleanupDir = function(keyValStorePath) {
	var absPath = path.join(process.cwd(), keyValStorePath);
	var exists = module.exports.existsSync(absPath);
	if (exists) {
		fs.removeSync(absPath);
	}
};

module.exports.getUniqueVersion = function(prefix) {
	if (!prefix) prefix = 'v';
	return prefix + Date.now();
};

// utility function to check if directory or file exists
// uses entire / absolute path from root
module.exports.existsSync = function(absolutePath /*string*/) {
	try  {
		var stat = fs.statSync(absolutePath);
		if (stat.isDirectory() || stat.isFile()) {
			return true;
		} else
			return false;
	}
	catch (e) {
		return false;
	}
};

module.exports.readFile = readFile;

var ORGS;

var	tlsOptions = {
	trustedRoots: [],
	verify: false
};

function getMember(username, password, client, userOrg, svcFile) {
	hfc.addConfigFile(svcFile);
	ORGS = hfc.getConfigSetting('test-network');

	var caUrl = ORGS[userOrg].ca.url;

	console.log('getMember, name: '+username+', client.getUserContext('+username+', true)');

	return client.getUserContext(username, true)
	.then((user) => {
		return new Promise((resolve, reject) => {
			if (user && user.isEnrolled()) {
				console.log('Successfully loaded member from persistence');
				return resolve(user);
			}

			var member = new User(username);
			var cryptoSuite = null;
			if (userOrg) {
				cryptoSuite = client.newCryptoSuite({path: module.exports.storePathForOrg(ORGS[userOrg].name)});
			} else {
				cryptoSuite = client.newCryptoSuite();
			}
			member.setCryptoSuite(cryptoSuite);

			// need to enroll it with CA server
			var cop = new copService(caUrl, tlsOptions, ORGS[userOrg].ca.name, cryptoSuite);

			return cop.enroll({
				enrollmentID: username,
				enrollmentSecret: password
			}).then((enrollment) => {
				console.log('Successfully enrolled user \'' + username + '\'');

				return member.setEnrollment(enrollment.key, enrollment.certificate, ORGS[userOrg].mspid);
			}).then(() => {
				return client.setUserContext(member);
			}).then(() => {
				return resolve(member);
			}).catch((err) => {
				console.log('Failed to enroll and persist user. Error: ' + err.stack ? err.stack : err);
			});
		});
	});
}

function getAdmin(client, userOrg, svcFile) {
        hfc.addConfigFile(svcFile);
        ORGS = hfc.getConfigSetting('test-network');
        var mspPath = ORGS[userOrg].mspPath;
        var keyPath =  ORGS[userOrg].adminPath + '/keystore';
        var keyPEM = Buffer.from(readAllFiles(keyPath)[0]).toString();
        var certPath = ORGS[userOrg].adminPath + '/signcerts';
        var certPEM = readAllFiles(certPath)[0];
        console.log('[getAdmin] keyPath: %s', keyPath);
        console.log('[getAdmin] certPath: %s', certPath);

	if (userOrg) {
		client.newCryptoSuite({path: module.exports.storePathForOrg(ORGS[userOrg].name)});
	}

	return Promise.resolve(client.createUser({
		username: 'peer'+userOrg+'Admin',
		mspid: ORGS[userOrg].mspid,
		cryptoContent: {
			privateKeyPEM: keyPEM.toString(),
			signedCertPEM: certPEM.toString()
		}
	}));
}

function getOrdererAdmin(client, userOrg, svcFile) {
        hfc.addConfigFile(svcFile);
        ORGS = hfc.getConfigSetting('test-network');
        var mspPath = ORGS.orderer.mspPath;
        var keyPath =  ORGS.orderer.adminPath + '/keystore';
        var keyPEM = Buffer.from(readAllFiles(keyPath)[0]).toString();
        var certPath = ORGS.orderer.adminPath + '/signcerts';
        var certPEM = readAllFiles(certPath)[0];
        console.log('[getOrdererAdmin] keyPath: %s', keyPath);
        console.log('[getOrdererAdmin] certPath: %s', certPath);

	return Promise.resolve(client.createUser({
		username: 'ordererAdmin',
		mspid: ORGS.orderer.mspid,
		cryptoContent: {
			privateKeyPEM: keyPEM.toString(),
			signedCertPEM: certPEM.toString()
		}
	}));
}

function readFile(path) {
	return new Promise((resolve, reject) => {
		fs.readFile(path, (err, data) => {
			if (!!err)
				reject(new Error('Failed to read file ' + path + ' due to error: ' + err));
			else
				resolve(data);
		});
	});
}

function readAllFiles(dir) {
	var files = fs.readdirSync(dir);
	var certs = [];
	files.forEach((file_name) => {
		let file_path = path.join(dir,file_name);
		console.log(' looking at file ::'+file_path);
		let data = fs.readFileSync(file_path);
		certs.push(data);
	});
	return certs;
}

module.exports.getOrderAdminSubmitter = function(client, userOrg, svcFile) {
	return getOrdererAdmin(client, userOrg, svcFile);
};

module.exports.getSubmitter = function(username, secret, client, peerOrgAdmin, org, svcFile) {
	if (arguments.length < 2) throw new Error('"client" and "test" are both required parameters');

	var peerAdmin, userOrg;
	if (typeof peerOrgAdmin === 'boolean') {
		peerAdmin = peerOrgAdmin;
	} else {
		peerAdmin = false;
	}

	// if the 3rd argument was skipped
	if (typeof peerOrgAdmin === 'string') {
		userOrg = peerOrgAdmin;
	} else {
		if (typeof org === 'string') {
			userOrg = org;
		} else {
			userOrg = 'org1';
		}
	}

	if (peerAdmin) {
		console.log(' >>>> getting the org admin');
		return getAdmin(client, userOrg, svcFile);
	} else {
		return getMember(username, secret, client, userOrg, svcFile);
	}
};
