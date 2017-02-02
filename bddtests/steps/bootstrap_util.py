# Copyright IBM Corp. 2016 All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

import time
import sys
import hashlib
if sys.version_info < (3, 6):
    import sha3

from OpenSSL import crypto
from OpenSSL import rand
import ecdsa
import shutil

from slugify import slugify

from collections import namedtuple
from enum import Enum

from google.protobuf import timestamp_pb2
from common import common_pb2 as common_dot_common_pb2
from common import configuration_pb2 as common_dot_configuration_pb2
from common import msp_principal_pb2
from msp import mspconfig_pb2
from peer import configuration_pb2 as peer_dot_configuration_pb2

# import orderer
from orderer import configuration_pb2 as orderer_dot_configuration_pb2
import orderer_util

import os
import re
import shutil
import compose
import uuid

# Type to represent tuple of user, nodeName, ogranization
NodeAdminTuple = namedtuple("NodeAdminTuple", ['user', 'nodeName', 'organization'])

class ContextHelper:

    def __init__(self, context):
        self.context = context
        self.guuid = GetUUID()

    def getGuuid(self):
        return self.guuid

    def getTmpPath(self):
        pathToReturn = "tmp"
        if not os.path.isdir(pathToReturn):
            os.makedirs(pathToReturn)
        return pathToReturn

    def getCachePath(self):
        pathToReturn = os.path.join(self.getTmpPath(), "cache")
        if not os.path.isdir(pathToReturn):
            os.makedirs(pathToReturn)
        return pathToReturn


    def getTmpProjectPath(self):
        pathToReturn = os.path.join(self.getTmpPath(), self.guuid)
        if not os.path.isdir(pathToReturn):
            os.makedirs(pathToReturn)
        return pathToReturn

    def getTmpPathForName(self, name, copyFromCache=False):
        'Returns the tmp path for a file, and a flag indicating if the file exists. Will also check in the cache and copy to tmp if copyFromCache==True'
        slugifiedName = slugify(name)
        tmpPath = os.path.join(self.getTmpProjectPath(), slugifiedName)
        fileExists = False
        if os.path.isfile(tmpPath):
            # file already exists in tmp path, return path and exists flag
            fileExists = True
        elif copyFromCache:
            # See if the file exists in cache, and copy over to project folder.
            cacheFilePath = os.path.join(self.getCachePath(), slugifiedName)
            if os.path.isfile(cacheFilePath):
                shutil.copy(cacheFilePath, tmpPath)
                fileExists = True
        return (tmpPath, fileExists)

    def copyToCache(self, name):
        srcPath, fileExists = self.getTmpPathForName(name, copyFromCache=False)
        assert fileExists, "Tried to copy source file to cache, but file not found for: {0}".format(srcPath)
        # Now copy to the cache if it does not already exist
        cacheFilePath = os.path.join(self.getCachePath(), slugify(name))
        if not os.path.isfile(cacheFilePath):
            shutil.copy(srcPath, cacheFilePath)


    def isConfigEnabled(self, configName):
        return self.context.config.userdata.get(configName, "false") == "true"

    @classmethod
    def GetHelper(cls, context):
        if not "contextHelper" in context:
            context.contextHelper = ContextHelper(context)
        return context.contextHelper

def GetUUID():
    return compose.Composition.GetUUID()


def createRSAKey():
    #Create RSA key, 2048 bit
    pk = crypto.PKey()
    pk.generate_key(crypto.TYPE_RSA,2048)
    assert pk.check()==True
    return pk

def createECDSAKey(curve=ecdsa.NIST256p):
    #Create ECDSA key
    sk = ecdsa.SigningKey.generate(curve=curve)
    return sk


def computeCryptoHash(data):
    ' This will currently return 128 hex characters'
    # s = hashlib.sha3_256()
    s = hashlib.shake_256()
    s.update(data)
    return  s.digest(64)


def createCertRequest(pkey, digest="sha256", **name):
    """
    Create a certificate request.
    Arguments: pkey   - The key to associate with the request
               digest - Digestion method to use for signing, default is sha256
               **name - The name of the subject of the request, possible
                        arguments are:
                          C     - Country name
                          ST    - State or province name
                          L     - Locality name
                          O     - Organization name
                          OU    - Organizational unit name
                          CN    - Common name
                          emailAddress - E-mail address
    Returns:   The certificate request in an X509Req object
    """
    req = crypto.X509Req()
    subj = req.get_subject()

    for key, value in name.items():
        setattr(subj, key, value)

    req.set_pubkey(pkey)
    req.sign(pkey, digest)
    return req

