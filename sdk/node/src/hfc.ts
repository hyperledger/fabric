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
 * "hfc" stands for "Hyperledger Fabric Client".
 * The Hyperledger Fabric Client SDK provides APIs through which a client can interact with a Hyperledger Fabric blockchain.
 *
 * Terminology:
 * 1) member - an identity for participating in the blockchain.  There are different types of members (users, peers, etc).
 * 2) member services - services related to obtaining and managing members
 * 3) registration - The act of adding a new member identity (with specific privileges) to the system.
 *               This is done by a member with the 'registrar' privilege.  The member is called a registrar.
 *               The registrar specifies the new member privileges when registering the new member.
 * 4) enrollment - Think of this as completing the registration process.  It may be done by the new member with a secret
 *               that it has obtained out-of-band from a registrar, or it may be performed by a middle-man who has
 *               delegated authority to act on behalf of the new member.
 *
 * These APIs have been designed to support two pluggable components.
 * 1) Pluggable key value store which is used to retrieve and store keys associated with a member.
 *    Call Chain.setKeyValStore() to override the default key value store implementation.
 *    For the default implementations, see FileKeyValStore and SqlKeyValStore (TBD).
 * 2) Pluggable member service which is used to register and enroll members.
 *    Call Chain.setMemberService() to override the default implementation.
 *    For the default implementation, see MemberServices.
 *    NOTE: This makes member services pluggable from the client side, but more work is needed to make it compatible on
 *          the server side transaction processing path.
 */

process.env.GRPC_SSL_CIPHER_SUITES = process.env.GRPC_SSL_CIPHER_SUITES
   ? process.env.GRPC_SSL_CIPHER_SUITES
   : 'ECDHE-RSA-AES128-GCM-SHA256:' +
     'ECDHE-RSA-AES128-SHA256:' +
     'ECDHE-RSA-AES256-SHA384:' +
     'ECDHE-RSA-AES256-GCM-SHA384:' +
     'ECDHE-ECDSA-AES128-GCM-SHA256:' +
     'ECDHE-ECDSA-AES128-SHA256:' +
     'ECDHE-ECDSA-AES256-SHA384:' +
     'ECDHE-ECDSA-AES256-GCM-SHA384' ;

var debugModule = require('debug');
var fs = require('fs');
var urlParser = require('url');
var grpc = require('grpc');
var util = require('util');
var jsrsa = require('jsrsasign');
var elliptic = require('elliptic');
var sha3 = require('js-sha3');
var BN = require('bn.js');
var Set = require('es6-set');
var HashTable = require('hashtable');
import * as crypto from "./crypto"
import * as stats from "./stats"
import * as sdk_util from "./sdk_util"
import events = require('events');

let debug = debugModule('hfc');   // 'hfc' stands for 'Hyperledger Fabric Client'
let asn1 = jsrsa.asn1;
var asn1Builder = require('asn1');

let _caProto = grpc.load(__dirname + "/protos/ca.proto").protos;
let _fabricProto = grpc.load(__dirname + "/protos/fabric.proto").protos;
let _chaincodeProto = grpc.load(__dirname + "/protos/chaincode.proto").protos;
let net = require('net');

let DEFAULT_SECURITY_LEVEL = 256;
let DEFAULT_HASH_ALGORITHM = "SHA3";
let CONFIDENTIALITY_1_2_STATE_KD_C6 = 6;

let _chains = {};

/**
 * The KeyValStore interface used for persistent storage.
 */
export interface KeyValStore {

    /**
     * Get the value associated with name.
     * @param name
     * @param cb function(err,value)
     */
    getValue(name:string, cb:GetValueCallback):void;

    /**
     * Set the value associated with name.
     * @param name
     * @param cb function(err)
     */
    setValue(name:string, value:string, cb:ErrorCallback);

}

export interface MemberServices {

    /**
     * Get the security level
     * @returns The security level
     */
    getSecurityLevel():number;

    /**
     * Set the security level
     * @params securityLevel The security level
     */
    setSecurityLevel(securityLevel:number):void;

    /**
     * Get the hash algorithm
     * @returns The security level
     */
    getHashAlgorithm():string;

    /**
     * Set the security level
     * @params securityLevel The security level
     */
    setHashAlgorithm(hashAlgorithm:string):void;

    /**
     * Register the member and return an enrollment secret.
     * @param req Registration request with the following fields: name, role
     * @param registrar The identity of the registar (i.e. who is performing the registration)
     * @param cb Callback of the form: {function(err,enrollmentSecret)}
     */
    register(req:RegistrationRequest, registrar:Member, cb:RegisterCallback):void;

    /**
     * Enroll the member and return an opaque member object
     * @param req Enrollment request with the following fields: name, enrollmentSecret
     * @param cb Callback to report an error if it occurs.
     */
    enroll(req:EnrollmentRequest, cb:EnrollCallback):void;

    /**
     * Get an array of transaction certificates (tcerts).
     * @param req A GetTCertBatchRequest
     * @param cb A GetTCertBatchCallback
     */
    getTCertBatch(req:GetTCertBatchRequest, cb:GetTCertBatchCallback):void;

}

/**
 * A registration request is information required to register a user, peer, or other
 * type of member.
 */
export interface RegistrationRequest {
    // The enrollment ID of the member
    enrollmentID:string;
    // Roles associated with this member.
    // Fabric roles include: 'client', 'peer', 'validator', 'auditor'
    // Default value: ['client']
    roles?:string[];
    // Affiliation for a user
    affiliation:string;
    // The attribute names and values to grant to this member
    attributes?:Attribute[];
    // 'registrar' enables this identity to register other members with types
    // and can delegate the 'delegationRoles' roles
    registrar?:{
        // The allowable roles which this member can register
        roles:string[],
        // The allowable roles which can be registered by members registered by this member
        delegateRoles?:string[]
    };
}

// An attribute consisting of a name and value
export interface Attribute {
   // The attribute name
   name:string;
   // The attribute value
   value:string;
}

// An enrollment request
export interface EnrollmentRequest {
    // The enrollment ID
    enrollmentID:string;
    // The enrollment secret (a one-time password)
    enrollmentSecret:string;
}

// The callback from the Chain.getMember or Chain.getUser methods
export interface GetMemberCallback { (err:Error, member?:Member):void }

// The callback from the Chain.register method
export interface RegisterCallback { (err:Error, enrollmentPassword?:string):void }

// The callback from the Chain.enroll method
export interface EnrollCallback { (err:Error, enrollment?:Enrollment):void }

// The callback from the newBuildOrDeployTransaction
export interface DeployTransactionCallback { (err:Error, deployTx?:Transaction):void }

// The callback from the newInvokeOrQueryTransaction
export interface InvokeOrQueryTransactionCallback { (err:Error, invokeOrQueryTx?:Transaction):void }

// Enrollment metadata
export interface Enrollment {
    key:Buffer;
    cert:string;
    chainKey:string;
}

// GRPCOptions
export interface GRPCOptions {
   pem: string;
   hostnameOverride: string;
}

// A request to get a batch of TCerts
export class GetTCertBatchRequest {
   constructor( public name: string,
                public enrollment: Enrollment,
                public num: number,
                public attrs: string[]) {};
}

// This is the object that is delivered as the result with the "submitted" event
// from a Transaction object for a **deploy** operation.
export class EventDeploySubmitted {
    // The transaction ID of a deploy transaction which was successfully submitted.
    constructor(public uuid:string, public chaincodeID:string){};
}

// This is the object that is delivered as the result with the "complete" event
// from a Transaction object for a **deploy** operation.
// TODO: This class may change once the real event processing is added.
export class EventDeployComplete {
    constructor(public uuid:string, public chaincodeID:string, public result?:any){};
}

// This is the data that is delivered as the result with the "submitted" event
// from a Transaction object for an **invoke** operation.
export class EventInvokeSubmitted {
    // The transaction ID of an invoke transaction which was successfully submitted.
    constructor(public uuid:string){};
}

// This is the object that is delivered as the result with the "complete" event
// from a Transaction object for a **invoke** operation.
// TODO: This class may change once the real event processing is added.
export class EventInvokeComplete {
    constructor(public result?:any){};
}

// This is the object that is delivered as the result with the "complete" event
// from a Transaction object for a **query** operation.
export class EventQueryComplete {
    constructor(public result?:any){};
}

// This is the data that is delivered as the result with the "error" event
// from a Transaction object for any of the following operations:
// **deploy**, **invoke**, or **query**.
export class EventTransactionError {
    public msg:string;
    // The transaction ID of an invoke transaction which was successfully submitted.
    constructor(public error:any){
       if (error && error.msg && isFunction(error.msg.toString)) {
          this.msg = error.msg.toString();
       } else if (isFunction(error.toString)) {
          this.msg = error.toString();
       }
    };
}

// This is the data that is delivered as the result with the 'submitted' event
// from a TransactionContext object.
export interface SubmittedTransactionResponse {
    // The transaction ID of a transaction which was successfully submitted.
    uuid:string;
}

// A callback to the MemberServices.getTCertBatch method
export interface GetTCertBatchCallback { (err:Error, tcerts?:TCert[]):void }

export interface GetTCertCallback { (err:Error, tcert?:TCert):void }

export interface GetTCertCallback { (err:Error, tcert?:TCert):void }

export enum PrivacyLevel {
    Nominal = 0,
    Anonymous = 1
}

// The base Certificate class
export class Certificate {
    constructor(public cert:Buffer,
                public privateKey:any,
                /** Denoting if the Certificate is anonymous or carrying its owner's identity. */
                public privLevel?:PrivacyLevel) {  // TODO: privLevel not currently used?
    }

    encode():Buffer {
        return this.cert;
    }
}

/**
 * Enrollment certificate.
 */
class ECert extends Certificate {

    constructor(public cert:Buffer,
                public privateKey:any) {
        super(cert, privateKey, PrivacyLevel.Nominal);
    }

}

/**
 * Transaction certificate.
 */
export class TCert extends Certificate {
    constructor(public publicKey:any,
                public privateKey:any) {
        super(publicKey, privateKey, PrivacyLevel.Anonymous);
    }
}

/**
 * A base transaction request common for DeployRequest, InvokeRequest, and QueryRequest.
 */
export interface TransactionRequest {
    // The chaincode ID as provided by the 'submitted' event emitted by a TransactionContext
    chaincodeID:string;
    // The name of the function to invoke
    fcn:string;
    // The arguments to pass to the chaincode invocation
    args:string[];
    // Specify whether the transaction is confidential or not.  The default value is false.
    confidential?:boolean,
    // Optionally provide a user certificate which can be used by chaincode to perform access control
    userCert?:Certificate;
    // Optionally provide additional metadata
    metadata?:Buffer
}

/**
 * Deploy request.
 */
export interface DeployRequest extends TransactionRequest {
    // The local path containing the chaincode to deploy in network mode.
    chaincodePath:string;
    // The name identifier for the chaincode to deploy in development mode.
    chaincodeName:string;
    // The directory on the server side, where the certificate.pem will be copied
    certificatePath:string;
}

/**
 * Invoke or query request.
 */
export interface InvokeOrQueryRequest extends TransactionRequest {
    // Optionally pass a list of attributes which can be used by chaincode to perform access control
    attrs?:string[];
}

/**
 * Query request.
 */
export interface QueryRequest extends InvokeOrQueryRequest {
}

/**
 * Invoke request.
 */
export interface InvokeRequest extends InvokeOrQueryRequest {
}

/**
 * A transaction.
 */
export interface TransactionProtobuf {
    getType():string;
    setCert(cert:Buffer):void;
    setSignature(sig:Buffer):void;
    setConfidentialityLevel(value:number): void;
    getConfidentialityLevel(): number;
    setConfidentialityProtocolVersion(version:string):void;
    setNonce(nonce:Buffer):void;
    setToValidators(Buffer):void;
    getTxid():string;
    getChaincodeID():{buffer: Buffer};
    setChaincodeID(buffer:Buffer):void;
    getMetadata():{buffer: Buffer};
    setMetadata(buffer:Buffer):void;
    getPayload():{buffer: Buffer};
    setPayload(buffer:Buffer):void;
    toBuffer():Buffer;
}

export class Transaction {
    constructor(public pb:TransactionProtobuf, public chaincodeID:string){};
}

/**
 * Common error callback.
 */
export interface ErrorCallback { (err:Error):void }

