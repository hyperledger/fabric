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

// requires
var debug = require('debug')('crypto');
var aesjs = require('aes-js');
var crypto = require('crypto');
var elliptic = require('elliptic');
var EC = elliptic.ec;
var sha3_256 = require('js-sha3').sha3_256;
var sha3_384 = require('js-sha3').sha3_384;
var sjcl = require('sjcl');
var util = require("util");
var jsrsa = require('jsrsasign');
var KEYUTIL = jsrsa.KEYUTIL;
var merge = require("node.extend");
var common = require("asn1js/org/pkijs/common");
var _asn1js = require("asn1js");
var _pkijs = require("pkijs");
var _x509schema = require("pkijs/org/pkijs/x509_schema");
var hashPrimitives = require(__dirname+"/hash");

// constants
const SHA2 = 'SHA2';
const SHA3 = 'SHA3';
export const TCertEncTCertIndex:string = "1.2.3.4.5.6.7";
const NonceSize:number = 24;
const AESKeyLength:number = 32;
const HMACKeyLength:number = 32;
const BlockSize:number = 16;

const GCMTagSize:number = 16;
const GCMStandardNonceSize:number = 12;

const ECIESKDFOutput = 512; // bits
const IVLength = 16; // bytes
const AESBlockLength = 16;

const CURVE_P_256_Size = 256;
const CURVE_P_384_Size = 384;

// variables
// #region Merging function/object declarations for ASN1js and PKIjs
var asn1js = merge(true, _asn1js, common);
var x509schema = merge(true, _x509schema, asn1js);
var pkijs_1 = merge(true, _pkijs, asn1js);
var pkijs = merge(true, pkijs_1, x509schema);

/**
 * The crypto class contains implementations of various crypto primitives.
 */
export class Crypto {

    private hashAlgorithm:string;
    private securityLevel:number;
    private curveName:string;
    private suite:string;
    private hashFunction:any;
    private hashFunctionKeyDerivation:any;
    private hashOutputSize:number;
    private ecdsaCurve:any;

    constructor(hashAlgorithm:string, securityLevel:number) {
        this.hashAlgorithm = hashAlgorithm;
        this.securityLevel = securityLevel;
        this.initialize()
    }

    /**
     * Get the security level
     * @returns The security level
     */
    getSecurityLevel():number {
        return this.securityLevel;
    }

    /**
     * Set the security level
     * @params securityLevel The security level
     */
    setSecurityLevel(securityLevel:number):void {
        this.checkSecurityLevel(securityLevel);

        this.securityLevel = securityLevel;
        this.initialize();
    }

    /**
     * Get the hash algorithm
     * @returns {string} The hash algorithm
     */
    getHashAlgorithm():string {
        return this.hashAlgorithm;
    }

    /**
     * Set the hash algorithm
     * @params hashAlgorithm The hash algorithm ('SHA2' or 'SHA3')
     */
    setHashAlgorithm(hashAlgorithm:string):void {
        this.checkHashFunction(hashAlgorithm);

        this.hashAlgorithm = hashAlgorithm;
        this.initialize();
    }

    generateNonce() {
        return crypto.randomBytes(NonceSize);
    }

    ecdsaKeyGen() {
        return KEYUTIL.generateKeypair("EC", this.curveName);
    };

    ecdsaKeyFromPrivate(key, encoding) {
        // select curve and hash algo based on level
        var keypair = new EC(this.ecdsaCurve).keyFromPrivate(key, encoding);
        debug('keypair: ', keypair);
        return keypair;
    };

    ecdsaKeyFromPublic(key, encoding) {
        var publicKey = new EC(this.ecdsaCurve).keyFromPublic(key, encoding);
        // debug('publicKey: [%j]', publicKey);
        return publicKey;
    };

    ecdsaSign(key:Buffer, msg:Buffer) {
        var ecdsa = new EC(this.ecdsaCurve);
        var signKey = ecdsa.keyFromPrivate(key, 'hex');
        var sig = ecdsa.sign(new Buffer(this.hashFunction(msg), 'hex'), signKey);
        debug('ecdsa signature: ', sig);
        return sig;
    };