def createCertificate(req, issuerCertKey, serial, validityPeriod, digest="sha256"):
    """
    Generate a certificate given a certificate request.
    Arguments: req        - Certificate request to use
               issuerCert - The certificate of the issuer
               issuerKey  - The private key of the issuer
               serial     - Serial number for the certificate
               notBefore  - Timestamp (relative to now) when the certificate
                            starts being valid
               notAfter   - Timestamp (relative to now) when the certificate
                            stops being valid
               digest     - Digest method to use for signing, default is sha256
    Returns:   The signed certificate in an X509 object
    """
    issuerCert, issuerKey = issuerCertKey
    notBefore, notAfter = validityPeriod
    cert = crypto.X509()
    cert.set_serial_number(serial)
    cert.gmtime_adj_notBefore(notBefore)
    cert.gmtime_adj_notAfter(notAfter)
    cert.set_issuer(issuerCert.get_subject())
    cert.set_subject(req.get_subject())
    cert.set_pubkey(req.get_pubkey())
    cert.sign(issuerKey, digest)
    return cert

#SUBJECT_DEFAULT = {countryName : "US", stateOrProvinceName : "NC", localityName : "RTP", organizationName : "IBM", organizationalUnitName : "Blockchain"}

class Entity:

    def __init__(self, name):
        self.name = name
        #Create a ECDSA key, then a crypto pKey from the DER for usage with cert requests, etc.
        self.ecdsaSigningKey = createECDSAKey()
        self.pKey = crypto.load_privatekey(crypto.FILETYPE_ASN1, self.ecdsaSigningKey.to_der())
        # Signing related ecdsa config
        self.hashfunc = hashlib.sha256
        self.sigencode=ecdsa.util.sigencode_der_canonize
        self.sigdecode=ecdsa.util.sigdecode_der


    def createCertRequest(self, nodeName):
        req = createCertRequest(self.pKey, CN=nodeName)
        #print("request => {0}".format(crypto.dump_certificate_request(crypto.FILETYPE_PEM, req)))
        return req

    def computeHash(self, data):
        s = self.hashfunc()
        s.update(data)
        return s.digest()

    def sign(self, dataAsBytearray):
        return self.ecdsaSigningKey.sign(dataAsBytearray, hashfunc=self.hashfunc, sigencode=self.sigencode)

    def verifySignature(self, signature, signersCert, data):
        'Will verify the signature of an entity based upon public cert'
        vk = ecdsa.VerifyingKey.from_der(crypto.dump_publickey(crypto.FILETYPE_ASN1, signersCert.get_pubkey()))
        assert vk.verify(signature, data, hashfunc=self.hashfunc, sigdecode=self.sigdecode), "Invalid signature!!"


class User(Entity, orderer_util.UserRegistration):
    def __init__(self, name):
        Entity.__init__(self, name)
        orderer_util.UserRegistration.__init__(self, name)
        self.tags = {}

class Network(Enum):
    Orderer = 1
    Peer = 2

class Organization(Entity):

    def __init__(self, name):
        Entity.__init__(self, name)
        req = createCertRequest(self.pKey, CN=name)
        numYrs = 1
        self.signedCert = createCertificate(req, (req, self.pKey), 1000, (0, 60*60*24*365*numYrs))
        # Which networks this organization belongs to
        self.networks = []

    def getSelfSignedCert(self):
        return self.signedCert

    def getMSPConfig(self):
        certPemsList = [crypto.dump_certificate(crypto.FILETYPE_PEM, self.getSelfSignedCert())]
        # For now, admin certs and CA certs are the same per @ASO
        adminCerts = certPemsList
        cacerts = adminCerts
        # Currently only 1 component, CN=<orgName>
        # name = self.getSelfSignedCert().get_subject().getComponents()[0][1]
        name = self.name
        fabricMSPConfig = mspconfig_pb2.FabricMSPConfig(Admins=adminCerts, RootCerts=cacerts, Name=name)
        mspConfig = mspconfig_pb2.MSPConfig(Config=fabricMSPConfig.SerializeToString(), Type=0)
        return mspConfig

    pass

    def createCertificate(self, certReq):
        numYrs = 1
        return createCertificate(certReq, (self.signedCert, self.pKey), 1000, (0, 60*60*24*365*numYrs))

    def addToNetwork(self, network):
        'Used to track which network this organization is defined in.'
        assert network in Network, 'Network not recognized ({0}), expected to be one of ({1})'.format(network, list(Network))
        if not network in self.networks:
            self.networks.append(network)