/**
 * A callback for the KeyValStore.getValue method.
 */
export interface GetValueCallback { (err:Error, value?:string):void }

/**
 * The class representing a chain with which the client SDK interacts.
 */
export class Chain {

    // Name of the chain is only meaningful to the client
    private name:string;

    // The peers on this chain to which the client can connect
    private peers:Peer[] = [];

    // Security enabled flag
    private securityEnabled:boolean = true;

    // A member cache associated with this chain
    // TODO: Make an LRU to limit size of member cache
    private members:{[name:string]:Member} = {};

    // The number of tcerts to get in each batch
    private tcertBatchSize:number = 200;

    // The registrar (if any) that registers & enrolls new members/users
    private registrar:Member;

    // The member services used for this chain
    private memberServices:MemberServices;

    // The eventHub service used for this chain
    private eventHub:EventHub;

    // The key-val store used for this chain
    private keyValStore:KeyValStore;

    // Is in dev mode or network mode
    private devMode:boolean = false;

    // If in prefetch mode, we prefetch tcerts from member services to help performance
    private preFetchMode:boolean = true;

    // Temporary variables to control how long to wait for deploy and invoke to complete before
    // emitting events.  This will be removed when the SDK is able to receive events from the
    private deployWaitTime:number = 30;
    private invokeWaitTime:number = 5;

    // The crypto primitives object
    cryptoPrimitives:crypto.Crypto;

    constructor(name:string) {
        this.name = name;
        this.eventHub = new EventHub();
    }

    /**
     * Get the chain name.
     * @returns The name of the chain.
     */
    getName():string {
        return this.name;
    }

    /**
     * Add a peer given an endpoint specification.
     * @param url The URL of the peer.
     * @param opts Optional GRPC options.
     * @returns {Peer} Returns a new peer.
     */
    addPeer(url:string, opts?:GRPCOptions):Peer {

        //check to see if the peer is already part of the chain
        this.peers.forEach(function(peer){
            if (peer.getUrl()===url)
            {
                var error = new Error();
                error.name = "DuplicatePeer";
                error.message = "Peer with URL " + url + " is already a member of the chain";
                throw error;
            }
        })

        let peer = new Peer(url, this, opts);
        this.peers.push(peer);
        return peer;
    };

    /**
     * Get the peers for this chain.
     */
    getPeers():Peer[] {
        return this.peers;
    }

    /**
     * Get the member whose credentials are used to register and enroll other users, or undefined if not set.
     * @param {Member} The member whose credentials are used to perform registration, or undefined if not set.
     */
    getRegistrar():Member {
        return this.registrar;
    }

    /**
     * Set the member whose credentials are used to register and enroll other users.
     * @param {Member} registrar The member whose credentials are used to perform registration.
     */
    setRegistrar(registrar:Member):void {
        this.registrar = registrar;
    }

    /**
     * Set the member services URL
     * @param {string} url Member services URL of the form: "grpc://host:port" or "grpcs://host:port"
     * @param {GRPCOptions} opts optional GRPC options
     */
    setMemberServicesUrl(url:string, opts?:GRPCOptions):void {
        this.setMemberServices(newMemberServices(url,opts));
    }

    /**
     * Get the member service associated this chain.
     * @returns {MemberService} Return the current member service, or undefined if not set.
     */
    getMemberServices():MemberServices {
        return this.memberServices;
    };

    /**
     * Set the member service associated this chain.  This allows the default implementation of member service to be overridden.
     */
    setMemberServices(memberServices:MemberServices):void {
        this.memberServices = memberServices;
        if (memberServices instanceof MemberServicesImpl) {
           this.cryptoPrimitives = (<MemberServicesImpl>memberServices).getCrypto();
        }
    };

    /**
     * Get the eventHub service associated this chain.
     * @returns {eventHub} Return the current eventHub service, or undefined if not set.
     */
    getEventHub():EventHub{
        return this.eventHub;
    };

    /**
     * Set and connect to the peer to be used as the event source.
     */
    eventHubConnect(peerUrl: string, opts?:GRPCOptions):void {
        this.eventHub.setPeerAddr(peerUrl, opts);
        this.eventHub.connect();
    };

    /**
     * Set and connect to the peer to be used as the event source.
     */
    eventHubDisconnect():void {
        this.eventHub.disconnect();
    };

    /**
     * Determine if security is enabled.
     */
    isSecurityEnabled():boolean {
        return this.memberServices !== undefined;
    }

    /**
     * Determine if pre-fetch mode is enabled to prefetch tcerts.
     */
    isPreFetchMode():boolean {
        return this.preFetchMode;
    }

    /**
     * Set prefetch mode to true or false.
     */
    setPreFetchMode(preFetchMode:boolean):void {
        this.preFetchMode = preFetchMode;
    }

    /**
     * Enable or disable ECDSA mode for GRPC.
     */
    setECDSAModeForGRPC(enabled:boolean):void {
       // TODO: Handle multiple chains in different modes appropriately; this will not currently work
       // since it is based env variables.
       if (enabled) {
          // Instruct boringssl to use ECC for tls.
          process.env.GRPC_SSL_CIPHER_SUITES = 'HIGH+ECDSA';
       } else {
          delete process.env.GRPC_SSL_CIPHER_SUITES;
       }
    }

    /**
     * Determine if dev mode is enabled.
     */
    isDevMode():boolean {
        return this.devMode
    }

    /**
     * Set dev mode to true or false.
     */
    setDevMode(devMode:boolean):void {
        this.devMode = devMode;
    }

    /**
     * Get the deploy wait time in seconds.
     */
    getDeployWaitTime():number {
        return this.deployWaitTime;
    }

    /**
     * Set the deploy wait time in seconds.
     * Node.js will automatically enforce a
     * minimum and maximum wait time.  If the
     * number of seconds is larger than 2147483,
     * less than 1, or not a number,
     * the actual wait time used will be 1 ms.
     * @param secs
     */
    setDeployWaitTime(secs:number):void {
        this.deployWaitTime = secs;
    }

    /**
     * Get the invoke wait time in seconds.
     */
    getInvokeWaitTime():number {
        return this.invokeWaitTime;
    }

    /**
     * Set the invoke wait time in seconds.
     * @param secs
     */
    setInvokeWaitTime(secs:number):void {
        this.invokeWaitTime = secs;
    }

    /**
     * Get the key val store implementation (if any) that is currently associated with this chain.
     * @returns {KeyValStore} Return the current KeyValStore associated with this chain, or undefined if not set.
     */
    getKeyValStore():KeyValStore {
        return this.keyValStore;
    }

    /**
     * Set the key value store implementation.
     */
    setKeyValStore(keyValStore:KeyValStore):void {
        this.keyValStore = keyValStore;
    }

    /**
     * Get the tcert batch size.
     */
    getTCertBatchSize():number {
        return this.tcertBatchSize;
    }

    /**
     * Set the tcert batch size.
     */
    setTCertBatchSize(batchSize:number) {
        this.tcertBatchSize = batchSize;
    }

    /**
     * Get the user member named 'name' or create
     * a new member if the member does not exist.
     * @param cb Callback of form "function(err,Member)"
     */
    getMember(name:string, cb:GetMemberCallback):void {
        let self = this;
        cb = cb || nullCB;
        if (!self.keyValStore) return cb(Error("No key value store was found.  You must first call Chain.configureKeyValStore or Chain.setKeyValStore"));
        if (!self.memberServices) return cb(Error("No member services was found.  You must first call Chain.configureMemberServices or Chain.setMemberServices"));
        self.getMemberHelper(name, function (err, member) {
            if (err) return cb(err);
            cb(null, member);
        });
    }

    /**
     * Get a user.
     * A user is a specific type of member.
     * Another type of member is a peer.
     */
    getUser(name:string, cb:GetMemberCallback):void {
        return this.getMember(name, cb);
    }

    // Try to get the member from cache.
    // If not found, create a new one.
    // If member is found in the key value store,
    //    restore the state to the new member, store in cache and return the member.
    // If there are no errors and member is not found in the key value store,
    //    return the new member.
    private getMemberHelper(name:string, cb:GetMemberCallback) {
        let self = this;
        // Try to get the member state from the cache
        let member = self.members[name];
        if (member) return cb(null, member);
        // Create the member and try to restore it's state from the key value store (if found).
        member = new Member(name, self);
        member.restoreState(function (err) {
            if (err) return cb(err);
            self.members[name]=member;
            cb(null, member);
        });
    }

    /**
     * Register a user or other member type with the chain.
     * @param registrationRequest Registration information.
     * @param cb Callback with registration results
     */
    register(registrationRequest:RegistrationRequest, cb:RegisterCallback):void {
        let self = this;
        self.getMember(registrationRequest.enrollmentID, function(err, member) {
            if (err) return cb(err);
            member.register(registrationRequest,cb);
        });
    }

    /**
     * Enroll a user or other identity which has already been registered.
     * If the user has already been enrolled, this will still succeed.
     * @param name The name of the user or other member to enroll.
     * @param secret The secret of the user or other member to enroll.
     * @param cb The callback to return the user or other member.
     */
    enroll(name: string, secret: string, cb: GetMemberCallback) {
        let self = this;
        self.getMember(name, function(err, member) {
            if (err) return cb(err);
            member.enroll(secret,function(err) {
                if (err) return cb(err);
                return cb(null,member);
            });
        });
    }

    /**
     * Register and enroll a user or other member type.
     * This assumes that a registrar with sufficient privileges has been set.
     * @param registrationRequest Registration information.
     * @params
     */
    registerAndEnroll(registrationRequest: RegistrationRequest, cb: GetMemberCallback) {
        let self = this;
        self.getMember(registrationRequest.enrollmentID, function(err, member) {
           if (err) return cb(err);
           if (member.isEnrolled()) {
               debug("already enrolled");
               return cb(null,member);
           }
           member.registerAndEnroll(registrationRequest, function (err) {
              if (err) return cb(err);
              return cb(null,member);
           });
        });
    }

    /**
     * Send a transaction to a peer.
     * @param tx A transaction
     * @param eventEmitter An event emitter
     */
    sendTransaction(tx:Transaction, eventEmitter:events.EventEmitter) {
        if (this.peers.length === 0) {
            return eventEmitter.emit('error', new EventTransactionError(util.format("chain %s has no peers", this.getName())));
        }
        let peers = this.peers;
        let trySendTransaction = (pidx) => {
            if( pidx >= peers.length ) {
                eventEmitter.emit('error', new EventTransactionError("None of "+peers.length+" peers reponding"));
                return;
            }
            let p = urlParser.parse(peers[pidx].getUrl());
            let client = new net.Socket();
            let tryNext = () => {
                debug("Skipping unresponsive peer "+peers[pidx].getUrl());
                client.destroy();
                trySendTransaction(pidx+1);
            }
            client.on('timeout', tryNext);
            client.on('error', tryNext);
            client.connect(p.port, p.hostname, () => {
            if( pidx > 0  &&  peers === this.peers )
                this.peers = peers.slice(pidx).concat(peers.slice(0,pidx));
                client.destroy();
            peers[pidx].sendTransaction(tx, eventEmitter);
        });
    }
    trySendTransaction(0);
    }
}

/**
 * A member is an entity that transacts on a chain.
 * Types of members include end users, peers, etc.
 */
export class Member {

    private chain:Chain;
    private name:string;
    private roles:string[];
    private affiliation:string;
    private enrollmentSecret:string;
    private enrollment:any;
    private memberServices:MemberServices;
    private keyValStore:KeyValStore;
    private keyValStoreName:string;
    private tcertGetterMap: {[s:string]:TCertGetter} = {};
    private tcertBatchSize:number;

    /**
     * Constructor for a member.
     * @param cfg {string | RegistrationRequest} The member name or registration request.
     * @returns {Member} A member who is neither registered nor enrolled.
     */
    constructor(cfg:any, chain:Chain) {
        if (util.isString(cfg)) {
            this.name = cfg;
        } else if (util.isObject(cfg)) {
            let req = cfg;
            this.name = req.enrollmentID || req.name;
            this.roles = req.roles || ['fabric.user'];
            this.affiliation = req.affiliation;
        }
        this.chain = chain;
        this.memberServices = chain.getMemberServices();
        this.keyValStore = chain.getKeyValStore();
        this.keyValStoreName = toKeyValStoreName(this.name);
        this.tcertBatchSize = chain.getTCertBatchSize();
    }