    ecdsaPEMToPublicKey(chainKey:any) {
        // enrollChainKey is a PEM. Extract the key from it.
        var pem = new Buffer(chainKey, 'hex').toString();
        debug("ChainKey %s", pem);
        var chainKey = KEYUTIL.getHexFromPEM(pem, 'ECDSA PUBLIC KEY');
        // debug(chainKey);
        var certBuffer = _toArrayBuffer(new Buffer(chainKey, 'hex'));
        var asn1 = pkijs.org.pkijs.fromBER(certBuffer);
        // debug('asn1:\n', asn1);
        var cert;
        cert = new pkijs.org.pkijs.simpl.PUBLIC_KEY_INFO({schema: asn1.result});
        // debug('cert:\n', JSON.stringify(cert, null, 4));

        var ab = new Uint8Array(cert.subjectPublicKey.value_block.value_hex);
        var ecdsaChainKey = this.ecdsaKeyFromPublic(ab, 'hex');

        return ecdsaChainKey
    }

    ecdsaPrivateKeyToASN1(prvKeyHex:string): Buffer {
        var Ber = require('asn1').Ber;
        var sk = new Ber.Writer();
        sk.startSequence();
        sk.writeInt(1);
        sk.writeBuffer(new Buffer(prvKeyHex, 'hex'), 4);
        sk.writeByte(160);
        sk.writeByte(7);
        if (this.securityLevel == CURVE_P_384_Size ) {
            // OID of P384
            sk.writeOID('1.3.132.0.34');
        } else if (this.securityLevel == CURVE_P_256_Size) {
            // OID of P256
            sk.writeOID('1.2.840.10045.3.1.7');
        } else {
            throw Error("Not supported. Level " + this.securityLevel)
        }
        sk.endSequence();
        return sk.buffer;
    }

    eciesKeyGen() {
        return KEYUTIL.generateKeypair("EC", this.curveName);
    }

    eciesEncryptECDSA(ecdsaRecipientPublicKey, msg): Buffer {
        var self = this;
        var EC = elliptic.ec;
        //var curve = elliptic.curves['p'+level];
        var ecdsa = new EC('p' + self.securityLevel);

        // Generate ephemeral key-pair
        var ephKeyPair = KEYUTIL.generateKeypair("EC", this.curveName);
        var ephPrivKey = ecdsa.keyFromPrivate(ephKeyPair.prvKeyObj.prvKeyHex, 'hex');
        var Rb = ephKeyPair.pubKeyObj.pubKeyHex;

        // Derive a shared secret field element z from the ephemeral secret key k
        // and convert z to an octet string Z
        // debug("ecdsa.keyFromPublic=%s", util.inspect(ecdsaRecipientPublicKey));//XXX
        var Z = ephPrivKey.derive(ecdsaRecipientPublicKey.pub);
        // debug('[Z]: %j', Z);
        var kdfOutput = self.hkdf(Z.toArray(), ECIESKDFOutput, null, null);
        // debug('[kdfOutput]: %j', new Buffer(new Buffer(kdfOutput).toString('hex'), 'hex').toString('hex'));

        var aesKey = kdfOutput.slice(0, AESKeyLength);
        var hmacKey = kdfOutput.slice(AESKeyLength, AESKeyLength + HMACKeyLength);
        // debug('[Ek] ', new Buffer(aesKey, 'hex'));
        // debug('[Mk] ', new Buffer(hmacKey, 'hex'));

        var iv = crypto.randomBytes(IVLength);
        var cipher = crypto.createCipheriv('aes-256-cfb', new Buffer(aesKey), iv);
        // debug("MSG %j: ", msg);
        var encryptedBytes = cipher.update(msg);
        // debug("encryptedBytes: ",JSON.stringify(encryptedBytes));
        var EM = Buffer.concat([iv, encryptedBytes]);
        var D = self.hmac(hmacKey, EM);

        // debug('[Rb] ', new Buffer(Rb,'hex').toString('hex')+" len="+Rb.length);
        // debug('[EM] ', EM.toString('hex'));
        // debug('[D] ', new Buffer(D).toString('hex'));

        return Buffer.concat([new Buffer(Rb, 'hex'), EM, new Buffer(D)]);
    }