class Directory:

    def __init__(self):
        self.organizations = {}
        self.users = {}
        self.ordererAdminTuples = {}

    def registerOrg(self, orgName, network):
        assert orgName not in self.organizations, "Organization already registered {0}".format(orgName)
        self.organizations[orgName] = Organization(orgName)
        return self.organizations[orgName]

    def registerUser(self, userName):
        assert userName not in self.users, "User already registered {0}".format(userName)
        self.users[userName] = User(userName)
        return self.users[userName]

    def getUser(self, userName, shouldCreate = False):
        if not userName in self.users and shouldCreate:
            self.users[userName] = User(userName)
        return self.users[userName]

    def getOrganization(self, orgName, shouldCreate = False):
        if not orgName in self.organizations and shouldCreate:
            self.organizations[orgName] = Organization(orgName)
        return self.organizations[orgName]

    def findCertByTuple(self, userName, contextName, orgName):
        ordererAdminTuple = NodeAdminTuple(user = userName, nodeName = contextName, organization = orgName)
        return self.ordererAdminTuples[ordererAdminTuple]

    def findCertForNodeAdminTuple(self, nodeAdminTuple):
        assert nodeAdminTuple in self.ordererAdminTuples, "Node admin tuple not found for: {0}".format(nodeAdminTuple)
        return self.ordererAdminTuples[nodeAdminTuple]

    def findNodeAdminTuple(self, userName, contextName, orgName):
        nodeAdminTuple = NodeAdminTuple(user = userName, nodeName = contextName, organization = orgName)
        assert nodeAdminTuple in self.ordererAdminTuples, "Node admin tuple not found for: {0}".format(nodeAdminTuple)
        return nodeAdminTuple

    def registerOrdererAdminTuple(self, userName, ordererName, organizationName):
        ' Assign the user as orderer admin'
        ordererAdminTuple = NodeAdminTuple(user = userName, nodeName = ordererName, organization = organizationName)
        assert ordererAdminTuple not in self.ordererAdminTuples, "Orderer admin tuple already registered {0}".format(ordererAdminTuple)
        assert organizationName in self.organizations, "Orderer Organization not defined {0}".format(organizationName)

        user = self.getUser(userName, shouldCreate = True)
        certReq = user.createCertRequest(ordererAdminTuple.nodeName)
        userCert = self.getOrganization(organizationName).createCertificate(certReq)

        # Verify the newly created certificate
        store = crypto.X509Store()
        # Assuming a list of trusted certs
        for trustedCert in [self.getOrganization(organizationName).signedCert]:
            store.add_cert(trustedCert)
        # Create a certificate context using the store and the certificate to verify
        store_ctx = crypto.X509StoreContext(store, userCert)
        # Verify the certificate, returns None if it can validate the certificate
        store_ctx.verify_certificate()
        self.ordererAdminTuples[ordererAdminTuple] = userCert
        return ordererAdminTuple


class AuthDSLHelper:

    @classmethod
    def Envelope(cls, signaturePolicy, identities):
        'Envelope builds an envelope message embedding a SignaturePolicy'
        return common_dot_configuration_pb2.SignaturePolicyEnvelope(
            Version=0,
            Policy=signaturePolicy,
            Identities=identities)

    @classmethod
    def NOutOf(cls, n, policies):
        'NOutOf creates a policy which requires N out of the slice of policies to evaluate to true'
        return common_dot_configuration_pb2.SignaturePolicy(
            From=common_dot_configuration_pb2.SignaturePolicy.NOutOf(
                N=n,
                Policies=policies,
            ),
        )