    /**
     * Get the member name.
     * @returns {string} The member name.
     */
    getName():string {
        return this.name;
    }

    /**
     * Get the chain.
     * @returns {Chain} The chain.
     */
    getChain():Chain {
        return this.chain;
    };

    /**
     * Get the member services.
     * @returns {MemberServices} The member services.
     */
    getMemberServices():MemberServices {
       return this.memberServices;
    };

    /**
     * Get the roles.
     * @returns {string[]} The roles.
     */
    getRoles():string[] {
        return this.roles;
    };

    /**
     * Set the roles.
     * @param roles {string[]} The roles.
     */
    setRoles(roles:string[]):void {
        this.roles = roles;
    };

    /**
     * Get the affiliation.
     * @returns {string} The affiliation.
     */
    getAffiliation():string {
        return this.affiliation;
    };

    /**
     * Set the affiliation.
     * @param affiliation The affiliation.
     */
    setAffiliation(affiliation:string):void {
        this.affiliation = affiliation;
    };

    /**
     * Get the transaction certificate (tcert) batch size, which is the number of tcerts retrieved
     * from member services each time (i.e. in a single batch).
     * @returns The tcert batch size.
     */
    getTCertBatchSize():number {
        if (this.tcertBatchSize === undefined) {
            return this.chain.getTCertBatchSize();
        } else {
            return this.tcertBatchSize;
        }
    }

    /**
     * Set the transaction certificate (tcert) batch size.
     * @param batchSize
     */
    setTCertBatchSize(batchSize:number):void {
        this.tcertBatchSize = batchSize;
    }

    /**
     * Get the enrollment info.
     * @returns {Enrollment} The enrollment.
     */
    getEnrollment():any {
        return this.enrollment;
    };

    /**
     * Determine if this name has been registered.
     * @returns {boolean} True if registered; otherwise, false.
     */
    isRegistered():boolean {
        return this.enrollmentSecret !== undefined;
    }

    /**
     * Determine if this name has been enrolled.
     * @returns {boolean} True if enrolled; otherwise, false.
     */
    isEnrolled():boolean {
        return this.enrollment !== undefined;
    }

    /**
     * Register the member.
     * @param cb Callback of the form: {function(err,enrollmentSecret)}
     */
    register(registrationRequest:RegistrationRequest, cb:RegisterCallback):void {
        let self = this;
        cb = cb || nullCB;
        if (registrationRequest.enrollmentID !== self.getName()) {
            return cb(Error("registration enrollment ID and member name are not equal"));
        }
        let enrollmentSecret = this.enrollmentSecret;
        if (enrollmentSecret) {
            debug("previously registered, enrollmentSecret=%s", enrollmentSecret);
            return cb(null, enrollmentSecret);
        }
        self.memberServices.register(registrationRequest, self.chain.getRegistrar(), function (err, enrollmentSecret) {
            debug("memberServices.register err=%s, secret=%s", err, enrollmentSecret);
            if (err) return cb(err);
            self.enrollmentSecret = enrollmentSecret;
            self.saveState(function (err) {
                if (err) return cb(err);
                cb(null, enrollmentSecret);
            });
        });
    }

    /**
     * Enroll the member and return the enrollment results.
     * @param enrollmentSecret The password or enrollment secret as returned by register.
     * @param cb Callback to report an error if it occurs
     */
    enroll(enrollmentSecret:string, cb:EnrollCallback):void {
        let self = this;
        cb = cb || nullCB;
        let enrollment = self.enrollment;
        if (enrollment) {
            debug("Previously enrolled, [enrollment=%j]", enrollment);
            return cb(null,enrollment);
        }
        let req = {enrollmentID: self.getName(), enrollmentSecret: enrollmentSecret};
        debug("Enrolling [req=%j]", req);
        self.memberServices.enroll(req, function (err:Error, enrollment:Enrollment) {
            debug("[memberServices.enroll] err=%s, enrollment=%j", err, enrollment);
            if (err) return cb(err);
            self.enrollment = enrollment;
            // Generate queryStateKey
            self.enrollment.queryStateKey = self.chain.cryptoPrimitives.generateNonce();

            // Save state
            self.saveState(function (err) {
                if (err) return cb(err);

                // Unmarshall chain key
                // TODO: during restore, unmarshall enrollment.chainKey
                debug("[memberServices.enroll] Unmarshalling chainKey");
                var ecdsaChainKey = self.chain.cryptoPrimitives.ecdsaPEMToPublicKey(self.enrollment.chainKey);
                self.enrollment.enrollChainKey = ecdsaChainKey;

                cb(null, enrollment);
            });
        });
    }

    /**
     * Perform both registration and enrollment.
     * @param cb Callback of the form: {function(err,{key,cert,chainKey})}
     */
    registerAndEnroll(registrationRequest:RegistrationRequest, cb:ErrorCallback):void {
        let self = this;
        cb = cb || nullCB;
        let enrollment = self.enrollment;
        if (enrollment) {
            debug("previously enrolled, enrollment=%j", enrollment);
            return cb(null);
        }
        self.register(registrationRequest, function (err, enrollmentSecret) {
            if (err) return cb(err);
            self.enroll(enrollmentSecret, function (err, enrollment) {
                if (err) return cb(err);
                cb(null);
            });
        });
    }

    /**
     * Issue a deploy request on behalf of this member.
     * @param deployRequest {Object}
     * @returns {TransactionContext} Emits 'submitted', 'complete', and 'error' events.
     */
    deploy(deployRequest:DeployRequest):TransactionContext {
        debug("Member.deploy");

        let tx = this.newTransactionContext();
        tx.deploy(deployRequest);
        return tx;
    }

    /**
     * Issue a invoke request on behalf of this member.
     * @param invokeRequest {Object}
     * @returns {TransactionContext} Emits 'submitted', 'complete', and 'error' events.
     */
    invoke(invokeRequest:InvokeRequest):TransactionContext {
        debug("Member.invoke");

        let tx = this.newTransactionContext();
        tx.invoke(invokeRequest);
        return tx;
    }

    /**
     * Issue a query request on behalf of this member.
     * @param queryRequest {Object}
     * @returns {TransactionContext} Emits 'submitted', 'complete', and 'error' events.
     */
    query(queryRequest:QueryRequest):TransactionContext {
        debug("Member.query");

        var tx = this.newTransactionContext();
        tx.query(queryRequest);
        return tx;
    }

    /**
     * Create a transaction context with which to issue build, deploy, invoke, or query transactions.
     * Only call this if you want to use the same tcert for multiple transactions.
     * @param {Object} tcert A transaction certificate from member services.  This is optional.
     * @returns A transaction context.
     */
    newTransactionContext(tcert?:TCert):TransactionContext {
        return new TransactionContext(this, tcert);
    }

    /**
     * Get a user certificate.
     * @param attrs The names of attributes to include in the user certificate.
     * @param cb A GetTCertCallback
     */
    getUserCert(attrs:string[], cb:GetTCertCallback):void {
        this.getNextTCert(attrs,cb);
    }

    /**
   * Get the next available transaction certificate with the appropriate attributes.
   * @param cb
   */
   getNextTCert(attrs:string[], cb:GetTCertCallback):void {
        let self = this;
        if (!self.isEnrolled()) {
            return cb(Error(util.format("user '%s' is not enrolled",self.getName())));
        }
        let key = getAttrsKey(attrs);
        debug("Member.getNextTCert: key=%s",key);
        let tcertGetter = self.tcertGetterMap[key];
        if (!tcertGetter) {
            debug("Member.getNextTCert: key=%s, creating new getter",key);
            tcertGetter = new TCertGetter(self,attrs,key);
            self.tcertGetterMap[key] = tcertGetter;
        }
        return tcertGetter.getNextTCert(cb);
   }

   /**
    * Save the state of this member to the key value store.
    * @param cb Callback of the form: {function(err}
    */
   saveState(cb:ErrorCallback):void {
      let self = this;
      self.keyValStore.setValue(self.keyValStoreName, self.toString(), cb);
   }

   /**
    * Restore the state of this member from the key value store (if found).  If not found, do nothing.
    * @param cb Callback of the form: function(err}
    */
   restoreState(cb:ErrorCallback):void {
      var self = this;
      self.keyValStore.getValue(self.keyValStoreName, function (err, memberStr) {
         if (err) return cb(err);
         // debug("restoreState: name=%s, memberStr=%s", self.getName(), memberStr);
         if (memberStr) {
             // The member was found in the key value store, so restore the state.
             self.fromString(memberStr);
         }
         cb(null);
      });
   }

    /**
     * Get the current state of this member as a string
     * @return {string} The state of this member as a string
     */
    fromString(str:string):void {
        let state = JSON.parse(str);
        if (state.name !== this.getName()) throw Error("name mismatch: '" + state.name + "' does not equal '" + this.getName() + "'");
        this.name = state.name;
        this.roles = state.roles;
        this.affiliation = state.affiliation;
        this.enrollmentSecret = state.enrollmentSecret;
        this.enrollment = state.enrollment;
    }

    /**
     * Save the current state of this member as a string
     * @return {string} The state of this member as a string
     */
    toString():string {
        let self = this;
        let state = {
            name: self.name,
            roles: self.roles,
            affiliation: self.affiliation,
            enrollmentSecret: self.enrollmentSecret,
            enrollment: self.enrollment
        };
        return JSON.stringify(state);
    }

}

/**
 * A transaction context emits events 'submitted', 'complete', and 'error'.
 * Each transaction context uses exactly one tcert.
 */
export class TransactionContext extends events.EventEmitter {

    private member:Member;
    private chain:Chain;
    private memberServices:MemberServices;
    private nonce: any;
    private binding: any;
    private tcert:TCert;
    private attrs:string[];
    private complete:boolean;
    private timeoutId:any;
    private waitTime:number;
    private cevent:any;

    constructor(member:Member, tcert:TCert) {
        super();
        this.member = member;
        this.chain = member.getChain();
        this.memberServices = this.chain.getMemberServices();
        this.tcert = tcert;
        this.nonce = this.chain.cryptoPrimitives.generateNonce();
        this.complete = false;
        this.timeoutId = null;
    }

    /**
     * Get the member with which this transaction context is associated.
     * @returns The member
     */
    getMember():Member {
        return this.member;
    }

    /**
     * Get the chain with which this transaction context is associated.
     * @returns The chain
     */
    getChain():Chain {
        return this.chain;
    };

    /**
     * Get the member services, or undefined if security is not enabled.
     * @returns The member services
     */
    getMemberServices():MemberServices {
        return this.memberServices;
    };

    /**
     * Emit a specific event provided an event listener is already registered.
     */
    emitMyEvent(name:string, event:any) {
       var self = this;

       setTimeout(function() {
         // Check if an event listener has been registered for the event
         let listeners = self.listeners(name);

         // If an event listener has been registered, emit the event
         if (listeners && listeners.length > 0) {
            self.emit(name, event);
         }
       }, 0);
    }

    /**
     * Issue a deploy transaction.
     * @param deployRequest {Object} A deploy request of the form: { chaincodeID, payload, metadata, uuid, timestamp, confidentiality: { level, version, nonce }
   */
    deploy(deployRequest:DeployRequest):TransactionContext {
        debug("TransactionContext.deploy");
        debug("Received deploy request: %j", deployRequest);

        let self = this;

        // Get a TCert to use in the deployment transaction
        self.getMyTCert(function (err) {
            if (err) {
                debug('Failed getting a new TCert [%s]', err);
                self.emitMyEvent('error', new EventTransactionError(err));

                return self;
            }

            debug("Got a TCert successfully, continue...");

            self.newBuildOrDeployTransaction(deployRequest, false, function(err, deployTx) {
              if (err) {
                debug("Error in newBuildOrDeployTransaction [%s]", err);
                self.emitMyEvent('error', new EventTransactionError(err));

                return self;
              }

              debug("Calling TransactionContext.execute");

              return self.execute(deployTx);
            });
        });
        return self;
    }