    eciesEncrypt(recipientPublicKey, msg) {
        var level = recipientPublicKey.ecparams.keylen;
        // debug("=============> %d", level);
        var EC = elliptic.ec;
        var curve = elliptic.curves["p" + level];
        // debug("=============> curve=%s", util.inspect(curve));
        var ecdsa = new EC(curve);

        return this.eciesEncryptECDSA(ecdsa.keyFromPublic(recipientPublicKey.pubKeyHex, 'hex'), msg)
    }

    eciesDecrypt(recipientPrivateKey, cipherText) {
        var self = this;
        // debug("recipientPrivateKey=%s", util.inspect(recipientPrivateKey));//XXX
        var level = recipientPrivateKey.ecparams.keylen;
        var curveName = recipientPrivateKey.curveName;
        // debug("=============> %d", level);
        if (this.securityLevel != level) {
            throw Error("Invalid key. It's security does not match the current security level " +  this.securityLevel + " " + level);
        }
        //cipherText = ephemeralPubKeyBytes + encryptedTokBytes + macBytes
        //ephemeralPubKeyBytes = first ((384+7)/8)*2 + 1 bytes = first 97 bytes
        //hmac is sha3_384 = 48 bytes or sha3_256 = 32 bytes
        var Rb_len = Math.floor((level + 7) / 8) * 2 + 1;
        var D_len = level >> 3;
        var ct_len = cipherText.length;

        if (ct_len <= Rb_len + D_len)
            throw new Error("Illegal cipherText length: " + ct_len + " must be > " + (Rb_len + D_len));

        var Rb = cipherText.slice(0, Rb_len);  // ephemeral public key bytes
        var EM = cipherText.slice(Rb_len, ct_len - D_len);  // encrypted content bytes
        var D = cipherText.slice(ct_len - D_len);

        // debug("Rb :\n", new Buffer(Rb).toString('hex'));
        // debug("EM :\n", new Buffer(EM).toString('hex'));
        // debug("D  :\n", new Buffer(D).toString('hex'));

        var EC = elliptic.ec;
        //var curve = elliptic.curves['p'+level];
        var ecdsa = new EC('p' + level);

        //convert bytes to usable key object
        var ephPubKey = ecdsa.keyFromPublic(new Buffer(Rb, 'hex'), 'hex');
        //var encPrivKey = ecdsa.keyFromPrivate(ecKeypair2.prvKeyObj.prvKeyHex, 'hex');
        var privKey = ecdsa.keyFromPrivate(recipientPrivateKey.prvKeyHex, 'hex');
        // debug('computing Z...', privKey, ephPubKey);

        var Z = privKey.derive(ephPubKey.pub);
        // debug('Z computed', Z);
        // debug('secret:  ', new Buffer(Z.toArray(), 'hex'));
        var kdfOutput = self.hkdf(Z.toArray(), ECIESKDFOutput, null, null);
        var aesKey = kdfOutput.slice(0, AESKeyLength);
        var hmacKey = kdfOutput.slice(AESKeyLength, AESKeyLength + HMACKeyLength);
        // debug('secret:  ', new Buffer(Z.toArray(), 'hex'));
        // debug('aesKey:  ', new Buffer(aesKey, 'hex'));
        // debug('hmacKey: ', new Buffer(hmacKey, 'hex'));

        var recoveredD = self.hmac(hmacKey, EM);
        debug('recoveredD:  ', new Buffer(recoveredD).toString('hex'));

        if (D.compare(new Buffer(recoveredD)) != 0) {
            // debug("D="+D.toString('hex')+" vs "+new Buffer(recoveredD).toString('hex'));
            throw new Error("HMAC verify failed");
        }
        var iv = EM.slice(0, IVLength);
        var cipher = crypto.createDecipheriv('aes-256-cfb', new Buffer(aesKey), iv);
        var decryptedBytes = cipher.update(EM.slice(IVLength));
        // debug("decryptedBytes: ",new Buffer(decryptedBytes).toString('hex'));
        return decryptedBytes;
    }