class BootstrapHelper:
    KEY_CONSENSUS_TYPE = "ConsensusType"
    KEY_CHAIN_CREATION_POLICY_NAMES = "ChainCreationPolicyNames"
    KEY_ACCEPT_ALL_POLICY = "AcceptAllPolicy"
    KEY_INGRESS_POLICY = "IngressPolicyNames"
    KEY_EGRESS_POLICY = "EgressPolicyNames"
    KEY_HASHING_ALGORITHM = "HashingAlgorithm"
    KEY_BATCH_SIZE = "BatchSize"
    KEY_BATCH_TIMEOUT = "BatchTimeout"
    KEY_CREATIONPOLICY = "CreationPolicy"
    KEY_MSP_INFO = "MSP"
    KEY_ANCHOR_PEERS = "AnchorPeers"

    KEY_NEW_CONFIGURATION_ITEM_POLICY = "NewConfigurationItemPolicy"
    DEFAULT_CHAIN_CREATORS = [KEY_ACCEPT_ALL_POLICY]

    DEFAULT_NONCE_SIZE = 24

    def __init__(self, chainId = "TestChain", lastModified = 0, msgVersion = 1, epoch = 0, consensusType = "solo", batchSize = 10, batchTimeout="10s", absoluteMaxBytes=100000000, preferredMaxBytes=512*1024, signers=[]):
        self.chainId = str(chainId)
        self.lastModified = lastModified
        self.msgVersion = msgVersion
        self.epoch = epoch
        self.consensusType = consensusType
        self.batchSize = batchSize
        self.batchTimeout = batchTimeout
        self.absoluteMaxBytes = absoluteMaxBytes
        self.preferredMaxBytes = preferredMaxBytes
        self.signers = signers

    @classmethod
    def getNonce(cls):
        return rand.bytes(BootstrapHelper.DEFAULT_NONCE_SIZE)

    @classmethod
    def addSignatureToSignedConfigItem(cls, signedConfigItem, (entity, cert)):
        sigHeader = common_dot_common_pb2.SignatureHeader(creator=crypto.dump_certificate(crypto.FILETYPE_ASN1,cert),nonce=BootstrapHelper.getNonce())
        sigHeaderBytes = sigHeader.SerializeToString()
        # Signature over the concatenation of configurationItem bytes and signatureHeader bytes
        signature = entity.sign(signedConfigItem.ConfigurationItem + sigHeaderBytes)
        # Now add new signature to Signatures repeated field
        newConfigSig = signedConfigItem.Signatures.add()
        newConfigSig.signatureHeader=sigHeaderBytes
        newConfigSig.signature=signature


    def makeChainHeader(self, type = common_dot_common_pb2.HeaderType.Value("CONFIGURATION_ITEM"), txID = "", extension='', version = 1,
                        timestamp = timestamp_pb2.Timestamp(seconds = int(time.time()), nanos = 0)):
        return common_dot_common_pb2.ChainHeader(type = type,
                                      version = version,
                                      timestamp = timestamp,
                                      chainID = self.chainId,
                                      epoch = self.epoch,
                                      txID = txID,
                                      extension = extension)
    def makeSignatureHeader(self, serializeCertChain, nonce):
        return common_dot_common_pb2.SignatureHeader(creator = serializeCertChain,
                                                     nonce = nonce)

    def signConfigItem(self, configItem):
        signedConfigItem = common_dot_configuration_pb2.SignedConfigurationItem(ConfigurationItem=configItem.SerializeToString(), Signatures=None)
        return signedConfigItem

    def getConfigItem(self, commonConfigType, key, value):
        configItem = common_dot_configuration_pb2.ConfigurationItem(
            Header=self.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value("CONFIGURATION_ITEM")),
            Type=commonConfigType,
            LastModified=self.lastModified,
            ModificationPolicy=BootstrapHelper.KEY_NEW_CONFIGURATION_ITEM_POLICY,
            Key=key,
            Value=value)
        return configItem


    def encodeAnchorInfo(self, ciValue):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Peer"),
            key=BootstrapHelper.KEY_ANCHOR_PEERS,
            value=ciValue.SerializeToString())
        return self.signConfigItem(configItem)

    def encodeMspInfo(self, mspUniqueId, ciValue):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("MSP"),
            key=mspUniqueId,
            value=ciValue.SerializeToString())
        return self.signConfigItem(configItem)

    def encodeHashingAlgorithm(self, hashingAlgorithm="SHAKE256"):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Chain"),
            key=BootstrapHelper.KEY_HASHING_ALGORITHM,
            value=common_dot_configuration_pb2.HashingAlgorithm(name=hashingAlgorithm).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeBatchSize(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_BATCH_SIZE,
            value=orderer_dot_configuration_pb2.BatchSize(maxMessageCount=self.batchSize, absoluteMaxBytes=self.absoluteMaxBytes, preferredMaxBytes=self.preferredMaxBytes).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeBatchTimeout(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_BATCH_TIMEOUT,
            value=orderer_dot_configuration_pb2.BatchTimeout(timeout=self.batchTimeout).SerializeToString())
        return self.signConfigItem(configItem)

    def  encodeConsensusType(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_CONSENSUS_TYPE,
            value=orderer_dot_configuration_pb2.ConsensusType(type=self.consensusType).SerializeToString())
        return self.signConfigItem(configItem)

    def  encodeChainCreators(self, ciValue = orderer_dot_configuration_pb2.ChainCreationPolicyNames(names=DEFAULT_CHAIN_CREATORS).SerializeToString()):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_CHAIN_CREATION_POLICY_NAMES,
            value=ciValue)
        return self.signConfigItem(configItem)

    def encodePolicy(self, key, policy=common_dot_configuration_pb2.Policy(type=common_dot_configuration_pb2.Policy.PolicyType.Value("SIGNATURE"), policy=AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(0,[]), identities=[]).SerializeToString())):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Policy"),
            key=key,
            value=policy.SerializeToString())
        return self.signConfigItem(configItem)


    def  encodeEgressPolicy(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_EGRESS_POLICY,
            value=orderer_dot_configuration_pb2.EgressPolicyNames(names=[BootstrapHelper.KEY_ACCEPT_ALL_POLICY]).SerializeToString())
        return self.signConfigItem(configItem)

    def  encodeIngressPolicy(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_INGRESS_POLICY,
            value=orderer_dot_configuration_pb2.IngressPolicyNames(names=[BootstrapHelper.KEY_ACCEPT_ALL_POLICY]).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeAcceptAllPolicy(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Policy"),
            key=BootstrapHelper.KEY_ACCEPT_ALL_POLICY,
            value=common_dot_configuration_pb2.Policy(type=1, policy=AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(0,[]), identities=[]).SerializeToString()).SerializeToString())
        return self.signConfigItem(configItem)

    def lockDefaultModificationPolicy(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Policy"),
            key=BootstrapHelper.KEY_NEW_CONFIGURATION_ITEM_POLICY,
            value=common_dot_configuration_pb2.Policy(type=1, policy=AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(1,[]), identities=[]).SerializeToString()).SerializeToString())
        return self.signConfigItem(configItem)

    def computeBlockDataHash(self, blockData):
        return computeCryptoHash(blockData.SerializeToString())


    def signInitialChainConfig(self, signedConfigItems, chainCreationPolicyName):
        'Create a signedConfigItem using previous config items'
        # Create byte array to store concatenated bytes
        # concatenatedConfigItemsBytes = bytearray()
        # for sci in signedConfigItems:
        #     concatenatedConfigItemsBytes = concatenatedConfigItemsBytes + bytearray(sci.ConfigurationItem)
        # hash = computeCryptoHash(concatenatedConfigItemsBytes)
        data = ''
        for sci in signedConfigItems:
            data = data + sci.ConfigurationItem
        # Compute hash over concatenated bytes
        hash = computeCryptoHash(data)
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_CREATIONPOLICY,
            value=orderer_dot_configuration_pb2.CreationPolicy(policy=chainCreationPolicyName, digest=hash).SerializeToString())
        return [self.signConfigItem(configItem)] + signedConfigItems