    /**
     * Issue an invoke transaction.
     * @param invokeRequest {Object} An invoke request of the form: XXX
     */
    invoke(invokeRequest:InvokeRequest):TransactionContext {
        debug("TransactionContext.invoke");
        debug("Received invoke request: %j", invokeRequest);

        let self = this;

        // Get a TCert to use in the invoke transaction
        self.setAttrs(invokeRequest.attrs);
        self.getMyTCert(function (err, tcert) {
            if (err) {
                debug('Failed getting a new TCert [%s]', err);
                self.emitMyEvent('error', new EventTransactionError(err));

                return self;
            }

            debug("Got a TCert successfully, continue...");

            self.newInvokeOrQueryTransaction(invokeRequest, true, function(err, invokeTx) {
              if (err) {
                debug("Error in newInvokeOrQueryTransaction [%s]", err);
                self.emitMyEvent('error', new EventTransactionError(err));

                return self;
              }

              debug("Calling TransactionContext.execute");

              return self.execute(invokeTx);
            });
        });
        return self;
    }

    /**
     * Issue an query transaction.
     * @param queryRequest {Object} A query request of the form: XXX
     */
    query(queryRequest:QueryRequest):TransactionContext {
      debug("TransactionContext.query");
      debug("Received query request: %j", queryRequest);

      let self = this;

      // Get a TCert to use in the query transaction
      self.setAttrs(queryRequest.attrs);
      self.getMyTCert(function (err, tcert) {
          if (err) {
              debug('Failed getting a new TCert [%s]', err);
              self.emitMyEvent('error', new EventTransactionError(err));

              return self;
          }

          debug("Got a TCert successfully, continue...");

          self.newInvokeOrQueryTransaction(queryRequest, false, function(err, queryTx) {
            if (err) {
              debug("Error in newInvokeOrQueryTransaction [%s]", err);
              self.emitMyEvent('error', new EventTransactionError(err));

              return self;
            }

            debug("Calling TransactionContext.execute");

            return self.execute(queryTx);
          });
        });
      return self;
    }

   /**
    * Get the attribute names associated
    */
   getAttrs(): string[] {
       return this.attrs;
   }

   /**
    * Set the attributes for this transaction context.
    */
   setAttrs(attrs:string[]): void {
       this.attrs = attrs;
   }

    /**
     * Execute a transaction
     * @param tx {Transaction} The transaction.
     */
    private execute(tx:Transaction):TransactionContext {
        debug('Executing transaction');

        let self = this;
        // Get the TCert
        self.getMyTCert(function (err, tcert) {
            if (err) {
                debug('Failed getting a new TCert [%s]', err);
                return self.emit('error', new EventTransactionError(err));
            }

            if (tcert) {
                // Set nonce
                tx.pb.setNonce(self.nonce);

                // Process confidentiality
                debug('Process Confidentiality...');

                self.processConfidentiality(tx);

                debug('Sign transaction...');

                // Add the tcert
                tx.pb.setCert(tcert.publicKey);
                // sign the transaction bytes
                let txBytes = tx.pb.toBuffer();
                let derSignature = self.chain.cryptoPrimitives.ecdsaSign(tcert.privateKey.getPrivate('hex'), txBytes).toDER();
                // debug('signature: ', derSignature);
                tx.pb.setSignature(new Buffer(derSignature));

                debug('Send transaction...');
                debug('Confidentiality: ', tx.pb.getConfidentialityLevel());

                if (tx.pb.getConfidentialityLevel() == _fabricProto.ConfidentialityLevel.CONFIDENTIAL &&
                        tx.pb.getType() == _fabricProto.Transaction.Type.CHAINCODE_QUERY) {
                    // Need to send a different event emitter so we can catch the response
                    // and perform decryption before sending the real complete response
                    // to the caller
                    var emitter = new events.EventEmitter();
                    emitter.on("complete", function (event:EventQueryComplete) {
                        debug("Encrypted: [%j]", event);
                        event.result = self.decryptResult(event.result);
                        debug("Decrypted: [%j]", event);
                        self.emit("complete", event);
                    });
                    emitter.on("error", function (event:EventTransactionError) {
                        self.emit("error", event);
                    });
                    self.getChain().sendTransaction(tx, emitter);
                } else {
                     let txType = tx.pb.getType();
                     let uuid = tx.pb.getTxid();
                     let eh = self.getChain().getEventHub();
                     // async deploy and invokes need to maintain
                     // tx context(completion status(self.complete))
                     if ( txType == _fabricProto.Transaction.Type.CHAINCODE_DEPLOY) {
                         self.cevent = new EventDeployComplete(uuid, tx.chaincodeID);
                         self.waitTime = self.getChain().getDeployWaitTime();
                     } else if ( txType == _fabricProto.Transaction.Type.CHAINCODE_INVOKE) {
                         self.cevent = new EventInvokeComplete("Tx "+uuid+" complete");
                         self.waitTime = self.getChain().getInvokeWaitTime();
                     }
                     eh.registerTxEvent(uuid, function (uuid) {
                         self.complete = true;
                         if (self.timeoutId) {
                             clearTimeout(self.timeoutId);
             }
                         eh.unregisterTxEvent(uuid);
                         self.emit("complete", self.cevent);
                     });
                     self.getChain().sendTransaction(tx, self);
                     // sync query can be skipped as response
                     // is processed and event generated in sendTransaction
                     // no timeout processing is necessary
                     if ( txType != _fabricProto.Transaction.Type.CHAINCODE_QUERY) {
                         debug("waiting %d seconds before emitting complete event", self.waitTime);
                         self.timeoutId = setTimeout(function() {
                             debug("timeout uuid=", uuid);
                             if(!self.complete)
                                 // emit error if eventhub connect otherwise
                                 // emit a complete event as done previously
                                 if(eh.isconnected())
                                     self.emit("error","timed out waiting for transaction to complete");
                                 else
                                     self.emit("complete",self.cevent);
                             else
                                 eh.unregisterTxEvent(uuid);
                             },
                                self.waitTime * 1000
                         );
             }
                }
            } else {
                debug('Missing TCert...');
                return self.emit('error', new EventTransactionError('Missing TCert.'));
            }

        });
        return self;
    }

    private getMyTCert(cb:GetTCertCallback): void {
        let self = this;
        if (!self.getChain().isSecurityEnabled() || self.tcert) {
            debug('[TransactionContext] TCert already cached.');
            return cb(null, self.tcert);
        }
        debug('[TransactionContext] No TCert cached. Retrieving one.');
        this.member.getNextTCert(self.attrs, function (err, tcert) {
            if (err) return cb(err);
            self.tcert = tcert;
            return cb(null, tcert);
        });
    }

    private processConfidentiality(transaction:Transaction) {
        // is confidentiality required?
        if (transaction.pb.getConfidentialityLevel() != _fabricProto.ConfidentialityLevel.CONFIDENTIAL) {
            // No confidentiality is required
            return
        }

        debug('Process Confidentiality ...');
        var self = this;

        // Set confidentiality level and protocol version
        transaction.pb.setConfidentialityProtocolVersion('1.2');

        // Generate transaction key. Common to all type of transactions
        var txKey = self.chain.cryptoPrimitives.eciesKeyGen();

        debug('txkey [%j]', txKey.pubKeyObj.pubKeyHex);
        debug('txKey.prvKeyObj %j', txKey.prvKeyObj.toString());

        var privBytes = self.chain.cryptoPrimitives.ecdsaPrivateKeyToASN1(txKey.prvKeyObj.prvKeyHex);
        debug('privBytes %s', privBytes.toString());

        // Generate stateKey. Transaction type dependent step.
        var stateKey;
        if (transaction.pb.getType() == _fabricProto.Transaction.Type.CHAINCODE_DEPLOY) {
            // The request is for a deploy
            stateKey = new Buffer(self.chain.cryptoPrimitives.aesKeyGen());
        } else if (transaction.pb.getType() == _fabricProto.Transaction.Type.CHAINCODE_INVOKE ) {
            // The request is for an execute
            // Empty state key
            stateKey = new Buffer([]);
        } else {
            // The request is for a query
            debug('Generate state key...');
            stateKey = new Buffer(self.chain.cryptoPrimitives.hmacAESTruncated(
                self.member.getEnrollment().queryStateKey,
                [CONFIDENTIALITY_1_2_STATE_KD_C6].concat(self.nonce)
            ));
        }

        // Prepare ciphertexts

        // Encrypts message to validators using self.enrollChainKey
        var chainCodeValidatorMessage1_2 = new asn1Builder.Ber.Writer();
        chainCodeValidatorMessage1_2.startSequence();
        chainCodeValidatorMessage1_2.writeBuffer(privBytes, 4);
        if (stateKey.length != 0) {
            debug('STATE KEY %j', stateKey);
            chainCodeValidatorMessage1_2.writeBuffer(stateKey, 4);
        } else {
            chainCodeValidatorMessage1_2.writeByte(4);
            chainCodeValidatorMessage1_2.writeLength(0);
        }
        chainCodeValidatorMessage1_2.endSequence();
        debug(chainCodeValidatorMessage1_2.buffer);

        debug('Using chain key [%j]', self.member.getEnrollment().chainKey);
        var ecdsaChainKey = self.chain.cryptoPrimitives.ecdsaPEMToPublicKey(
            self.member.getEnrollment().chainKey
        );

        let encMsgToValidators = self.chain.cryptoPrimitives.eciesEncryptECDSA(
            ecdsaChainKey,
            chainCodeValidatorMessage1_2.buffer
        );
        transaction.pb.setToValidators(encMsgToValidators);

        // Encrypts chaincodeID using txKey
        // debug('CHAINCODE ID %j', transaction.chaincodeID);

        let encryptedChaincodeID = self.chain.cryptoPrimitives.eciesEncrypt(
            txKey.pubKeyObj,
            transaction.pb.getChaincodeID().buffer
        );
        transaction.pb.setChaincodeID(encryptedChaincodeID);

        // Encrypts payload using txKey
        // debug('PAYLOAD ID %j', transaction.payload);
        let encryptedPayload = self.chain.cryptoPrimitives.eciesEncrypt(
            txKey.pubKeyObj,
            transaction.pb.getPayload().buffer
        );
        transaction.pb.setPayload(encryptedPayload);

        // Encrypt metadata using txKey
        if (transaction.pb.getMetadata() != null && transaction.pb.getMetadata().buffer != null) {
            debug('Metadata [%j]', transaction.pb.getMetadata().buffer);
            let encryptedMetadata = self.chain.cryptoPrimitives.eciesEncrypt(
                txKey.pubKeyObj,
                transaction.pb.getMetadata().buffer
            );
            transaction.pb.setMetadata(encryptedMetadata);
        }
    }

    private decryptResult(ct:Buffer) {
        let key = new Buffer(
            this.chain.cryptoPrimitives.hmacAESTruncated(
                this.member.getEnrollment().queryStateKey,
                [CONFIDENTIALITY_1_2_STATE_KD_C6].concat(this.nonce))
        );

        debug('Decrypt Result [%s]', ct.toString('hex'));
        return this.chain.cryptoPrimitives.aes256GCMDecrypt(key, ct);
    }

    /**
     * Create a deploy transaction.
     * @param request {Object} A BuildRequest or DeployRequest
     */
    private newBuildOrDeployTransaction(request:DeployRequest, isBuildRequest:boolean, cb:DeployTransactionCallback):void {
        debug("newBuildOrDeployTransaction");

        let self = this;

        // Determine if deployment is for dev mode or net mode
        if (self.chain.isDevMode()) {
            // Deployment in developent mode. Build a dev mode transaction.
            this.newDevModeTransaction(request, isBuildRequest, function(err, tx) {
                if(err) {
                    return cb(err);
                } else {
                    return cb(null, tx);
                }
            });
        } else {
            // Deployment in network mode. Build a net mode transaction.
            this.newNetModeTransaction(request, isBuildRequest, function(err, tx) {
                if(err) {
                    return cb(err);
                } else {
                    return cb(null, tx);
                }
            });
        }
    } // end newBuildOrDeployTransaction