    aesKeyGen() {
        return crypto.randomBytes(AESKeyLength);
    }

    aesCFBDecryt(key, encryptedBytes) {

        var iv = crypto.randomBytes(IVLength);
        var aes = new aesjs.ModeOfOperation.cfb(key, iv, IVLength);

        debug("encryptedBytes: ", encryptedBytes);

        //need to pad encryptedBytes to multiples of 16
        var numMissingBytes = IVLength - (encryptedBytes.length % AESBlockLength);
        debug("missingBytes: ", numMissingBytes);

        if (numMissingBytes > 0) {
            encryptedBytes = Buffer.concat([encryptedBytes, new Buffer(numMissingBytes)]);
        }

        debug("encryptedBytes: ", encryptedBytes);

        var decryptedBytes = aes.decrypt(encryptedBytes);

        return decryptedBytes.slice(IVLength, decryptedBytes.length - numMissingBytes);

    }

    aesCBCPKCS7Decrypt(key, bytes) {

        var decryptedBytes, unpaddedBytes;

        decryptedBytes = this.CBCDecrypt(key, bytes);
        unpaddedBytes = this.PKCS7UnPadding(decryptedBytes);

        return unpaddedBytes;
    };

    aes256GCMDecrypt(key:Buffer, ct:Buffer) {
        let decipher = crypto.createDecipheriv('aes-256-gcm', key, ct.slice(0, GCMStandardNonceSize));
        decipher.setAuthTag(ct.slice(ct.length - GCMTagSize));
        let dec = decipher.update(
            ct.slice(GCMStandardNonceSize, ct.length - GCMTagSize).toString('hex'),
            'hex', 'hex'
        );
        dec += decipher.final('hex');
        return dec;
    }

    hkdf(ikm, keyBitLength, salt, info) {

        if (!salt)
            salt = _zeroBuffer(this.hashOutputSize);

        if (!info)
            info = "";

        var key = this.hkdf2(bytesToBits(new Buffer(ikm)), keyBitLength, bytesToBits(salt), info, this.hashFunctionKeyDerivation);

        return bitsToBytes(key);

    }

    hmac(key, bytes) {
        var self = this;
        debug('key: ', JSON.stringify(key));
        debug('bytes: ', JSON.stringify(bytes));

        var hmac = new sjcl.misc.hmac(bytesToBits(key), this.hashFunctionKeyDerivation);
        hmac.update(bytesToBits(bytes));
        var result = hmac.digest();
        debug("result: ", bitsToBytes(result));
        return bitsToBytes(result);
    }

    hmacAESTruncated(key, bytes) {
        var res = this.hmac(key, bytes);
        return res.slice(0, AESKeyLength);
    }

    hash(bytes) {
        debug('bytes: ', JSON.stringify(bytes));
        return this.hashFunction(bytes);
    }

    private checkSecurityLevel(securityLevel:number) {
        if (securityLevel != 256 && securityLevel != 384)
            throw new Error("Illegal level: " + this.securityLevel + " - must be either 256 or 384");
    }

    private checkHashFunction(hashAlgorithm: string) {
        if (!_isString(hashAlgorithm))
            throw new Error("Illegal Hash function family: " + hashAlgorithm + " - must be either SHA2 or SHA3");

        hashAlgorithm = hashAlgorithm.toUpperCase();
        if (hashAlgorithm != SHA2 && hashAlgorithm != SHA3)
            throw new Error("Illegal Hash function family: " + hashAlgorithm + " - must be either SHA2 or SHA3");
    }