def signInitialChainConfig(signedConfigItems, chainId, chainCreationPolicyName):
    bootstrapHelper = BootstrapHelper(chainId = chainId)
    # Returns a list prepended with a signedConfiguration
    signedConfigItems = bootstrapHelper.signInitialChainConfig(signedConfigItems, chainCreationPolicyName)
    return common_dot_configuration_pb2.ConfigurationEnvelope(Items=signedConfigItems)

def getDirectory(context):
    if 'bootstrapDirectory' not in context:
        context.bootstrapDirectory = Directory()
    return context.bootstrapDirectory

def getOrdererBootstrapAdmin(context, shouldCreate=False):
    directory = getDirectory(context)
    ordererBootstrapAdmin = directory.getUser(userName="ordererBootstrapAdmin", shouldCreate=shouldCreate)
    return ordererBootstrapAdmin

def addOrdererBootstrapAdminOrgReferences(context, policyName, orgNames):
    'Adds a key/value pair of policyName/[orgName,...]'
    directory = getDirectory(context)
    ordererBootstrapAdmin = directory.getUser(userName="ordererBootstrapAdmin", shouldCreate=False)
    if not 'OrgReferences' in ordererBootstrapAdmin.tags:
        ordererBootstrapAdmin.tags['OrgReferences'] = {}
    policyNameToOrgNamesDict = ordererBootstrapAdmin.tags['OrgReferences']
    assert not policyName in policyNameToOrgNamesDict, "PolicyName '{0}' already registered with ordererBootstrapAdmin".format(policyName)
    policyNameToOrgNamesDict[policyName] = orgNames
    return policyNameToOrgNamesDict

def getOrdererBootstrapAdminOrgReferences(context):
    directory = getDirectory(context)
    ordererBootstrapAdmin = directory.getUser(userName="ordererBootstrapAdmin", shouldCreate=False)
    if not 'OrgReferences' in ordererBootstrapAdmin.tags:
        ordererBootstrapAdmin.tags['OrgReferences'] = {}
    return ordererBootstrapAdmin.tags['OrgReferences']

def getSignedMSPConfigItems(context, chainId, orgNames):
    directory = getDirectory(context)
    bootstrapHelper =  BootstrapHelper(chainId=chainId)
    orgs = [directory.getOrganization(orgName) for orgName in orgNames]
    mspSignedConfigItems = [bootstrapHelper.encodeMspInfo(org.name, org.getMSPConfig()) for org in orgs]
    return mspSignedConfigItems