    /**
     * Create a development mode deploy transaction.
     * @param request {Object} A development mode BuildRequest or DeployRequest
     */
    private newDevModeTransaction(request:DeployRequest, isBuildRequest:boolean, cb:DeployTransactionCallback):void {
        debug("newDevModeTransaction");

        let self = this;

        // Verify that chaincodeName is being passed
        if (!request.chaincodeName || request.chaincodeName === "") {
          return cb(Error("missing chaincodeName in DeployRequest"));
        }

        let tx = new _fabricProto.Transaction();

        if (isBuildRequest) {
            tx.setType(_fabricProto.Transaction.Type.CHAINCODE_BUILD);
        } else {
            tx.setType(_fabricProto.Transaction.Type.CHAINCODE_DEPLOY);
        }

        // Set the chaincodeID
        let chaincodeID = new _chaincodeProto.ChaincodeID();
        chaincodeID.setName(request.chaincodeName);
        debug("newDevModeTransaction: chaincodeID: " + JSON.stringify(chaincodeID));
        tx.setChaincodeID(chaincodeID.toBuffer());

        // Construct the ChaincodeSpec
        let chaincodeSpec = new _chaincodeProto.ChaincodeSpec();
        // Set Type -- GOLANG is the only chaincode language supported at this time
        chaincodeSpec.setType(_chaincodeProto.ChaincodeSpec.Type.GOLANG);
        // Set chaincodeID
        chaincodeSpec.setChaincodeID(chaincodeID);
        // Set ctorMsg
        let chaincodeInput = new _chaincodeProto.ChaincodeInput();
        chaincodeInput.setArgs(prepend(request.fcn, request.args));
        chaincodeSpec.setCtorMsg(chaincodeInput);

        // Construct the ChaincodeDeploymentSpec (i.e. the payload)
        let chaincodeDeploymentSpec = new _chaincodeProto.ChaincodeDeploymentSpec();
        chaincodeDeploymentSpec.setChaincodeSpec(chaincodeSpec);
        tx.setPayload(chaincodeDeploymentSpec.toBuffer());

        // Set the transaction UUID
        tx.setTxid(request.chaincodeName);

        // Set the transaction timestamp
        tx.setTimestamp(sdk_util.GenerateTimestamp());

        // Set confidentiality level
        if (request.confidential) {
            debug("Set confidentiality level to CONFIDENTIAL");
            tx.setConfidentialityLevel(_fabricProto.ConfidentialityLevel.CONFIDENTIAL);
        } else {
            debug("Set confidentiality level to PUBLIC");
            tx.setConfidentialityLevel(_fabricProto.ConfidentialityLevel.PUBLIC);
        }

        // Set request metadata
        if (request.metadata) {
            tx.setMetadata(request.metadata);
        }

        // Set the user certificate data
        if (request.userCert) {
            // cert based
            let certRaw = new Buffer(self.tcert.publicKey);
            // debug('========== Invoker Cert [%s]', certRaw.toString('hex'));
            let nonceRaw = new Buffer(self.nonce);
            let bindingMsg = Buffer.concat([certRaw, nonceRaw]);
            // debug('========== Binding Msg [%s]', bindingMsg.toString('hex'));
            this.binding = new Buffer(self.chain.cryptoPrimitives.hash(bindingMsg), 'hex');
            // debug('========== Binding [%s]', this.binding.toString('hex'));
            let ctor = chaincodeSpec.getCtorMsg().toBuffer();
            // debug('========== Ctor [%s]', ctor.toString('hex'));
            let txmsg = Buffer.concat([ctor, this.binding]);
            // debug('========== Payload||binding [%s]', txmsg.toString('hex'));
            let mdsig = self.chain.cryptoPrimitives.ecdsaSign(request.userCert.privateKey.getPrivate('hex'), txmsg);
            let sigma = new Buffer(mdsig.toDER());
            // debug('========== Sigma [%s]', sigma.toString('hex'));
            tx.setMetadata(sigma);
        }

        tx = new Transaction(tx, request.chaincodeName);

        return cb(null, tx);
    }

    /**
     * Create a network mode deploy transaction.
     * @param request {Object} A network mode BuildRequest or DeployRequest
     */
    private newNetModeTransaction(request:DeployRequest, isBuildRequest:boolean, cb:DeployTransactionCallback):void {
        debug("newNetModeTransaction");

        let self = this;

        // Verify that chaincodePath is being passed
        if (!request.chaincodePath || request.chaincodePath === "") {
          return cb(Error("missing chaincodePath in DeployRequest"));
        }

        // Determine the user's $GOPATH
        let goPath =  process.env['GOPATH'];
        debug("$GOPATH: " + goPath);

        // Compose the path to the chaincode project directory
        let projDir = goPath + "/src/" + request.chaincodePath;
        debug("projDir: " + projDir);

        // Compute the hash of the chaincode deployment parameters
        let hash = sdk_util.GenerateParameterHash(request.chaincodePath, request.fcn, request.args);

        // Compute the hash of the project directory contents
        hash = sdk_util.GenerateDirectoryHash(goPath + "/src/", request.chaincodePath, hash);
        debug("hash: " + hash);

        // Compose the Dockerfile commands
        let dockerFileContents =
          "from hyperledger/fabric-baseimage" + "\n" +
          "COPY . $GOPATH/src/build-chaincode/" + "\n" +
          "WORKDIR $GOPATH" + "\n\n" +
          "RUN go install build-chaincode && cp src/build-chaincode/vendor/github.com/hyperledger/fabric/peer/core.yaml $GOPATH/bin && mv $GOPATH/bin/build-chaincode $GOPATH/bin/%s";

        // Substitute the hashStrHash for the image name
        dockerFileContents = util.format(dockerFileContents, hash);

        // Add the certificate path on the server, if it is being passed in
        debug("type of request.certificatePath: " + typeof(request.certificatePath));
        debug("request.certificatePath: " + request.certificatePath);
        if (request.certificatePath !== "" && request.certificatePath !== undefined) {
            debug("Adding COPY certificate.pem command");

            dockerFileContents = dockerFileContents + "\n" + "COPY certificate.pem %s";
            dockerFileContents = util.format(dockerFileContents, request.certificatePath);
        }

        // Create a Docker file with dockerFileContents
        let dockerFilePath = projDir + "/Dockerfile";
        fs.writeFile(dockerFilePath, dockerFileContents, function(err) {
            if (err) {
                debug(util.format("Error writing file [%s]: %s", dockerFilePath, err));
                return cb(Error(util.format("Error writing file [%s]: %s", dockerFilePath, err)));
            }

            debug("Created Dockerfile at [%s]", dockerFilePath);

            // Create the .tar.gz file of the chaincode package
            let targzFilePath = "/tmp/deployment-package.tar.gz";
            // Create the compressed archive
            sdk_util.GenerateTarGz(projDir, targzFilePath, function(err) {
                if(err) {
                    debug(util.format("Error creating deployment archive [%s]: %s", targzFilePath, err));
                    return cb(Error(util.format("Error creating deployment archive [%s]: %s", targzFilePath, err)));
                }

                debug(util.format("Created deployment archive at [%s]", targzFilePath));

                //
                // Initialize a transaction structure
                //

                let tx = new _fabricProto.Transaction();

                //
                // Set the transaction type
                //

                if (isBuildRequest) {
                    tx.setType(_fabricProto.Transaction.Type.CHAINCODE_BUILD);
                } else {
                    tx.setType(_fabricProto.Transaction.Type.CHAINCODE_DEPLOY);
                }

                //
                // Set the chaincodeID
                //

                let chaincodeID = new _chaincodeProto.ChaincodeID();
                chaincodeID.setName(hash);
                debug("chaincodeID: " + JSON.stringify(chaincodeID));
                tx.setChaincodeID(chaincodeID.toBuffer());

                //
                // Set the payload
                //

                // Construct the ChaincodeSpec
                let chaincodeSpec = new _chaincodeProto.ChaincodeSpec();

                // Set Type -- GOLANG is the only chaincode language supported at this time
                chaincodeSpec.setType(_chaincodeProto.ChaincodeSpec.Type.GOLANG);
                // Set chaincodeID
                chaincodeSpec.setChaincodeID(chaincodeID);
                // Set ctorMsg
                let chaincodeInput = new _chaincodeProto.ChaincodeInput();
                chaincodeInput.setArgs(prepend(request.fcn, request.args));
                chaincodeSpec.setCtorMsg(chaincodeInput);
                debug("chaincodeSpec: " + JSON.stringify(chaincodeSpec));

                // Construct the ChaincodeDeploymentSpec and set it as the Transaction payload
                let chaincodeDeploymentSpec = new _chaincodeProto.ChaincodeDeploymentSpec();
                chaincodeDeploymentSpec.setChaincodeSpec(chaincodeSpec);

                // Read in the .tar.zg and set it as the CodePackage in ChaincodeDeploymentSpec
                fs.readFile(targzFilePath, function(err, data) {
                    if(err) {
                        debug(util.format("Error reading deployment archive [%s]: %s", targzFilePath, err));
                        return cb(Error(util.format("Error reading deployment archive [%s]: %s", targzFilePath, err)));
                    }

                    debug(util.format("Read in deployment archive from [%s]", targzFilePath));

                    chaincodeDeploymentSpec.setCodePackage(data);
                    tx.setPayload(chaincodeDeploymentSpec.toBuffer());

                    //
                    // Set the transaction ID
                    //

                    tx.setTxid(hash);

                    //
                    // Set the transaction timestamp
                    //

                    tx.setTimestamp(sdk_util.GenerateTimestamp());

                    //
                    // Set confidentiality level
                    //

                    if (request.confidential) {
                        debug("Set confidentiality level to CONFIDENTIAL");
                        tx.setConfidentialityLevel(_fabricProto.ConfidentialityLevel.CONFIDENTIAL);
                    } else {
                        debug("Set confidentiality level to PUBLIC");
                        tx.setConfidentialityLevel(_fabricProto.ConfidentialityLevel.PUBLIC);
                    }

                    //
                    // Set request metadata
                    //

                    if (request.metadata) {
                        tx.setMetadata(request.metadata);
                    }

                    //
                    // Set the user certificate data
                    //

                    if (request.userCert) {
                        // cert based
                        let certRaw = new Buffer(self.tcert.publicKey);
                        // debug('========== Invoker Cert [%s]', certRaw.toString('hex'));
                        let nonceRaw = new Buffer(self.nonce);
                        let bindingMsg = Buffer.concat([certRaw, nonceRaw]);
                        // debug('========== Binding Msg [%s]', bindingMsg.toString('hex'));
                        self.binding = new Buffer(self.chain.cryptoPrimitives.hash(bindingMsg), 'hex');
                        // debug('========== Binding [%s]', self.binding.toString('hex'));
                        let ctor = chaincodeSpec.getCtorMsg().toBuffer();
                        // debug('========== Ctor [%s]', ctor.toString('hex'));
                        let txmsg = Buffer.concat([ctor, self.binding]);
                        // debug('========== Payload||binding [%s]', txmsg.toString('hex'));
                        let mdsig = self.chain.cryptoPrimitives.ecdsaSign(request.userCert.privateKey.getPrivate('hex'), txmsg);
                        let sigma = new Buffer(mdsig.toDER());
                        // debug('========== Sigma [%s]', sigma.toString('hex'));
                        tx.setMetadata(sigma);
                    }

                    //
                    // Clean up temporary files
                    //

                    // Remove the temporary .tar.gz with the deployment contents and the Dockerfile
                    fs.unlink(targzFilePath, function(err) {
                        if(err) {
                            debug(util.format("Error deleting temporary archive [%s]: %s", targzFilePath, err));
                            return cb(Error(util.format("Error deleting temporary archive [%s]: %s", targzFilePath, err)));
                        }

                        debug("Temporary archive deleted successfully ---> " + targzFilePath);

                        fs.unlink(dockerFilePath, function(err) {
                            if(err) {
                                debug(util.format("Error deleting temporary file [%s]: %s", dockerFilePath, err));
                                return cb(Error(util.format("Error deleting temporary file [%s]: %s", dockerFilePath, err)));
                            }

                            debug("File deleted successfully ---> " + dockerFilePath);

                            //
                            // Return the deploy transaction structure
                            //

                            tx = new Transaction(tx, hash);

                            return cb(null, tx);
                        }); // end delete Dockerfile
                    }); // end delete .tar.gz
                }); // end reading .tar.zg and composing transaction
            }); // end writing .tar.gz
        }); // end writing Dockerfile
    }