    private initialize() {
        this.checkSecurityLevel(this.securityLevel);
        this.checkHashFunction(this.hashAlgorithm);

        this.suite = this.hashAlgorithm.toLowerCase() + '-' + this.securityLevel;
        if (this.securityLevel == CURVE_P_256_Size) {
            this.curveName = "secp256r1"
        } else if (this.securityLevel == CURVE_P_384_Size) {
            this.curveName = "secp384r1";
        }

        switch (this.suite) {
            case "sha3-256":
                debug("Using sha3-256");
                this.hashFunction = sha3_256;
                this.hashFunctionKeyDerivation = hashPrimitives.hash_sha3_256;
                this.hashOutputSize = 32;
                break;
            case "sha3-384":
                debug("Using sha3-384");
                this.hashFunction = sha3_384;
                this.hashFunctionKeyDerivation = hashPrimitives.hash_sha3_384;
                this.hashOutputSize = 48;
                break;
            case "sha2-256":
                debug("Using sha2-256");
                this.hashFunction = hashPrimitives.sha2_256;
                this.hashFunctionKeyDerivation = hashPrimitives.hash_sha2_256;
                this.hashOutputSize = 32;
                break;
        }

        switch (this.securityLevel) {
            case 256:
                this.ecdsaCurve = elliptic.curves['p256'];
                break;
            case 384:
                this.ecdsaCurve = elliptic.curves['p384'];
                break;
        }

    }

    /** HKDF with the specified hash function.
     * @param {bitArray} ikm The input keying material.
     * @param {Number} keyBitLength The output key length, in bits.
     * @param {String|bitArray} salt The salt for HKDF.
     * @param {String|bitArray} info The info for HKDF.
     * @param {Object} [Hash=sjcl.hash.sha256] The hash function to use.
     * @return {bitArray} derived key.
     */
    private hkdf2(ikm, keyBitLength, salt, info, Hash) {
        var hmac, key, i, hashLen, loops, curOut, ret = [];

        // Hash = Hash || sjcl.hash.sha256;
        if (typeof info === "string") {
            info = sjcl.codec.utf8String.toBits(info);
        } else if (!info) {
            info = sjcl.codec.utf8String.toBits('');
        }
        if (typeof salt === "string") {
            salt = sjcl.codec.utf8String.toBits(salt);
        } else if (!salt) {
            salt = [];
        }

        hmac = new sjcl.misc.hmac(salt, Hash);
        //key = hmac.mac(ikm);
        hmac.update(ikm);
        key = hmac.digest();
        // debug("prk: %j", new Buffer(bitsToBytes(key)).toString('hex'));
        hashLen = sjcl.bitArray.bitLength(key);

        loops = Math.ceil(keyBitLength / hashLen);
        if (loops > 255) {
            throw new sjcl.exception.invalid("key bit length is too large for hkdf");
        }

        curOut = [];
        for (i = 1; i <= loops; i++) {
            hmac = new sjcl.misc.hmac(key, Hash);
            hmac.update(curOut);
            hmac.update(info);
            // debug('sjcl.bitArray.partial(8, i): %j', sjcl.bitArray.partial(8, i));
            hmac.update(bytesToBits([i]));

            // hmac.update([sjcl.bitArray.partial(8, i)]);
            curOut = hmac.digest();
            ret = sjcl.bitArray.concat(ret, curOut);
        }
        return sjcl.bitArray.clamp(ret, keyBitLength);
    }