def getSignedAnchorConfigItems(context, chainId, nodeAdminTuples):
    directory = getDirectory(context)
    bootstrapHelper =  BootstrapHelper(chainId=chainId)

    anchorPeers = peer_dot_configuration_pb2.AnchorPeers()
    for nodeAdminTuple in nodeAdminTuples:
        anchorPeer = anchorPeers.anchorPeers.add()
        anchorPeer.Host=nodeAdminTuple.nodeName
        anchorPeer.Port=5611
        anchorPeer.Cert=crypto.dump_certificate(crypto.FILETYPE_PEM, directory.findCertForNodeAdminTuple(nodeAdminTuple))

    anchorsSignedConfigItems = [bootstrapHelper.encodeAnchorInfo(anchorPeers)]
    return anchorsSignedConfigItems


def getMspConfigItemsForPolicyNames(context, chainId, policyNames):
    directory = getDirectory(context)
    ordererBootstrapAdmin = getOrdererBootstrapAdmin(context)
    policyNameToOrgNamesDict = getOrdererBootstrapAdminOrgReferences(context)
    # Get unique set of org names and return set of signed MSP ConfigItems
    orgNamesReferenced = list(set([orgName for policyName in policyNames for orgName in policyNameToOrgNamesDict[policyName]]))
    orgNamesReferenced.sort()
    return getSignedMSPConfigItems(context=context, chainId=chainId, orgNames=orgNamesReferenced)



def createSignedConfigItems(context, chainId, consensusType, signedConfigItems = []):
    # directory = getDirectory(context)
    # assert len(directory.ordererAdminTuples) > 0, "No orderer admin tuples defined!!!"
    bootstrapHelper = BootstrapHelper(chainId = chainId, consensusType=consensusType)
    configItems = signedConfigItems
    configItems.append(bootstrapHelper.encodeHashingAlgorithm())

    # configItems.append(bootstrapHelper.encodeBlockDataHashingStructure())
    # configItems.append(bootstrapHelper.encodeOrdererAddresses())

    configItems.append(bootstrapHelper.encodeBatchSize())
    configItems.append(bootstrapHelper.encodeBatchTimeout())
    configItems.append(bootstrapHelper.encodeConsensusType())
    configItems.append(bootstrapHelper.encodeAcceptAllPolicy())
    configItems.append(bootstrapHelper.encodeIngressPolicy())
    configItems.append(bootstrapHelper.encodeEgressPolicy())
    configItems.append(bootstrapHelper.lockDefaultModificationPolicy())
    return configItems


def createConfigTxEnvelope(chainId, signedConfigEnvelope):
    bootstrapHelper = BootstrapHelper(chainId = chainId)
    payloadChainHeader = bootstrapHelper.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value("CONFIGURATION_TRANSACTION"))

    #Now the SignatureHeader
    serializedCreatorCertChain = None
    nonce = None
    payloadSignatureHeader = common_dot_common_pb2.SignatureHeader(
        creator=serializedCreatorCertChain,
        nonce=bootstrapHelper.getNonce(),
    )

    payloadHeader = common_dot_common_pb2.Header(
        chainHeader=payloadChainHeader,
        signatureHeader=payloadSignatureHeader,
    )
    payload = common_dot_common_pb2.Payload(header=payloadHeader, data=signedConfigEnvelope.SerializeToString())
    envelope = common_dot_common_pb2.Envelope(payload=payload.SerializeToString(), signature=None)
    return envelope

def createGenesisBlock(context, chainId, consensusType, signedConfigItems = []):
    'Generates the genesis block for starting the oderers and for use in the chain config transaction by peers'
    #assert not "bootstrapGenesisBlock" in context,"Genesis block already created:\n{0}".format(context.bootstrapGenesisBlock)
    directory = getDirectory(context)
    assert len(directory.ordererAdminTuples) > 0, "No orderer admin tuples defined!!!"

    configItems = createSignedConfigItems(context, chainId, consensusType, signedConfigItems = signedConfigItems)

    bootstrapHelper = BootstrapHelper(chainId = chainId, consensusType=consensusType)
    configEnvelope = common_dot_configuration_pb2.ConfigurationEnvelope(Items=configItems)

    envelope = createConfigTxEnvelope(chainId, configEnvelope)

    blockData = common_dot_common_pb2.BlockData(Data=[envelope.SerializeToString()])


    # Spoke with kostas, for orderer in general
    signaturesMetadata = ""
    lastConfigurationBlockMetadata = common_dot_common_pb2.Metadata(value=common_dot_common_pb2.LastConfiguration(index=0).SerializeToString()).SerializeToString()
    ordererConfigMetadata = ""
    transactionFilterMetadata = ""
    block = common_dot_common_pb2.Block(
        Header=common_dot_common_pb2.BlockHeader(
            Number=0,
            PreviousHash=None,
            DataHash=bootstrapHelper.computeBlockDataHash(blockData),
        ),
        Data=blockData,
        Metadata=common_dot_common_pb2.BlockMetadata(Metadata=[signaturesMetadata,lastConfigurationBlockMetadata,transactionFilterMetadata, ordererConfigMetadata]),
    )

    # Add this back once crypto certs are required
    for nodeAdminTuple in directory.ordererAdminTuples:
        userCert = directory.ordererAdminTuples[nodeAdminTuple]
        certAsPEM = crypto.dump_certificate(crypto.FILETYPE_PEM, userCert)
        # print("UserCert for orderer genesis:\n{0}\n".format(certAsPEM))
        # print("")

    return (block, envelope)