    /**
     * Create an invoke or query transaction.
     * @param request {Object} A build or deploy request of the form: { chaincodeID, payload, metadata, uuid, timestamp, confidentiality: { level, version, nonce }
     */
    private newInvokeOrQueryTransaction(request:InvokeOrQueryRequest, isInvokeRequest:boolean, cb:InvokeOrQueryTransactionCallback):void {
        let self = this;

        // Verify that chaincodeID is being passed
        if (!request.chaincodeID || request.chaincodeID === "") {
          return cb(Error("missing chaincodeID in InvokeOrQueryRequest"));
        }

        // Create a deploy transaction
        let tx = new _fabricProto.Transaction();
        if (isInvokeRequest) {
            tx.setType(_fabricProto.Transaction.Type.CHAINCODE_INVOKE);
        } else {
            tx.setType(_fabricProto.Transaction.Type.CHAINCODE_QUERY);
        }

        // Set the chaincodeID
        let chaincodeID = new _chaincodeProto.ChaincodeID();
        chaincodeID.setName(request.chaincodeID);
        debug("newInvokeOrQueryTransaction: request=%j, chaincodeID=%s", request, JSON.stringify(chaincodeID));
        tx.setChaincodeID(chaincodeID.toBuffer());

        // Construct the ChaincodeSpec
        let chaincodeSpec = new _chaincodeProto.ChaincodeSpec();
        // Set Type -- GOLANG is the only chaincode language supported at this time
        chaincodeSpec.setType(_chaincodeProto.ChaincodeSpec.Type.GOLANG);
        // Set chaincodeID
        chaincodeSpec.setChaincodeID(chaincodeID);
        // Set ctorMsg
        let chaincodeInput = new _chaincodeProto.ChaincodeInput();
        chaincodeInput.setArgs(prepend(request.fcn, request.args));
        chaincodeSpec.setCtorMsg(chaincodeInput);
        // Construct the ChaincodeInvocationSpec (i.e. the payload)
        let chaincodeInvocationSpec = new _chaincodeProto.ChaincodeInvocationSpec();
        chaincodeInvocationSpec.setChaincodeSpec(chaincodeSpec);
        tx.setPayload(chaincodeInvocationSpec.toBuffer());

        // Set the transaction UUID
        tx.setTxid(sdk_util.GenerateUUID());

        // Set the transaction timestamp
        tx.setTimestamp(sdk_util.GenerateTimestamp());

        // Set confidentiality level
        if (request.confidential) {
            debug('Set confidentiality on');
            tx.setConfidentialityLevel(_fabricProto.ConfidentialityLevel.CONFIDENTIAL)
        } else {
            debug('Set confidentiality on');
            tx.setConfidentialityLevel(_fabricProto.ConfidentialityLevel.PUBLIC)
        }

        if (request.metadata) {
            tx.setMetadata(request.metadata)
        }

        if (request.userCert) {
            // cert based
            let certRaw = new Buffer(self.tcert.publicKey);
            // debug('========== Invoker Cert [%s]', certRaw.toString('hex'));
            let nonceRaw = new Buffer(self.nonce);
            let bindingMsg = Buffer.concat([certRaw, nonceRaw]);
            // debug('========== Binding Msg [%s]', bindingMsg.toString('hex'));
            this.binding = new Buffer(self.chain.cryptoPrimitives.hash(bindingMsg), 'hex');
            // debug('========== Binding [%s]', this.binding.toString('hex'));
            let ctor = chaincodeSpec.getCtorMsg().toBuffer();
            // debug('========== Ctor [%s]', ctor.toString('hex'));
            let txmsg = Buffer.concat([ctor, this.binding]);
            // debug('========== Pyaload||binding [%s]', txmsg.toString('hex'));
            let mdsig = self.chain.cryptoPrimitives.ecdsaSign(request.userCert.privateKey.getPrivate('hex'), txmsg);
            let sigma = new Buffer(mdsig.toDER());
            // debug('========== Sigma [%s]', sigma.toString('hex'));
            tx.setMetadata(sigma)
        }

        tx = new Transaction(tx, request.chaincodeID);

        return cb(null, tx);
    }

}  // end TransactionContext

// A class to get TCerts.
// There is one class per set of attributes requested by each member.
class TCertGetter {

    private chain:Chain;
    private member:Member;
    private attrs:string[];
    private key:string;
    private memberServices:MemberServices;
    private tcerts:any[] = [];
    private arrivalRate:stats.Rate = new stats.Rate();
    private getTCertResponseTime:stats.ResponseTime = new stats.ResponseTime();
    private getTCertWaiters:GetTCertCallback[] = [];
    private gettingTCerts:boolean = false;

    /**
    * Constructor for a member.
    * @param cfg {string | RegistrationRequest} The member name or registration request.
    * @returns {Member} A member who is neither registered nor enrolled.
    */
    constructor(member:Member, attrs:string[], key:string) {
        this.member = member;
        this.attrs = attrs;
        this.key = key;
        this.chain = member.getChain();
        this.memberServices = member.getMemberServices();
        this.tcerts = [];
    }

    /**
    * Get the chain.
    * @returns {Chain} The chain.
    */
    getChain():Chain {
        return this.chain;
    };

    getUserCert(cb:GetTCertCallback):void {
        this.getNextTCert(cb);
    }

    /**
    * Get the next available transaction certificate.
    * @param cb
    */
    getNextTCert(cb:GetTCertCallback):void {
        let self = this;
        self.arrivalRate.tick();
        let tcert = self.tcerts.length > 0? self.tcerts.shift() : undefined;
        if (tcert) {
            return cb(null,tcert);
        } else {
           if (!cb) throw Error("null callback");
            self.getTCertWaiters.push(cb);
        }
        if (self.shouldGetTCerts()) {
            self.getTCerts();
        }
    }

    // Determine if we should issue a request to get more tcerts now.
    private shouldGetTCerts(): boolean {
        let self = this;
        // Do nothing if we are already getting more tcerts
        if (self.gettingTCerts) {
            debug("shouldGetTCerts: no, already getting tcerts");
            return false;
        }
        // If there are none, then definitely get more
        if (self.tcerts.length == 0) {
            debug("shouldGetTCerts: yes, we have no tcerts");
            return true;
        }
        // If we aren't in prefetch mode, return false;
        if (!self.chain.isPreFetchMode()) {
            debug("shouldGetTCerts: no, prefetch disabled");
            return false;
        }
        // Otherwise, see if we should prefetch based on the arrival rate
        // (i.e. the rate at which tcerts are requested) and the response
        // time.
        // "arrivalRate" is in req/ms and "responseTime" in ms,
        // so "tcertCountThreshold" is number of tcerts at which we should
        // request the next batch of tcerts so we don't have to wait on the
        // transaction path.  Note that we add 1 sec to the average response
        // time to add a little buffer time so we don't have to wait.
        let arrivalRate = self.arrivalRate.getValue();
        let responseTime = self.getTCertResponseTime.getValue() + 1000;
        let tcertThreshold = arrivalRate * responseTime;
        let tcertCount = self.tcerts.length;
        let result = tcertCount <= tcertThreshold;
        debug(util.format("shouldGetTCerts: %s, threshold=%s, count=%s, rate=%s, responseTime=%s",
        result, tcertThreshold, tcertCount, arrivalRate, responseTime));
        return result;
    }

    // Call member services to get more tcerts
    private getTCerts():void {
        let self = this;
        let req = {
            name: self.member.getName(),
            enrollment: self.member.getEnrollment(),
            num: self.member.getTCertBatchSize(),
            attrs: self.attrs
        };
        self.getTCertResponseTime.start();
        self.memberServices.getTCertBatch(req, function (err, tcerts) {
            if (err) {
                self.getTCertResponseTime.cancel();
                // Error all waiters
                while (self.getTCertWaiters.length > 0) {
                    self.getTCertWaiters.shift()(err);
                }
                return;
            }
            self.getTCertResponseTime.stop();
            // Add to member's tcert list
            while (tcerts.length > 0) {
                self.tcerts.push(tcerts.shift());
            }
            // Allow waiters to proceed
            while (self.getTCertWaiters.length > 0 && self.tcerts.length > 0) {
                let waiter = self.getTCertWaiters.shift();
                waiter(null,self.tcerts.shift());
            }
        });
    }

} // end TCertGetter

/**
 * The Peer class represents a peer to which HFC sends deploy, invoke, or query requests.
 */
export class Peer {

    private url:string;
    private chain:Chain;
    private ep:Endpoint;
    private peerClient:any;

    /**
     * Constructs a Peer given its endpoint configuration settings
     * and returns the new Peer.
     * @param {string} url The URL with format of "grpcs://host:port".
     * @param {Chain} chain The chain of which this peer is a member.
     * @param {GRPCOptions} optional GRPC options to use with the gRPC,
     * protocol (that is, with TransportCredentials) including a root
     * certificate file, in PEM format, and hostnameOverride. A certificate
     * is required when using the grpcs (TLS) protocol.
     * @returns {Peer} The new peer.
     */
    constructor(url:string, chain:Chain, opts:GRPCOptions) {
        this.url = url;
        this.chain = chain;
        let pem = getPemFromOpts(opts);
        opts = getOptsFromOpts(opts);
        this.ep = new Endpoint(url,pem);
        this.peerClient = new _fabricProto.Peer(this.ep.addr, this.ep.creds, opts);
    }

    /**
     * Get the chain of which this peer is a member.
     * @returns {Chain} The chain of which this peer is a member.
     */
    getChain():Chain {
        return this.chain;
    }

    /**
     * Get the URL of the peer.
     * @returns {string} Get the URL associated with the peer.
     */
    getUrl():string {
        return this.url;
    }

    /**
     * Send a transaction to this peer.
     * @param tx A transaction
     * @param eventEmitter The event emitter
     */
    sendTransaction = function (tx:Transaction, eventEmitter:events.EventEmitter) {
        var self = this;

        debug("peer.sendTransaction");

        // Send the transaction to the peer node via grpc
        // The rpc specification on the peer side is:
        //     rpc ProcessTransaction(Transaction) returns (Response) {}
        self.peerClient.processTransaction(tx.pb, function (err, response) {
            if (err) {
                debug("peer.sendTransaction: error=%j", err);
                return eventEmitter.emit('error', new EventTransactionError(err));
            }

            debug("peer.sendTransaction: received %j", response);

            // Check transaction type here, as invoke is an asynchronous call,
            // whereas a deploy and a query are synchonous calls. As such,
            // invoke will emit 'submitted' and 'error', while a deploy/query
            // will emit 'complete' and 'error'.
            let txType = tx.pb.getType();
            switch (txType) {
               case _fabricProto.Transaction.Type.CHAINCODE_DEPLOY: // async
                  if (response.status === "SUCCESS") {
                     // Deploy transaction has been completed
                     if (!response.msg || response.msg === "") {
                        eventEmitter.emit("error", new EventTransactionError("the deploy response is missing the transaction UUID"));
                     } else {
                        let event = new EventDeploySubmitted(response.msg.toString(), tx.chaincodeID);
                        debug("EventDeploySubmitted event: %j", event);
                        eventEmitter.emit("submitted", event);
                     }
                  } else {
                     // Deploy completed with status "FAILURE" or "UNDEFINED"
                     eventEmitter.emit("error", new EventTransactionError(response));
                  }
                  break;
               case _fabricProto.Transaction.Type.CHAINCODE_INVOKE: // async
                  if (response.status === "SUCCESS") {
                     // Invoke transaction has been submitted
                     if (!response.msg || response.msg === "") {
                        eventEmitter.emit("error", new EventTransactionError("the invoke response is missing the transaction UUID"));
                     } else {
                        eventEmitter.emit("submitted", new EventInvokeSubmitted(response.msg.toString()));
                     }
                  } else {
                     // Invoke completed with status "FAILURE" or "UNDEFINED"
                     eventEmitter.emit("error", new EventTransactionError(response));
                  }
                  break;
               case _fabricProto.Transaction.Type.CHAINCODE_QUERY: // sync
                  if (response.status === "SUCCESS") {
                     // Query transaction has been completed
                     eventEmitter.emit("complete", new EventQueryComplete(response.msg));
                  } else {
                     // Query completed with status "FAILURE" or "UNDEFINED"
                     eventEmitter.emit("error", new EventTransactionError(response));
                  }
                  break;
               default: // not implemented
                  eventEmitter.emit("error", new EventTransactionError("processTransaction for this transaction type is not yet implemented!"));
            }
          });
    };

    /**
     * Remove the peer from the chain.
     */
    remove():void {
        throw Error("TODO: implement");
    }

} // end Peer

/**
 * An endpoint currently takes only URL (currently).
 * @param url
 */
class Endpoint {

    addr:string;
    creds:Buffer;