    private CBCDecrypt(key, bytes) {
        debug('key length: ', key.length);
        debug('bytes length: ', bytes.length);
        var iv = bytes.slice(0, BlockSize);
        debug('iv length: ', iv.length);
        var encryptedBytes = bytes.slice(BlockSize);
        debug('encrypted bytes length: ', encryptedBytes.length);

        var decryptedBlocks = [];
        var decryptedBytes;

        // CBC only works with 16 bytes blocks
        if (encryptedBytes.length > BlockSize) {
            //CBC only support cipertext with length Blocksize
            var start = 0;
            var end = BlockSize;
            while (end <= encryptedBytes.length) {
                var aesCbc = new aesjs.ModeOfOperation.cbc(key, iv);
                debug('start|end', start, end);
                var encryptedBlock = encryptedBytes.slice(start, end);
                var decryptedBlock = aesCbc.decrypt(encryptedBlock);
                debug('decryptedBlock: ', decryptedBlock);
                decryptedBlocks.push(decryptedBlock);
                //iv for next round equals previous block
                iv = encryptedBlock;
                start += BlockSize;
                end += BlockSize;
            }
            ;

            decryptedBytes = Buffer.concat(decryptedBlocks);
        }
        else {
            var aesCbc = new aesjs.ModeOfOperation.cbc(key, iv);
            decryptedBytes = aesCbc.decrypt(encryptedBytes);
        }

        debug('decrypted bytes: ', JSON.stringify(decryptedBytes));

        return decryptedBytes;

    };

    private PKCS7UnPadding(bytes) {

        //last byte is the number of padded bytes
        var padding = bytes.readUInt8(bytes.length - 1);
        debug('padding: ', padding);
        //should check padded bytes, but just going to extract
        var unpadded = bytes.slice(0, bytes.length - padding);
        debug('unpadded bytes: ', JSON.stringify(unpadded));
        return unpadded;
    };

}  // end Crypto class

export class X509Certificate {

    private _cert:any;

    constructor(buffer) {
        debug('cert:', JSON.stringify(buffer));
        // convert certBuffer to arraybuffer
        var certBuffer = _toArrayBuffer(buffer);
        // parse the DER-encoded buffer
        var asn1 = pkijs.org.pkijs.fromBER(certBuffer);
        this._cert = {};
        try {
            this._cert = new pkijs.org.pkijs.simpl.CERT({schema: asn1.result});
            debug('decoded certificate:\n', JSON.stringify(this._cert, null, 4));
        } catch (ex) {
            debug('error parsing certificate bytes: ', ex)
            throw ex;
        }
    }

    criticalExtension(oid) {
        var ext;
        debug('oid: ', oid);
        this._cert.extensions.some(function (extension) {
            debug('extnID: ', extension.extnID);
            if (extension.extnID === oid) {
                ext = extension;
                return true;
            }
        });
        debug('found extension: ', ext);
        debug('extValue: ', _toBuffer(ext.extnValue.value_block.value_hex));
        return _toBuffer(ext.extnValue.value_block.value_hex);
    }

} // end X509Certificate class

function bitsToBytes(arr) {
    var out = [], bl = sjcl.bitArray.bitLength(arr), i, tmp;
    for (i = 0; i < bl / 8; i++) {
        if ((i & 3) === 0) {
            tmp = arr[i / 4];
        }
        out.push(tmp >>> 24);
        tmp <<= 8;
    }
    return out;
}

/** Convert from an array of bytes to a bitArray. */
function bytesToBits(bytes) {
    var out = [], i, tmp = 0;
    for (i = 0; i < bytes.length; i++) {
        tmp = tmp << 8 | bytes[i];
        if ((i & 3) === 3) {
            out.push(tmp);
            tmp = 0;
        }
    }
    if (i & 3) {
        out.push(sjcl.bitArray.partial(8 * (i & 3), tmp));
    }
    return out;
}

function _zeroBuffer(length) {
    var buf = new Buffer(length);
    buf.fill(0);
    return buf
}

// utility function to convert Node buffers to Javascript arraybuffer
function _toArrayBuffer(buffer) {
    var ab = new ArrayBuffer(buffer.length);
    var view = new Uint8Array(ab);
    for (var i = 0; i < buffer.length; ++i) {
        view[i] = buffer[i];
    }
    return ab;
}

// utility function to convert Javascript arraybuffer to Node buffers
function _toBuffer(ab) {
    var buffer = new Buffer(ab.byteLength);
    var view = new Uint8Array(ab);
    for (var i = 0; i < buffer.length; ++i) {
        buffer[i] = view[i];
    }
    return buffer;
}

// Determine if an object is a string
function _isString(obj:any):boolean {
    return (typeof obj === 'string' || obj instanceof String);
}