class PathType(Enum):
    'Denotes whether Path relative to Local filesystem or Containers volume reference.'
    Local = 1
    Container = 2


class OrdererGensisBlockCompositionCallback(compose.CompositionCallback):
    'Responsible for setting the GensisBlock for the Orderer nodes upon composition'

    def __init__(self, context, genesisBlock, genesisFileName = "genesis_file"):
        self.context = context
        self.genesisFileName = genesisFileName
        self.genesisBlock = genesisBlock
        self.volumeRootPathInContainer="/var/hyperledger/bddtests"
        compose.Composition.RegisterCallbackInContext(context, self)

    def getVolumePath(self, composition, pathType=PathType.Local):
        assert pathType in PathType, "Expected pathType of {0}".format(PathType)
        basePath = "."
        if pathType == PathType.Container:
            basePath = self.volumeRootPathInContainer
        return "{0}/volumes/orderer/{1}".format(basePath, composition.projectName)

    def getGenesisFilePath(self, composition, pathType=PathType.Local):
        return "{0}/{1}".format(self.getVolumePath(composition, pathType), self.genesisFileName)

    def composing(self, composition, context):
        print("Will copy gensisiBlock over at this point ")
        os.makedirs(self.getVolumePath(composition))
        with open(self.getGenesisFilePath(composition), "wb") as f:
            f.write(self.genesisBlock.SerializeToString())

    def decomposing(self, composition, context):
        'Will remove the orderer volume path folder for the context'
        shutil.rmtree(self.getVolumePath(composition))

    def getEnv(self, composition, context, env):
        env["ORDERER_GENERAL_GENESISMETHOD"]="file"
        env["ORDERER_GENERAL_GENESISFILE"]=self.getGenesisFilePath(composition, pathType=PathType.Container)


class PeerCompositionCallback(compose.CompositionCallback):
    'Responsible for setting up Peer nodes upon composition'

    def __init__(self, context):
        self.context = context
        self.volumeRootPathInContainer="/var/hyperledger/bddtests"
        compose.Composition.RegisterCallbackInContext(context, self)

    def getVolumePath(self, composition, pathType=PathType.Local):
        assert pathType in PathType, "Expected pathType of {0}".format(PathType)
        basePath = "."
        if pathType == PathType.Container:
            basePath = self.volumeRootPathInContainer
        return "{0}/volumes/peer/{1}".format(basePath, composition.projectName)

    def getPeerList(self, composition):
        return [serviceName for serviceName in composition.getServiceNames() if "peer" in serviceName]

    def getLocalMspConfigPath(self, composition, peerService, pathType=PathType.Local):
        return "{0}/{1}/localMspConfig".format(self.getVolumePath(composition, pathType), peerService)

    def _createLocalMspConfigDirs(self, mspConfigPath):
        os.makedirs("{0}/{1}".format(mspConfigPath, "signcerts"))
        os.makedirs("{0}/{1}".format(mspConfigPath, "admincerts"))
        os.makedirs("{0}/{1}".format(mspConfigPath, "cacerts"))
        os.makedirs("{0}/{1}".format(mspConfigPath, "keystore"))


    def composing(self, composition, context):
        'Will copy local MSP info over at this point for each peer node'

        directory = getDirectory(context)

        for peerService in self.getPeerList(composition):
            localMspConfigPath = self.getLocalMspConfigPath(composition, peerService)
            self._createLocalMspConfigDirs(localMspConfigPath)
            # Loop through directory and place Peer Organization Certs into cacerts folder
            for peerOrg in [org for orgName,org in directory.organizations.items() if Network.Peer in org.networks]:
                with open("{0}/cacerts/{1}.pem".format(localMspConfigPath, peerOrg.name), "w") as f:
                    f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, peerOrg.getSelfSignedCert()))

            # Loop through directory and place Peer Organization Certs into admincerts folder
            #TODO: revisit this, ASO recommended for now
            for peerOrg in [org for orgName,org in directory.organizations.items() if Network.Peer in org.networks]:
                with open("{0}/admincerts/{1}.pem".format(localMspConfigPath, peerOrg.name), "w") as f:
                    f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, peerOrg.getSelfSignedCert()))

            # Find the peer signer Tuple for this peer and add to signcerts folder
            for pnt, cert in [(peerNodeTuple,cert) for peerNodeTuple,cert in directory.ordererAdminTuples.items() if peerService in peerNodeTuple.user and "signer" in peerNodeTuple.user.lower()]:
                # Put the PEM file in the signcerts folder
                with open("{0}/signcerts/{1}.pem".format(localMspConfigPath, pnt.user), "w") as f:
                    f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, cert))
                # Put the associated private key into the keystore folder
                user = directory.getUser(pnt.user, shouldCreate=False)
                with open("{0}/keystore/{1}.pem".format(localMspConfigPath, pnt.user), "w") as f:
                    f.write(user.ecdsaSigningKey.to_pem())
                    # f.write(crypto.dump_privatekey(crypto.FILETYPE_PEM, user.pKey))


    def decomposing(self, composition, context):
        'Will remove the orderer volume path folder for the context'
        shutil.rmtree(self.getVolumePath(composition))

    def getEnv(self, composition, context, env):
        for peerService in self.getPeerList(composition):
            localMspConfigPath = self.getLocalMspConfigPath(composition, peerService, pathType=PathType.Container)
            env["{0}_CORE_PEER_MSPCFGPATH".format(peerService.upper())]=localMspConfigPath