    constructor(url:string, pem?:string) {
        let purl = parseUrl(url);
        var protocol;
        if (purl.protocol) {
            protocol = purl.protocol.toLowerCase().slice(0,-1);
        }
        if (protocol === 'grpc') {
            this.addr = purl.host;
            this.creds = grpc.credentials.createInsecure();
        }
        else if (protocol === 'grpcs') {
            this.addr = purl.host;
            this.creds = grpc.credentials.createSsl(new Buffer(pem));
        }
        else {
            var error = new Error();
            error.name = "InvalidProtocol";
            error.message = "Invalid protocol: " + protocol +
                ".  URLs must begin with grpc:// or grpcs://"
            throw error;
        }
    }
}

/**
 * MemberServicesImpl is the default implementation of a member services client.
 */
class MemberServicesImpl implements MemberServices {

    private ecaaClient:any;
    private ecapClient:any;
    private tcapClient:any;
    private tlscapClient:any;
    private cryptoPrimitives:crypto.Crypto;

    /**
     * MemberServicesImpl constructor
     * @param config The config information required by this member services implementation.
     * @returns {MemberServices} A MemberServices object.
     */
    constructor(url:string,opts:GRPCOptions) {
        var pem = getPemFromOpts(opts);
        opts = getOptsFromOpts(opts);
        let ep = new Endpoint(url,pem);
        this.ecaaClient = new _caProto.ECAA(ep.addr, ep.creds, opts);
        this.ecapClient = new _caProto.ECAP(ep.addr, ep.creds, opts);
        this.tcapClient = new _caProto.TCAP(ep.addr, ep.creds, opts);
        this.tlscapClient = new _caProto.TLSCAP(ep.addr, ep.creds, opts);
        this.cryptoPrimitives = new crypto.Crypto(DEFAULT_HASH_ALGORITHM, DEFAULT_SECURITY_LEVEL);
    }

    /**
     * Get the security level
     * @returns The security level
     */
    getSecurityLevel():number {
        return this.cryptoPrimitives.getSecurityLevel();
    }

    /**
     * Set the security level
     * @params securityLevel The security level
     */
    setSecurityLevel(securityLevel:number):void {
        this.cryptoPrimitives.setSecurityLevel(securityLevel);
    }

    /**
     * Get the hash algorithm
     * @returns {string} The hash algorithm
     */
    getHashAlgorithm():string {
        return this.cryptoPrimitives.getHashAlgorithm();
    }

    /**
     * Set the hash algorithm
     * @params hashAlgorithm The hash algorithm ('SHA2' or 'SHA3')
     */
    setHashAlgorithm(hashAlgorithm:string):void {
        this.cryptoPrimitives.setHashAlgorithm(hashAlgorithm);
    }

    /**
     * Get the crypto object.
     */
    getCrypto():crypto.Crypto {
        return this.cryptoPrimitives;
    }

    /**
     * Register the member and return an enrollment secret.
     * @param req Registration request with the following fields: name, role
     * @param registrar The identity of the registrar (i.e. who is performing the registration)
     * @param cb Callback of the form: {function(err,enrollmentSecret)}
     */
    register(req:RegistrationRequest, registrar:Member, cb:RegisterCallback):void {
        let self = this;
        debug("MemberServicesImpl.register: req=%j", req);
        if (!req.enrollmentID) return cb(new Error("missing req.enrollmentID"));
        if (!registrar) return cb(new Error("chain registrar is not set"));
        // Create proto request
        let protoReq = new _caProto.RegisterUserReq();
        protoReq.setId({id:req.enrollmentID});
        protoReq.setRole(rolesToMask(req.roles));
        protoReq.setAffiliation(req.affiliation);
        let attrs = req.attributes;
        if (Array.isArray(attrs)) {
           let pattrs = [];
           for (var i = 0; i < attrs.length; i++) {
              var attr = attrs[i];
              var pattr = new _caProto.Attribute();
              if (attr.name) pattr.setName(attr.name);
              if (attr.value) pattr.setValue(attr.value);
              if (attr.notBefore) pattr.setNotBefore(attr.notBefore);
              if (attr.notAfter) pattr.setNotAfter(attr.notAfter);
              pattrs.push(pattr);
           }
           protoReq.setAttributes(pattrs);
        }
        // Create registrar info
        let protoRegistrar = new _caProto.Registrar();
        protoRegistrar.setId({id:registrar.getName()});
        if (req.registrar) {
            if (req.registrar.roles) {
               protoRegistrar.setRoles(req.registrar.roles);
            }
            if (req.registrar.delegateRoles) {
               protoRegistrar.setDelegateRoles(req.registrar.delegateRoles);
            }
        }
        protoReq.setRegistrar(protoRegistrar);
        // Sign the registration request
        var buf = protoReq.toBuffer();
        var signKey = self.cryptoPrimitives.ecdsaKeyFromPrivate(registrar.getEnrollment().key, 'hex');
        var sig = self.cryptoPrimitives.ecdsaSign(signKey, buf);
        protoReq.setSig( new _caProto.Signature(
            {
                type: _caProto.CryptoType.ECDSA,
                r: new Buffer(sig.r.toString()),
                s: new Buffer(sig.s.toString())
            }
        ));
        // Send the registration request
        self.ecaaClient.registerUser(protoReq, function (err, token) {
            debug("register %j: err=%j, token=%s", protoReq, err, token);
            if (cb) return cb(err, token ? token.tok.toString() : null);
        });
    }

    /**
     * Enroll the member and return an opaque member object
     * @param req Enrollment request with the following fields: name, enrollmentSecret
     * @param cb Callback of the form: {function(err,{key,cert,chainKey})}
     */
    enroll(req:EnrollmentRequest, cb:EnrollCallback):void {
        let self = this;
        cb = cb || nullCB;

        debug("[MemberServicesImpl.enroll] [%j]", req);
        if (!req.enrollmentID) return cb(Error("req.enrollmentID is not set"));
        if (!req.enrollmentSecret) return cb(Error("req.enrollmentSecret is not set"));

        debug("[MemberServicesImpl.enroll] Generating keys...");

        // generate ECDSA keys: signing and encryption keys
        // 1) signing key
        var signingKeyPair = self.cryptoPrimitives.ecdsaKeyGen();
        var spki = new asn1.x509.SubjectPublicKeyInfo(signingKeyPair.pubKeyObj);
        // 2) encryption key
        var encryptionKeyPair = self.cryptoPrimitives.ecdsaKeyGen();
        var spki2 = new asn1.x509.SubjectPublicKeyInfo(encryptionKeyPair.pubKeyObj);

        debug("[MemberServicesImpl.enroll] Generating keys...done!");

        // create the proto message
        var eCertCreateRequest = new _caProto.ECertCreateReq();
        var timestamp = sdk_util.GenerateTimestamp();
        eCertCreateRequest.setTs(timestamp);
        eCertCreateRequest.setId({id: req.enrollmentID});
        eCertCreateRequest.setTok({tok: new Buffer(req.enrollmentSecret)});

        debug("[MemberServicesImpl.enroll] Generating request! %j", spki.getASN1Object().getEncodedHex());

        // public signing key (ecdsa)
        var signPubKey = new _caProto.PublicKey(
            {
                type: _caProto.CryptoType.ECDSA,
                key: new Buffer(spki.getASN1Object().getEncodedHex(), 'hex')
            });
        eCertCreateRequest.setSign(signPubKey);

        debug("[MemberServicesImpl.enroll] Adding signing key!");

        // public encryption key (ecdsa)
        var encPubKey = new _caProto.PublicKey(
            {
                type: _caProto.CryptoType.ECDSA,
                key: new Buffer(spki2.getASN1Object().getEncodedHex(), 'hex')
            });
        eCertCreateRequest.setEnc(encPubKey);

        debug("[MemberServicesImpl.enroll] Assding encryption key!");

        debug("[MemberServicesImpl.enroll] [Contact ECA] %j ", eCertCreateRequest);
        self.ecapClient.createCertificatePair(eCertCreateRequest, function (err, eCertCreateResp) {
            if (err) {
                debug("[MemberServicesImpl.enroll] failed to create cert pair: err=%j", err);
                return cb(err);
            }
            let cipherText = eCertCreateResp.tok.tok;
            var decryptedTokBytes = self.cryptoPrimitives.eciesDecrypt(encryptionKeyPair.prvKeyObj, cipherText);

            //debug(decryptedTokBytes);
            // debug(decryptedTokBytes.toString());
            // debug('decryptedTokBytes [%s]', decryptedTokBytes.toString());
            eCertCreateRequest.setTok({tok: decryptedTokBytes});
            eCertCreateRequest.setSig(null);

            var buf = eCertCreateRequest.toBuffer();

            var signKey = self.cryptoPrimitives.ecdsaKeyFromPrivate(signingKeyPair.prvKeyObj.prvKeyHex, 'hex');
            //debug(new Buffer(sha3_384(buf),'hex'));
            var sig = self.cryptoPrimitives.ecdsaSign(signKey, buf);

            eCertCreateRequest.setSig(new _caProto.Signature(
                {
                    type: _caProto.CryptoType.ECDSA,
                    r: new Buffer(sig.r.toString()),
                    s: new Buffer(sig.s.toString())
                }
            ));
            self.ecapClient.createCertificatePair(eCertCreateRequest, function (err, eCertCreateResp) {
                if (err) return cb(err);
                debug('[MemberServicesImpl.enroll] eCertCreateResp : [%j]' + eCertCreateResp);

                let enrollment = {
                    key: signingKeyPair.prvKeyObj.prvKeyHex,
                    cert: eCertCreateResp.certs.sign.toString('hex'),
                    chainKey: eCertCreateResp.pkchain.toString('hex')
                };
                // debug('cert:\n\n',enrollment.cert)
                cb(null, enrollment);
            });
        });

    } // end enroll

    /**
     * Get an array of transaction certificates (tcerts).
     * @param {Object} req Request of the form: {name,enrollment,num} where
     * 'name' is the member name,
     * 'enrollment' is what was returned by enroll, and
     * 'num' is the number of transaction contexts to obtain.
     * @param {function(err,[Object])} cb The callback function which is called with an error as 1st arg and an array of tcerts as 2nd arg.
     */
    getTCertBatch(req:GetTCertBatchRequest, cb:GetTCertBatchCallback): void {
        let self = this;
        cb = cb || nullCB;

        let timestamp = sdk_util.GenerateTimestamp();

        // create the proto
        let tCertCreateSetReq = new _caProto.TCertCreateSetReq();
        tCertCreateSetReq.setTs(timestamp);
        tCertCreateSetReq.setId({id: req.name});
        tCertCreateSetReq.setNum(req.num);
        if (req.attrs) {
            let attrs = [];
            for (let i = 0; i < req.attrs.length; i++) {
                attrs.push({attributeName:req.attrs[i]});
            }
            tCertCreateSetReq.setAttributes(attrs);
        }

        // serialize proto
        let buf = tCertCreateSetReq.toBuffer();

        // sign the transaction using enrollment key
        let signKey = self.cryptoPrimitives.ecdsaKeyFromPrivate(req.enrollment.key, 'hex');
        let sig = self.cryptoPrimitives.ecdsaSign(signKey, buf);

        tCertCreateSetReq.setSig(new _caProto.Signature(
            {
                type: _caProto.CryptoType.ECDSA,
                r: new Buffer(sig.r.toString()),
                s: new Buffer(sig.s.toString())
            }
        ));

        // send the request
        self.tcapClient.createCertificateSet(tCertCreateSetReq, function (err, resp) {
            if (err) return cb(err);
            // debug('tCertCreateSetResp:\n', resp);
            cb(null, self.processTCertBatch(req, resp));
        });
    }

    /**
     * Process a batch of tcerts after having retrieved them from the TCA.
     */
    private processTCertBatch(req:GetTCertBatchRequest, resp:any):TCert[] {
        let self = this;

        //
        // Derive secret keys for TCerts
        //

        let enrollKey = req.enrollment.key;
        let tCertOwnerKDFKey = resp.certs.key;
        let tCerts = resp.certs.certs;

        let byte1 = new Buffer(1);
        byte1.writeUInt8(0x1, 0);
        let byte2 = new Buffer(1);
        byte2.writeUInt8(0x2, 0);

        let tCertOwnerEncryptKey = self.cryptoPrimitives.hmac(tCertOwnerKDFKey, byte1).slice(0, 32);
        let expansionKey = self.cryptoPrimitives.hmac(tCertOwnerKDFKey, byte2);

        let tCertBatch:TCert[] = [];

        // Loop through certs and extract private keys
        for (var i = 0; i < tCerts.length; i++) {
            var tCert = tCerts[i];
            let x509Certificate;
            try {
                x509Certificate = new crypto.X509Certificate(tCert.cert);
            } catch (ex) {
                debug('Warning: problem parsing certificate bytes; retrying ... ', ex);
                continue
            }

            // debug("HERE2: got x509 cert");
            // extract the encrypted bytes from extension attribute
            let tCertIndexCT = x509Certificate.criticalExtension(crypto.TCertEncTCertIndex);
            // debug('tCertIndexCT: ',JSON.stringify(tCertIndexCT));
            let tCertIndex = self.cryptoPrimitives.aesCBCPKCS7Decrypt(tCertOwnerEncryptKey, tCertIndexCT);
            // debug('tCertIndex: ',JSON.stringify(tCertIndex));

            let expansionValue = self.cryptoPrimitives.hmac(expansionKey, tCertIndex);
            // debug('expansionValue: ',expansionValue);

            // compute the private key
            let one = new BN(1);
            let k = new BN(expansionValue);
            let n = self.cryptoPrimitives.ecdsaKeyFromPrivate(enrollKey, 'hex').ec.curve.n.sub(one);
            k = k.mod(n).add(one);

            let D = self.cryptoPrimitives.ecdsaKeyFromPrivate(enrollKey, 'hex').getPrivate().add(k);
            let pubHex = self.cryptoPrimitives.ecdsaKeyFromPrivate(enrollKey, 'hex').getPublic('hex');
            D = D.mod(self.cryptoPrimitives.ecdsaKeyFromPublic(pubHex, 'hex').ec.curve.n);

            // Put private and public key in returned tcert
            let tcert = new TCert(tCert.cert, self.cryptoPrimitives.ecdsaKeyFromPrivate(D, 'hex'));
            tCertBatch.push(tcert);
        }

        if (tCertBatch.length == 0) {
            throw Error('Failed fetching TCertBatch. No valid TCert received.')
        }

        return tCertBatch;

    } // end processTCertBatch

} // end MemberServicesImpl

function newMemberServices(url:string,opts:GRPCOptions) {
    return new MemberServicesImpl(url,opts);
}

/**
 * A local file-based key value store.
 * This implements the KeyValStore interface.
 */
class FileKeyValStore implements KeyValStore {

    private dir:string;   // root directory for the file store

    constructor(dir:string) {
        this.dir = dir;
        if (!fs.existsSync(dir)) {
            fs.mkdirSync(dir);
        }
    }

    /**
     * Get the value associated with name.
     * @param name
     * @param cb function(err,value)
     */
    getValue(name:string, cb:GetValueCallback) {
        var path = this.dir + '/' + name;
        fs.readFile(path, 'utf8', function (err, data) {
            if (err) {
                if (err.code !== 'ENOENT') return cb(err);
                return cb(null, null);
            }
            return cb(null, data);
        });
    }

    /**
     * Set the value associated with name.
     * @param name
     * @param cb function(err)
     */
    setValue = function (name:string, value:string, cb:ErrorCallback) {
        var path = this.dir + '/' + name;
        fs.writeFile(path, value, cb);
    }

} // end FileKeyValStore

function toKeyValStoreName(name:string):string {
    return "member." + name;
}

// Return a unique string value for the list of attributes.
function getAttrsKey(attrs?:string[]): string {
    if (!attrs) return "null";
    let key = "[]";
    for (let i = 0; i < attrs.length; i++) {
       key += "," + attrs[i];
    }
    return key;
}

// A null callback to use when the user doesn't pass one in
function nullCB():void {
}

// Determine if an object is a string
function isString(obj:any):boolean {
    return (typeof obj === 'string' || obj instanceof String);
}

// Determine if 'obj' is an object (not an array, string, or other type)
function isObject(obj:any):boolean {
    return (!!obj) && (obj.constructor === Object);
}

function isFunction(fcn:any):boolean {
    return (typeof fcn === 'function');
}

function parseUrl(url:string):any {
    // TODO: find ambient definition for url
    var purl = urlParser.parse(url, true);
    return purl;
}

// Convert a list of member type names to the role mask currently used by the peer
function rolesToMask(roles?:string[]):number {
    let mask:number = 0;
    if (roles) {
        for (let role in roles) {
            switch (roles[role]) {
                case 'client':
                    mask |= 1;
                    break;       // Client mask
                case 'peer':
                    mask |= 2;
                    break;       // Peer mask
                case 'validator':
                    mask |= 4;
                    break;  // Validator mask
                case 'auditor':
                    mask |= 8;
                    break;    // Auditor mask
            }
        }
    }
    if (mask === 0) mask = 1;  // Client
    return mask;
}

// Get the PEM from the options
function getPemFromOpts(opts:any):string {
   if (isObject(opts)) return opts.pem;
   return opts;
}

// Normalize opts
function getOptsFromOpts(opts:any):GRPCOptions {
    if (typeof opts === 'object') {
        var optCopy = {};

        for (var prop in opts) {
            if (prop !== 'pem') {
                if (prop === 'hostnameOverride') {
                    optCopy['grpc.ssl_target_name_override'] = opts.hostnameOverride;
                    optCopy['grpc.default_authority'] = opts.hostnameOverride;
                } else {
                    optCopy[prop] = opts[prop];
                }
            }
        }

        return <GRPCOptions>optCopy;
    }

    if (typeof opts === 'string') {
        // backwards compatible to handle pem as opts
        return <GRPCOptions>{ pem: opts };
    }
}

function endsWith(str:string, suffix:string) {
    return str.length >= suffix.length && str.substr(str.length - suffix.length) === suffix;
};

function prepend(item:string, list:string[]) {
    var l = list.slice();
    l.unshift(item);
    return l.map(function(x) { return new Buffer(x) });
};

/**
 * Create a new chain.  If it already exists, throws an Error.
 * @param name {string} Name of the chain.  It can be any name and has value only for the client.
 * @returns
 */
export function newChain(name) {
    let chain = _chains[name];
    if (chain) throw Error(util.format("chain %s already exists", name));
    chain = new Chain(name);
    _chains[name] = chain;
    return chain;
}

/**
 * Get a chain.  If it doesn't yet exist and 'create' is true, create it.
 * @param {string} chainName The name of the chain to get or create.
 * @param {boolean} create If the chain doesn't already exist, specifies whether to create it.
 * @return {Chain} Returns the chain, or null if it doesn't exist and create is false.
 */
export function getChain(chainName, create) {
    let chain = _chains[chainName];
    if (!chain && create) {
        chain = newChain(chainName);
    }
    return chain;
}

/**
 * Create an instance of a FileKeyValStore.
 */
export function newFileKeyValStore(dir:string):KeyValStore {
    return new FileKeyValStore(dir);
}

/**
 * The ChainCodeCBE is used internal to the EventHub to hold chaincode event registration callbacks.
 */
export class ChainCodeCBE {
    // chaincode id
    ccid: string;
    // event name regex filter
    eventNameFilter: RegExp;
    // callback function to invoke on successful filter match
    cb: Function;
    constructor(ccid: string, eventNameFilter: string, cb: Function) {
        this.ccid = ccid;
        this.eventNameFilter = new RegExp(eventNameFilter);
        this.cb = cb;
    }
}

/**
 * The EventHub is used to distribute events from a specific event source(peer)
 */
export class EventHub {
        // peer addr to connect to
        private ep: Endpoint;
        // grpc options
        private opts: GRPCOptions;
        // grpc events interface
        private events: any;
        // grpc event client interface
        private client: any;
        // grpc chat streaming interface
        private call: any;
        // hashtable of clients registered for chaincode events
        private chaincodeRegistrants: any;
        // set of clients registered for block events
        private blockRegistrants: any;
        // hashtable of clients registered for transactional events
        private txRegistrants: any;
         // fabric connection state of this eventhub
        private connected: boolean;
    constructor() {
        this.chaincodeRegistrants = new HashTable();
        this.blockRegistrants = new Set();
        this.txRegistrants = new HashTable();
        this.ep = null;
        this.connected = false;
    }

    public setPeerAddr(peeraddr: string, opts?:GRPCOptions) {
        let pem = getPemFromOpts(opts);
        this.opts = getOptsFromOpts(opts);
        this.ep = new Endpoint(peeraddr,pem);
    }

    public isconnected() {
        return this.connected;
    }

    public connect() {
        if (this.connected) return;
        if (!this.ep) throw Error("Must set peer address before connecting.");
        this.events = grpc.load(__dirname + "/protos/events.proto" ).protos;
        this.client = new this.events.Events(this.ep.addr, this.ep.creds, this.opts);
        this.call = this.client.chat();
        this.connected = true;
        this.registerBlockEvent(this.txCallback);

        let eh = this; // for callback context
        this.call.on('data', function(event) {
            if ( event.Event == "chaincodeEvent" ) {
                var cbtable = eh.chaincodeRegistrants.get(event.chaincodeEvent.chaincodeID);
                if( !cbtable ) {
                    return;
                }
                cbtable.forEach(function (cbe) {
                    if ( cbe.eventNameFilter.test(event.chaincodeEvent.eventName)) {
                        cbe.cb(event.chaincodeEvent);
                    }
                });
        } else if ( event.Event == "block") {
                    eh.blockRegistrants.forEach(function(cb){
                      cb(event.block);
                });
        }
        });
        this.call.on('end', function()  {
            eh.call.end();
            // clean up Registrants - should app get notified?
            eh.chaincodeRegistrants.clear();
            eh.blockRegistrants.clear();
        });
    }

    public disconnect() {
        if (!this.connected) return;
        this.unregisterBlockEvent(this.txCallback);
        this.call.end();
        this.connected = false;
    }

    public registerChaincodeEvent(ccid: string, eventname: string, callback: Function): ChainCodeCBE {
        if (!this.connected) return;
        let cb = new ChainCodeCBE(ccid, eventname, callback);
        let cbtable = this.chaincodeRegistrants.get(ccid);
        if ( !cbtable ) {
            cbtable = new Set();
            this.chaincodeRegistrants.put(ccid, cbtable);
            cbtable.add(cb);
            let register = { register: { events: [ { eventType: "CHAINCODE", chaincodeRegInfo:{ chaincodeID: ccid , eventName: "" }} ] }};
            this.call.write(register);
        } else {
            cbtable.add(cb);
        }
        return cb;
    }

    public unregisterChaincodeEvent(cbe: ChainCodeCBE){
        if (!this.connected) return;
        let cbtable = this.chaincodeRegistrants.get(cbe.ccid);
        if ( !cbtable ) {
            debug("No event registration for ccid %s ", cbe.ccid);
            return;
        }
        cbtable.delete(cbe);
        if( cbtable.size <= 0 ) {
            var unregister = { unregister: { events: [ { eventType: "CHAINCODE", chaincodeRegInfo:{ chaincodeID: cbe.ccid, eventName: "" }} ] }};
            this.chaincodeRegistrants.remove(cbe.ccid);
            this.call.write(unregister);
        }
    }

    public registerBlockEvent(callback:Function){
        if (!this.connected) return;
        this.blockRegistrants.add(callback);
        if(this.blockRegistrants.size==1) {
            var register = { register: { events: [ { eventType: "BLOCK"} ] }};
            this.call.write(register);
        }
    }

    public unregisterBlockEvent(callback:Function){
        if (!this.connected) return;
        if(this.blockRegistrants.size<=1) {
            var unregister = { unregister: { events: [ { eventType: "BLOCK"} ] }};
            this.call.write(unregister);
        }
        this.blockRegistrants.delete(callback);
    }

    public registerTxEvent(txid:string, callback:Function){
        debug("reg txid "+txid);
        this.txRegistrants.put(txid, callback);
    }

    public unregisterTxEvent(txid:string){
        this.txRegistrants.remove(txid);
    }

    private txCallback = (event) => {
        debug("txCallback event=%j", event);
        var eh = this;
        event.transactions.forEach(function(transaction) {
            debug("transaction.txid="+transaction.txid);
            var cb = eh.txRegistrants.get(transaction.txid);
            if (cb)
                cb(transaction.txid);
        });
    }
}