def createChainCreationPolicyNames(context, chainCreationPolicyNames, chaindId):
    directory = getDirectory(context)
    bootstrapHelper = BootstrapHelper(chainId = chaindId)
    chainCreationPolicyNamesSignedConfigItem = bootstrapHelper.encodeChainCreators(ciValue = orderer_dot_configuration_pb2.ChainCreationPolicyNames(names=chainCreationPolicyNames).SerializeToString())
    return chainCreationPolicyNamesSignedConfigItem


def createChainCreatorsPolicy(context, chainCreatePolicyName, chaindId, orgNames):
    'Creates the chain Creator Policy with name'
    directory = getDirectory(context)
    bootstrapHelper = BootstrapHelper(chainId = chaindId)

    # This represents the domain of organization which can create channels for the orderer
    # First create org MSPPrincicpal

    # Collect the orgs from the table
    mspPrincipalList = []
    for org in [directory.getOrganization(orgName) for orgName in orgNames]:
        mspPrincipalList.append(msp_principal_pb2.MSPPrincipal(PrincipalClassification=msp_principal_pb2.MSPPrincipal.Classification.Value("ByIdentity"),
            Principal=crypto.dump_certificate(crypto.FILETYPE_ASN1, org.getSelfSignedCert())))
    policyTypeSig = common_dot_configuration_pb2.Policy.PolicyType.Value("SIGNATURE")
    chainCreatorsOrgsPolicySignedConfigItem = bootstrapHelper.encodePolicy(key=chainCreatePolicyName , policy=common_dot_configuration_pb2.Policy(type=policyTypeSig, policy=AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(0,[]), identities=mspPrincipalList).SerializeToString()))
    # print("signed Config Item:\n{0}\n".format(chainCreationPolicyNamesSignedConfigItem))
    #print("chain Creation orgs signed Config Item:\n{0}\n".format(chainCreatorsOrgsPolicySignedConfigItem))
    return chainCreatorsOrgsPolicySignedConfigItem

def setOrdererBootstrapGenesisBlock(genesisBlock):
    'Responsible for setting the GensisBlock for the Orderer nodes upon composition'

def broadcastCreateChannelConfigTx(context, composeService, chainId, configTxEnvelope, user):

    dataFunc = lambda x: configTxEnvelope
    user.broadcastMessages(context=context,numMsgsToBroadcast=1,composeService=composeService, chainID=chainId ,dataFunc=dataFunc, chainHeaderType=common_dot_common_pb2.CONFIGURATION_TRANSACTION)

def getArgsFromContextForUser(context, userName):
    directory = getDirectory(context)
    # Update the chaincodeSpec ctorMsg for invoke
    args = []
    if 'table' in context:
        if context.table:
            # There are function arguments
            user = directory.getUser(context, userName)
            # Allow the user to specify expressions referencing tags in the args list
            pattern = re.compile('\{(.*)\}$')
            for arg in context.table[0].cells:
                m = pattern.match(arg)
                if m:
                    # tagName reference found in args list
                    tagName = m.groups()[0]
                    # make sure the tagName is found in the users tags
                    assert tagName in user.tags, "TagName '{0}' not found for user '{1}'".format(tagName, user.getUserName())
                    args.append(user.tags[tagName])
                else:
                    #No tag referenced, pass the arg
                    args.append(arg)
    return args
