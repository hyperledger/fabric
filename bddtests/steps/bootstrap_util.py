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

from collections import namedtuple
from itertools import groupby

from enum import Enum

from google.protobuf import timestamp_pb2
from common import common_pb2 as common_dot_common_pb2
from common import configtx_pb2 as common_dot_configtx_pb2
from common import configuration_pb2 as common_dot_configuration_pb2
from common import policies_pb2 as common_dot_policies_pb2
from common import msp_principal_pb2
from msp import mspconfig_pb2
from peer import configuration_pb2 as peer_dot_configuration_pb2
from orderer import configuration_pb2 as orderer_dot_configuration_pb2
import identities_pb2
import orderer_util

from contexthelper import ContextHelper

import os
import re
import shutil
import compose

# Type to represent tuple of user, nodeName, ogranization
NodeAdminTuple = namedtuple("NodeAdminTuple", ['user', 'nodeName', 'organization'])


ApplicationGroup = "Application"
OrdererGroup     = "Orderer"
MSPKey           = "MSP"
toValue = lambda message: message.SerializeToString()


class Network(Enum):
    Orderer = 1
    Peer = 2


def GetUUID():
    return compose.Composition.GetUUID()


def createRSAKey():
    # Create RSA key, 2048 bit
    pk = crypto.PKey()
    pk.generate_key(crypto.TYPE_RSA, 2048)
    assert pk.check() == True
    return pk


def createECDSAKey(curve=ecdsa.NIST256p):
    # Create ECDSA key
    sk = ecdsa.SigningKey.generate(curve=curve)
    return sk


def computeCryptoHash(data):
    ' This will currently return 128 hex characters'
    # s = hashlib.sha3_256()
    s = hashlib.sha256()
    # s = hashlib.shake_256()
    #return s.digest(64)
    s.update(data)
    return s.digest()


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


def createCertificate(req, issuerCertKey, serial, validityPeriod, digest="sha256", isCA=False):
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
    cert.set_version(3)
    cert.set_serial_number(serial)
    cert.gmtime_adj_notBefore(notBefore)
    cert.gmtime_adj_notAfter(notAfter)
    cert.set_issuer(issuerCert.get_subject())
    cert.set_subject(req.get_subject())
    cert.set_pubkey(req.get_pubkey())
    if isCA:
        cert.add_extensions([crypto.X509Extension("basicConstraints", True,
                                                  "CA:TRUE, pathlen:0"),
                             crypto.X509Extension("subjectKeyIdentifier", False, "hash",
                                                  subject=cert)])
        #TODO: This only is appropriate for root self signed!!!!
        cert.add_extensions([crypto.X509Extension("authorityKeyIdentifier", False, "keyid:always", issuer=cert)])
    else:
        cert.add_extensions([crypto.X509Extension("basicConstraints", True,
                                                  "CA:FALSE"),
                             crypto.X509Extension("subjectKeyIdentifier", False, "hash",
                                                  subject=cert)])
        cert.add_extensions([crypto.X509Extension("authorityKeyIdentifier", False, "keyid:always", issuer=issuerCert)])

    cert.sign(issuerKey, digest)
    return cert


# SUBJECT_DEFAULT = {countryName : "US", stateOrProvinceName : "NC", localityName : "RTP", organizationName : "IBM", organizationalUnitName : "Blockchain"}

class Entity:
    def __init__(self, name):
        self.name = name
        # Create a ECDSA key, then a crypto pKey from the DER for usage with cert requests, etc.
        self.ecdsaSigningKey = createECDSAKey()
        self.rsaSigningKey = createRSAKey()
        self.pKey = crypto.load_privatekey(crypto.FILETYPE_ASN1, self.ecdsaSigningKey.to_der())
        # Signing related ecdsa config
        self.hashfunc = hashlib.sha256
        self.sigencode = ecdsa.util.sigencode_der_canonize
        self.sigdecode = ecdsa.util.sigdecode_der

    def createCertRequest(self, nodeName):
        req = createCertRequest(self.pKey, C="US", ST="North Carolina", L="RTP", O="IBM", CN=nodeName)
        # print("request => {0}".format(crypto.dump_certificate_request(crypto.FILETYPE_PEM, req)))
        return req

    def createTLSCertRequest(self, nodeName):
        req = createCertRequest(self.rsaSigningKey, C="US", ST="North Carolina", L="RTP", O="IBM", CN=nodeName)
        # print("request => {0}".format(crypto.dump_certificate_request(crypto.FILETYPE_PEM, req)))
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

    def getPrivateKeyAsPEM(self):
        return self.ecdsaSigningKey.to_pem()


class User(Entity, orderer_util.UserRegistration):
    def __init__(self, name, directory):
        Entity.__init__(self, name)
        orderer_util.UserRegistration.__init__(self, name, directory)
        self.tags = {}

    def setTagValue(self, tagKey, tagValue, overwrite=False):
        if tagKey in self.tags:
            assert not overwrite,"TagKey '{0}' already exists for user {1}, and did not provide overwrite=True".format(tagKey, self.getUserName())
        self.tags[tagKey] = tagValue
        return tagValue

    def cleanup(self):
        self.closeStreams()


class Organization(Entity):

    def __init__(self, name):
        Entity.__init__(self, name)
        req = createCertRequest(self.pKey, C="US", ST="North Carolina", L="RTP", O="IBM", CN=name)
        numYrs = 1
        self.signedCert = createCertificate(req, (req, self.pKey), 1000, (0, 60 * 60 * 24 * 365 * numYrs), isCA=True)
        # Which networks this organization belongs to
        self.networks = []

    def __repr__(self):
        return "name:{0}, networks: {1}".format(self.name, self.networks)

    def getSelfSignedCert(self):
        return self.signedCert

    def getCertAsPEM(self):
        return crypto.dump_certificate(crypto.FILETYPE_PEM, self.getSelfSignedCert())

    def getMSPConfig(self):
        certPemsList = [crypto.dump_certificate(crypto.FILETYPE_PEM, self.getSelfSignedCert())]
        # For now, admin certs and CA certs are the same per @ASO
        adminCerts = certPemsList
        cacerts = adminCerts
        # Currently only 1 component, CN=<orgName>
        # name = self.getSelfSignedCert().get_subject().getComponents()[0][1]
        name = self.name
        fabricMSPConfig = mspconfig_pb2.FabricMSPConfig(admins=adminCerts, root_certs=cacerts, name=name)
        mspConfig = mspconfig_pb2.MSPConfig(config=fabricMSPConfig.SerializeToString(), type=0)
        return mspConfig

    def isInNetwork(self, network):
        for n in self.networks:
            if str(n)==str(network):
                return True
        return False

    def getMspPrincipalAsRole(self, mspRoleTypeAsString):
        mspRole = msp_principal_pb2.MSPRole(msp_identifier=self.name, Role=msp_principal_pb2.MSPRole.MSPRoleType.Value(mspRoleTypeAsString))
        mspPrincipal = msp_principal_pb2.MSPPrincipal(
            principal_classification=msp_principal_pb2.MSPPrincipal.Classification.Value('ROLE'),
            principal=mspRole.SerializeToString())
        return mspPrincipal

    def createCertificate(self, certReq):
        numYrs = 1
        return createCertificate(certReq, (self.signedCert, self.pKey), 1000, (0, 60 * 60 * 24 * 365 * numYrs))

    def addToNetwork(self, network):
        'Used to track which network this organization is defined in.'
        # assert network in Network, 'Network not recognized ({0}), expected to be one of ({1})'.format(network, list(Network))
        if not network in self.networks:
            self.networks.append(network)


class Directory:
    def __init__(self):
        self.organizations = {}
        self.users = {}
        self.ordererAdminTuples = {}

    def getNamedCtxTuples(self):
        return self.ordererAdminTuples

    def _registerOrg(self, orgName):
        assert orgName not in self.organizations, "Organization already registered {0}".format(orgName)
        self.organizations[orgName] = Organization(orgName)
        return self.organizations[orgName]

    def _registerUser(self, userName):
        assert userName not in self.users, "User already registered {0}".format(userName)
        self.users[userName] = User(userName, directory=self)
        return self.users[userName]

    def getUser(self, userName, shouldCreate=False):
        if not userName in self.users and shouldCreate:
            # self.users[userName] = User(userName)
            self._registerUser(userName)
        return self.users[userName]

    def getUsers(self):
        return self.users

    def cleanup(self):
        '''Perform cleanup of resources'''
        [user.cleanup() for user in self.users.values()]

    def getOrganization(self, orgName, shouldCreate=False):
        if not orgName in self.organizations and shouldCreate:
            # self.organizations[orgName] = Organization(orgName)
            self._registerOrg(orgName)
        return self.organizations[orgName]

    def getOrganizations(self):
        return self.organizations

    def findCertByTuple(self, userName, contextName, orgName):
        ordererAdminTuple = NodeAdminTuple(user=userName, nodeName=contextName, organization=orgName)
        return self.ordererAdminTuples[ordererAdminTuple]

    def findCertForNodeAdminTuple(self, nodeAdminTuple):
        assert nodeAdminTuple in self.ordererAdminTuples, "Node admin tuple not found for: {0}".format(nodeAdminTuple)
        return self.ordererAdminTuples[nodeAdminTuple]

    def getCertAsPEM(self, nodeAdminTuple):
        assert nodeAdminTuple in self.ordererAdminTuples, "Node admin tuple not found for: {0}".format(nodeAdminTuple)
        return crypto.dump_certificate(crypto.FILETYPE_PEM, self.ordererAdminTuples[nodeAdminTuple])

    def findNodeAdminTuple(self, userName, contextName, orgName):
        nodeAdminTuple = NodeAdminTuple(user=userName, nodeName=contextName, organization=orgName)
        assert nodeAdminTuple in self.ordererAdminTuples, "Node admin tuple not found for: {0}".format(nodeAdminTuple)
        return nodeAdminTuple

    def getTrustedRootsForPeerNetworkAsPEM(self):
        pems = [peerOrg.getCertAsPEM() for peerOrg in [org for org in self.getOrganizations().values() if org.isInNetwork(Network.Peer)]]
        return "".join(pems)

    def getTrustedRootsForOrdererNetworkAsPEM(self):
        pems = [ordererOrg.getCertAsPEM() for ordererOrg in [org for org in self.getOrganizations().values() if org.isInNetwork(Network.Orderer)]]
        return "".join(pems)


    def registerOrdererAdminTuple(self, userName, ordererName, organizationName):
        ' Assign the user as orderer admin'
        ordererAdminTuple = NodeAdminTuple(user=userName, nodeName=ordererName, organization=organizationName)
        assert ordererAdminTuple not in self.ordererAdminTuples, "Orderer admin tuple already registered {0}".format(
            ordererAdminTuple)
        assert organizationName in self.organizations, "Orderer Organization not defined {0}".format(organizationName)

        user = self.getUser(userName, shouldCreate=True)
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
        return common_dot_policies_pb2.SignaturePolicyEnvelope(
            version=0,
            policy=signaturePolicy,
            identities=identities)

    @classmethod
    def NOutOf(cls, n, policies):
        'NOutOf creates a policy which requires N out of the slice of policies to evaluate to true'
        return common_dot_policies_pb2.SignaturePolicy(
            n_out_of=common_dot_policies_pb2.SignaturePolicy.NOutOf(
                N=n,
                policies=policies,
            ),
        )

    @classmethod
    def SignedBy(cls, index):
        'NOutOf creates a policy which requires N out of the slice of policies to evaluate to true'
        return common_dot_policies_pb2.SignaturePolicy(
            signed_by=index
        )

class BootstrapHelper:
    KEY_CONSENSUS_TYPE = "ConsensusType"
    KEY_CHAIN_CREATION_POLICY_NAMES = "ChainCreationPolicyNames"
    KEY_ACCEPT_ALL_POLICY = "AcceptAllPolicy"
    KEY_HASHING_ALGORITHM = "HashingAlgorithm"
    KEY_BLOCKDATA_HASHING_STRUCTURE = "BlockDataHashingStructure"
    KEY_BATCH_SIZE = "BatchSize"
    KEY_BATCH_TIMEOUT = "BatchTimeout"
    KEY_CREATIONPOLICY = "CreationPolicy"
    KEY_MSP_INFO = "MSP"
    KEY_ANCHOR_PEERS = "AnchorPeers"

    KEY_NEW_CONFIGURATION_ITEM_POLICY = "NewConfigurationItemPolicy"
    DEFAULT_CHAIN_CREATORS = [KEY_ACCEPT_ALL_POLICY]

    # ReadersPolicyKey is the key used for the read policy
    KEY_POLICY_READERS = "Readers"
    # WritersPolicyKey is the key used for the writer policy
    KEY_POLICY_WRITERS = "Writers"
    # AdminsPolicyKey is the key used for the admins policy
    KEY_POLICY_ADMINS = "Admins"

    KEY_POLICY_BLOCK_VALIDATION = "BlockValidation"

    # OrdererAddressesKey is the cb.ConfigItem type key name for the OrdererAddresses message
    KEY_ORDERER_ADDRESSES = "OrdererAddresses"


    DEFAULT_NONCE_SIZE = 24

    @classmethod
    def getNonce(cls):
        return rand.bytes(BootstrapHelper.DEFAULT_NONCE_SIZE)

    @classmethod
    def addSignatureToSignedConfigItem(cls, configUpdateEnvelope, (entity, cert)):
        serializedIdentity = identities_pb2.SerializedIdentity(Mspid=entity.name, IdBytes=crypto.dump_certificate(crypto.FILETYPE_PEM, cert))
        sigHeader = common_dot_common_pb2.SignatureHeader(creator=serializedIdentity.SerializeToString(),
                                                          nonce=BootstrapHelper.getNonce())
        sigHeaderBytes = sigHeader.SerializeToString()
        # Signature over the concatenation of configurationItem bytes and signatureHeader bytes
        signature = entity.sign(sigHeaderBytes + configUpdateEnvelope.config_update)
        # Now add new signature to Signatures repeated field
        newConfigSig = configUpdateEnvelope.signatures.add()
        newConfigSig.signature_header = sigHeaderBytes
        newConfigSig.signature = signature

    def __init__(self, chainId, lastModified=0, msgVersion=1, epoch=0, consensusType="solo", batchSize=10,
                 batchTimeout=".5s", absoluteMaxBytes=100000000, preferredMaxBytes=512 * 1024, signers=[]):
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

    def makeChainHeader(self, type, txID="", extension='',
                        version=1,
                        timestamp=timestamp_pb2.Timestamp(seconds=int(time.time()), nanos=0)):
        return common_dot_common_pb2.ChannelHeader(type=type,
                                                 version=version,
                                                 timestamp=timestamp,
                                                 channel_id=self.chainId,
                                                 epoch=self.epoch,
                                                 tx_id=txID,
                                                 extension=extension)

    def makeSignatureHeader(self, serializeCertChain, nonce):
        return common_dot_common_pb2.SignatureHeader(creator=serializeCertChain,
                                                     nonce=nonce)

    def signConfigItem(self, configItem):
        # signedConfigItem = common_dot_configuration_pb2.SignedConfigurationItem(
        #     ConfigurationItem=configItem.SerializeToString(), Signatures=None)
        # return signedConfigItem
        return configItem

    def getConfigItem(self, commonConfigType, key, value):
        configItem = common_dot_configtx_pb2.ConfigItem(
            version=self.lastModified,
            configPath=commonConfigType,
            key=key,
            mod_policy=BootstrapHelper.KEY_NEW_CONFIGURATION_ITEM_POLICY,
            value=value)
        return configItem

    def encodeAnchorInfo(self, ciValue):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("PEER"),
            key=BootstrapHelper.KEY_ANCHOR_PEERS,
            value=ciValue.SerializeToString())
        return self.signConfigItem(configItem)

    def encodeMspInfo(self, mspUniqueId, ciValue):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("MSP"),
            key=mspUniqueId,
            value=ciValue.SerializeToString())
        return self.signConfigItem(configItem)

    def encodeHashingAlgorithm(self, hashingAlgorithm="SHAKE256"):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("CHAIN"),
            key=BootstrapHelper.KEY_HASHING_ALGORITHM,
            value=common_dot_configuration_pb2.HashingAlgorithm(name=hashingAlgorithm).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeBatchSize(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("ORDERER"),
            key=BootstrapHelper.KEY_BATCH_SIZE,
            value=orderer_dot_configuration_pb2.BatchSize(maxMessageCount=self.batchSize,
                                                          absoluteMaxBytes=self.absoluteMaxBytes,
                                                          preferredMaxBytes=self.preferredMaxBytes).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeBatchTimeout(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("ORDERER"),
            key=BootstrapHelper.KEY_BATCH_TIMEOUT,
            value=orderer_dot_configuration_pb2.BatchTimeout(timeout=self.batchTimeout).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeConsensusType(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("ORDERER"),
            key=BootstrapHelper.KEY_CONSENSUS_TYPE,
            value=orderer_dot_configuration_pb2.ConsensusType(type=self.consensusType).SerializeToString())
        return self.signConfigItem(configItem)

    def encodeChainCreators(self, ciValue=orderer_dot_configuration_pb2.ChainCreationPolicyNames(
        names=DEFAULT_CHAIN_CREATORS).SerializeToString()):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("ORDERER"),
            key=BootstrapHelper.KEY_CHAIN_CREATION_POLICY_NAMES,
            value=ciValue)
        return self.signConfigItem(configItem)

    def encodePolicy(self, key, policy=common_dot_policies_pb2.Policy(
        type=common_dot_policies_pb2.Policy.PolicyType.Value("SIGNATURE"),
        policy=AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(0, []), identities=[]).SerializeToString())):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("POLICY"),
            key=key,
            value=policy.SerializeToString())
        return self.signConfigItem(configItem)

    def encodeAcceptAllPolicy(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("POLICY"),
            key=BootstrapHelper.KEY_ACCEPT_ALL_POLICY,
            value=common_dot_policies_pb2.Policy(type=1, policy=AuthDSLHelper.Envelope(
                signaturePolicy=AuthDSLHelper.NOutOf(0, []), identities=[]).SerializeToString()).SerializeToString())
        return self.signConfigItem(configItem)

    def lockDefaultModificationPolicy(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("POLICY"),
            key=BootstrapHelper.KEY_NEW_CONFIGURATION_ITEM_POLICY,
            value=common_dot_policies_pb2.Policy(type=1, policy=AuthDSLHelper.Envelope(
                signaturePolicy=AuthDSLHelper.NOutOf(1, []), identities=[]).SerializeToString()).SerializeToString())
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
        # data = ''
        # for sci in signedConfigItems:
        #     data = data + sci.ConfigurationItem
        # # Compute hash over concatenated bytes
        # hash = computeCryptoHash(data)
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configtx_pb2.ConfigItem.ConfigType.Value("ORDERER"),
            key=BootstrapHelper.KEY_CREATIONPOLICY,
            value=orderer_dot_configuration_pb2.CreationPolicy(policy=chainCreationPolicyName).SerializeToString())
        return [self.signConfigItem(configItem)] + signedConfigItems


def createConfigUpdateEnvelope(channelConfigGroup, chainId, chainCreationPolicyName):
    # Returns a list prepended with a signedConfiguration
    channelConfigGroup.groups[OrdererGroup].values[BootstrapHelper.KEY_CREATIONPOLICY].value = toValue(
        orderer_dot_configuration_pb2.CreationPolicy(policy=chainCreationPolicyName))
    config_update_envelope = createNewConfigUpdateEnvelope(channelConfig=channelConfigGroup, chainId=chainId)
    return config_update_envelope


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
    assert not policyName in policyNameToOrgNamesDict, "PolicyName '{0}' already registered with ordererBootstrapAdmin".format(
        policyName)
    policyNameToOrgNamesDict[policyName] = orgNames
    return policyNameToOrgNamesDict


def getOrdererBootstrapAdminOrgReferences(context):
    directory = getDirectory(context)
    ordererBootstrapAdmin = directory.getUser(userName="ordererBootstrapAdmin", shouldCreate=False)
    if not 'OrgReferences' in ordererBootstrapAdmin.tags:
        ordererBootstrapAdmin.tags['OrgReferences'] = {}
    return ordererBootstrapAdmin.tags['OrgReferences']


def getSignedMSPConfigItems(context, orgNames):
    directory = getDirectory(context)
    orgs = [directory.getOrganization(orgName) for orgName in orgNames]

    channel = common_dot_configtx_pb2.ConfigGroup()
    for org in orgs:
        channel.groups[ApplicationGroup].groups[org.name].values[BootstrapHelper.KEY_MSP_INFO].value = toValue(
            org.getMSPConfig())
    return [channel]


def getAnchorPeersConfigGroup(context, nodeAdminTuples):
    directory = getDirectory(context)
    config_group = common_dot_configtx_pb2.ConfigGroup()
    for orgName, group in groupby([(nat.organization, nat) for nat in nodeAdminTuples], lambda x: x[0]):
        anchorPeers = peer_dot_configuration_pb2.AnchorPeers()
        for (k,nodeAdminTuple) in group:
            anchorPeer = anchorPeers.anchor_peers.add()
            anchorPeer.host = nodeAdminTuple.nodeName
            anchorPeer.port = 7051
            # anchorPeer.cert = crypto.dump_certificate(crypto.FILETYPE_PEM,
            #                                           directory.findCertForNodeAdminTuple(nodeAdminTuple))
        config_group.groups[ApplicationGroup].groups[orgName].values[BootstrapHelper.KEY_ANCHOR_PEERS].value=toValue(anchorPeers)
    return [config_group]


def getMspConfigItemsForPolicyNames(context, policyNames):
    policyNameToOrgNamesDict = getOrdererBootstrapAdminOrgReferences(context)
    # Get unique set of org names and return set of signed MSP ConfigItems
    orgNamesReferenced = list(
        set([orgName for policyName in policyNames for orgName in policyNameToOrgNamesDict[policyName]]))
    orgNamesReferenced.sort()
    return getSignedMSPConfigItems(context=context, orgNames=orgNamesReferenced)


def createSignedConfigItems(directory, configGroups=[]):

    channelConfig = createChannelConfigGroup(directory)
    for configGroup in configGroups:
        mergeConfigGroups(channelConfig, configGroup)
    return channelConfig

def setMetaPolicy(channelId, channgel_config_groups):
    #New meta policy info
    typeImplicitMeta = common_dot_policies_pb2.Policy.PolicyType.Value("IMPLICIT_META")
    P = common_dot_policies_pb2.Policy
    IMP=common_dot_policies_pb2.ImplicitMetaPolicy
    ruleAny = common_dot_policies_pb2.ImplicitMetaPolicy.Rule.Value("ANY")
    ruleMajority = common_dot_policies_pb2.ImplicitMetaPolicy.Rule.Value("MAJORITY")
    channgel_config_groups.groups[channelId].groups[ApplicationGroup].policies[BootstrapHelper.KEY_POLICY_READERS].policy.CopyFrom(
        P(type=typeImplicitMeta, policy=IMP(
            rule=ruleAny).SerializeToString()))
    channgel_config_groups.groups[channelId].groups[ApplicationGroup].policies[BootstrapHelper.KEY_POLICY_WRITERS].policy.CopyFrom(
        P(type=typeImplicitMeta, policy=IMP(
            rule=ruleAny).SerializeToString()))
    channgel_config_groups.groups[channelId].groups[ApplicationGroup].policies[BootstrapHelper.KEY_POLICY_ADMINS].policy.CopyFrom(
        P(type=typeImplicitMeta, policy=IMP(
            rule=ruleAny).SerializeToString()))



def createChannelConfigGroup(directory, hashingAlgoName="SHA256", consensusType="solo", batchTimeout="1s", batchSizeMaxMessageCount=10, batchSizeAbsoluteMaxBytes=100000000, batchSizePreferredMaxBytes=512 * 1024):

    channel = common_dot_configtx_pb2.ConfigGroup()
    # channel.groups[ApplicationGroup] = common_dot_configtx_pb2.ConfigGroup()
    # channel.groups[OrdererGroup] = common_dot_configtx_pb2.ConfigGroup()
    channel.groups[ApplicationGroup]
    channel.groups[OrdererGroup]
    # v = common_dot_configtx_pb2.ConfigItem.ConfigType.Value
    # configItems.append(bootstrapHelper.encodeHashingAlgorithm())
    channel.values[BootstrapHelper.KEY_HASHING_ALGORITHM].value = toValue(
        common_dot_configuration_pb2.HashingAlgorithm(name=hashingAlgoName))

    golangMathMaxUint32 = 4294967295
    channel.values[BootstrapHelper.KEY_BLOCKDATA_HASHING_STRUCTURE].value = toValue(
        common_dot_configuration_pb2.BlockDataHashingStructure(width=golangMathMaxUint32))

    channel.groups[OrdererGroup].values[BootstrapHelper.KEY_BATCH_SIZE].value = toValue(orderer_dot_configuration_pb2.BatchSize(maxMessageCount=batchSizeMaxMessageCount,absoluteMaxBytes=batchSizeAbsoluteMaxBytes,preferredMaxBytes=batchSizePreferredMaxBytes))
    channel.groups[OrdererGroup].values[BootstrapHelper.KEY_BATCH_TIMEOUT].value = toValue(orderer_dot_configuration_pb2.BatchTimeout(timeout=batchTimeout))
    channel.groups[OrdererGroup].values[BootstrapHelper.KEY_CONSENSUS_TYPE].value = toValue(orderer_dot_configuration_pb2.ConsensusType(type=consensusType))

    acceptAllPolicy = common_dot_policies_pb2.Policy(type=1, policy=AuthDSLHelper.Envelope(
        signaturePolicy=AuthDSLHelper.NOutOf(0, []), identities=[]).SerializeToString())
    channel.policies[BootstrapHelper.KEY_ACCEPT_ALL_POLICY].policy.CopyFrom(acceptAllPolicy)

    # For now, setting same policies for each 'Non-Org' group
    typeImplicitMeta = common_dot_policies_pb2.Policy.PolicyType.Value("IMPLICIT_META")
    Policy = common_dot_policies_pb2.Policy
    IMP = common_dot_policies_pb2.ImplicitMetaPolicy
    ruleAny = common_dot_policies_pb2.ImplicitMetaPolicy.Rule.Value("ANY")
    ruleMajority = common_dot_policies_pb2.ImplicitMetaPolicy.Rule.Value("MAJORITY")
    for group in [channel, channel.groups[ApplicationGroup], channel.groups[OrdererGroup]]:
        group.policies[BootstrapHelper.KEY_POLICY_READERS].policy.CopyFrom(Policy(type=typeImplicitMeta, policy=IMP(
            rule=ruleAny, sub_policy=BootstrapHelper.KEY_POLICY_READERS).SerializeToString()))
        group.policies[BootstrapHelper.KEY_POLICY_WRITERS].policy.CopyFrom(Policy(type=typeImplicitMeta, policy=IMP(
            rule=ruleAny, sub_policy=BootstrapHelper.KEY_POLICY_WRITERS).SerializeToString()))
        group.policies[BootstrapHelper.KEY_POLICY_ADMINS].policy.CopyFrom(Policy(type=typeImplicitMeta, policy=IMP(
            rule=ruleMajority, sub_policy=BootstrapHelper.KEY_POLICY_ADMINS).SerializeToString()))
    # Setting block validation policy for the orderer group
    channel.groups[OrdererGroup].policies[BootstrapHelper.KEY_POLICY_BLOCK_VALIDATION].policy.CopyFrom(Policy(type=typeImplicitMeta, policy=IMP(
        rule=ruleAny, sub_policy=BootstrapHelper.KEY_POLICY_WRITERS).SerializeToString()))

    # Add the orderer org groups MSPConfig info
    for ordererOrg in [org for org in directory.getOrganizations().values() if Network.Orderer in org.networks]:
        channel.groups[OrdererGroup].groups[ordererOrg.name].values[BootstrapHelper.KEY_MSP_INFO].value = toValue(
            ordererOrg.getMSPConfig())



    # Now set policies for each org group (Both peer and orderer)
    #TODO: Revisit after Jason does a bit more refactoring on chain creation policy enforcement
    groupNameDict = {Network.Peer : ApplicationGroup, Network.Orderer : OrdererGroup}
    for org in directory.getOrganizations().values():
        for network in org.networks:
            groupName = groupNameDict[network]
            mspPrincipalForMemberRole = org.getMspPrincipalAsRole(mspRoleTypeAsString='MEMBER')
            memberSignaturePolicyEnvelope = AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.SignedBy(0), identities=[mspPrincipalForMemberRole])
            memberPolicy = common_dot_policies_pb2.Policy(
                type=common_dot_policies_pb2.Policy.PolicyType.Value("SIGNATURE"),
                policy=memberSignaturePolicyEnvelope.SerializeToString())
            channel.groups[groupName].groups[org.name].policies[BootstrapHelper.KEY_POLICY_READERS].policy.CopyFrom(memberPolicy)
            channel.groups[groupName].groups[org.name].policies[BootstrapHelper.KEY_POLICY_WRITERS].policy.CopyFrom(memberPolicy)

            mspPrincipalForAdminRole = org.getMspPrincipalAsRole(mspRoleTypeAsString='ADMIN')
            adminSignaturePolicyEnvelope = AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.SignedBy(0), identities=[mspPrincipalForAdminRole])
            adminPolicy = common_dot_policies_pb2.Policy(
                type=common_dot_policies_pb2.Policy.PolicyType.Value("SIGNATURE"),
                policy=adminSignaturePolicyEnvelope.SerializeToString())
            channel.groups[groupName].groups[org.name].policies[BootstrapHelper.KEY_POLICY_ADMINS].policy.CopyFrom(adminPolicy)

            # signaturePolicyEnvelope = AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.SignedBy(0), identities=[mspPrincipal])


    #New OrdererAddress
    ordererAddress = common_dot_configuration_pb2.OrdererAddresses()
    for ordererNodeTuple, cert in [(user_node_tuple, cert) for user_node_tuple, cert in directory.ordererAdminTuples.iteritems() if
                      "orderer" in user_node_tuple.user and "signer" in user_node_tuple.user.lower()]:
        ordererAddress.addresses.append("{0}:7050".format(ordererNodeTuple.nodeName))
    assert len(ordererAddress.addresses) > 0, "No orderer nodes were found while trying to create channel ConfigGroup"
    channel.values[BootstrapHelper.KEY_ORDERER_ADDRESSES].value = toValue(ordererAddress)
    return channel

def createEnvelopeForMsg(directory, nodeAdminTuple, chainId, msg, typeAsString):
    # configEnvelope = common_dot_configtx_pb2.ConfigEnvelope(last_update=envelope.SerializeToString())
    bootstrapHelper = BootstrapHelper(chainId=chainId)
    payloadChainHeader = bootstrapHelper.makeChainHeader(
        type=common_dot_common_pb2.HeaderType.Value(typeAsString))

    # Now the SignatureHeader
    org = directory.getOrganization(nodeAdminTuple.organization)
    user = directory.getUser(nodeAdminTuple.user)
    cert = directory.findCertForNodeAdminTuple(nodeAdminTuple)
    serializedIdentity = identities_pb2.SerializedIdentity(Mspid=org.name, IdBytes=crypto.dump_certificate(crypto.FILETYPE_PEM, cert))
    serializedCreatorCertChain = serializedIdentity.SerializeToString()
    nonce = None
    payloadSignatureHeader = common_dot_common_pb2.SignatureHeader(
        creator=serializedCreatorCertChain,
        nonce=bootstrapHelper.getNonce(),
    )

    payloadHeader = common_dot_common_pb2.Header(
        channel_header=payloadChainHeader.SerializeToString(),
        signature_header=payloadSignatureHeader.SerializeToString(),
    )
    payload = common_dot_common_pb2.Payload(header=payloadHeader, data=msg.SerializeToString())
    payloadBytes = payload.SerializeToString()
    envelope = common_dot_common_pb2.Envelope(payload=payloadBytes, signature=user.sign(payloadBytes))
    return envelope

    return configEnvelope

def createNewConfigUpdateEnvelope(channelConfig, chainId):
    configUpdate = common_dot_configtx_pb2.ConfigUpdate(channel_id=chainId,
                                                        write_set=channelConfig)
    configUpdateEnvelope = common_dot_configtx_pb2.ConfigUpdateEnvelope(config_update=configUpdate.SerializeToString(), signatures =[])
    return configUpdateEnvelope


def mergeConfigGroups(configGroupTarget, configGroupSource):
    for k, v in configGroupSource.groups.iteritems():
        if k in configGroupTarget.groups.keys():
            mergeConfigGroups(configGroupTarget.groups[k], configGroupSource.groups[k])
        else:
            configGroupTarget.groups[k].MergeFrom(v)
    for k, v in configGroupSource.policies.iteritems():
        if k in configGroupTarget.policies.keys():
            mergeConfigGroups(configGroupTarget.policies[k], configGroupSource.policies[k])
        else:
            configGroupTarget.policies[k].MergeFrom(v)
    for k, v in configGroupSource.values.iteritems():
        assert not k in configGroupTarget.values.keys(), "Value already exists in target config group: {0}".format(k)
        configGroupTarget.values[k].CopyFrom(v)


def createGenesisBlock(context, chainId, consensusType, nodeAdminTuple, signedConfigItems=[]):
    'Generates the genesis block for starting the oderers and for use in the chain config transaction by peers'
    # assert not "bootstrapGenesisBlock" in context,"Genesis block already created:\n{0}".format(context.bootstrapGenesisBlock)
    directory = getDirectory(context)
    assert len(directory.ordererAdminTuples) > 0, "No orderer admin tuples defined!!!"

    channelConfig = createChannelConfigGroup(directory)
    for configGroup in signedConfigItems:
        mergeConfigGroups(channelConfig, configGroup)

    # (fileName, fileExist) = ContextHelper.GetHelper(context=context).getTmpPathForName(name="t",extension="protobuf")
    # with open(fileName, 'w') as f:
    #     f.write(channelConfig.SerializeToString())

    config = common_dot_configtx_pb2.Config(
        sequence=0,
        channel_group=channelConfig)

    configEnvelope = common_dot_configtx_pb2.ConfigEnvelope(config=config)
    envelope = createEnvelopeForMsg(directory=directory, chainId=chainId, nodeAdminTuple=nodeAdminTuple, msg=configEnvelope, typeAsString="CONFIG")
    blockData = common_dot_common_pb2.BlockData(data=[envelope.SerializeToString()])

    # Spoke with kostas, for orderer in general
    signaturesMetadata = ""
    lastConfigurationBlockMetadata = common_dot_common_pb2.Metadata(
        value=common_dot_common_pb2.LastConfig(index=0).SerializeToString()).SerializeToString()
    ordererConfigMetadata = ""
    transactionFilterMetadata = ""
    bootstrapHelper = BootstrapHelper(chainId="NOT_USED")
    block = common_dot_common_pb2.Block(
        header=common_dot_common_pb2.BlockHeader(
            number=0,
            previous_hash=None,
            data_hash=bootstrapHelper.computeBlockDataHash(blockData),
        ),
        data=blockData,
        metadata=common_dot_common_pb2.BlockMetadata(
            metadata=[signaturesMetadata, lastConfigurationBlockMetadata, transactionFilterMetadata,
                      ordererConfigMetadata]),
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


class CallbackHelper:
    def __init__(self, discriminator, volumeRootPathInContainer = "/var/hyperledger/bddtests"):
        self.volumeRootPathInContainer = volumeRootPathInContainer
        self.discriminator = discriminator

    def getVolumePath(self, composition, pathType=PathType.Local):
        assert pathType in PathType, "Expected pathType of {0}".format(PathType)
        basePath = "."
        if pathType == PathType.Container:
            basePath = self.volumeRootPathInContainer
        return "{0}/volumes/{1}/{2}".format(basePath, self.discriminator, composition.projectName)

    def getLocalMspConfigPath(self, composition, compose_service, pathType=PathType.Local):
        return "{0}/{1}/localMspConfig".format(self.getVolumePath(composition, pathType), compose_service)

    def getLocalTLSConfigPath(self, composition, compose_service, pathType=PathType.Local):
        return os.path.join(self.getVolumePath(composition, pathType), compose_service, "tls_config")

    def _getPathAndUserInfo(self, directory , composition, compose_service, nat_discriminator="Signer", pathType=PathType.Local):
        matchingNATs = [nat for nat in directory.getNamedCtxTuples() if ((compose_service in nat.user) and (nat_discriminator in nat.user) and ((compose_service in nat.nodeName)))]
        assert len(matchingNATs)==1, "Unexpected number of matching NodeAdminTuples: {0}".format(matchingNATs)
        localMspConfigPath = self.getLocalMspConfigPath(composition=composition, compose_service=compose_service,pathType=pathType)
        return (localMspConfigPath, matchingNATs[0])

    def getLocalMspConfigPrivateKeyPath(self, directory , composition, compose_service, pathType=PathType.Local):
        (localMspConfigPath, nodeAdminTuple) = self._getPathAndUserInfo(directory=directory, composition=composition, compose_service=compose_service, pathType=pathType)
        return "{0}/keystore/{1}.pem".format(localMspConfigPath, nodeAdminTuple.user)

    def getLocalMspConfigPublicCertPath(self, directory , composition, compose_service, pathType=PathType.Local):
        (localMspConfigPath, nodeAdminTuple) = self._getPathAndUserInfo(directory=directory, composition=composition, compose_service=compose_service, pathType=pathType)
        return "{0}/signcerts/{1}.pem".format(localMspConfigPath, nodeAdminTuple.user)

    def getTLSKeyPaths(self, pnt , composition, compose_service, pathType=PathType.Local):
        localTLSConfigPath = self.getLocalTLSConfigPath(composition, compose_service, pathType=pathType)
        certPath = os.path.join(localTLSConfigPath,
                                "{0}-{1}-{2}-tls.crt".format(pnt.user, pnt.nodeName, pnt.organization))
        keyPath = os.path.join(localTLSConfigPath,
                               "{0}-{1}-{2}-tls.key".format(pnt.user, pnt.nodeName, pnt.organization))
        return (keyPath, certPath)


    def getLocalMspConfigRootCertPath(self, directory , composition, compose_service, pathType=PathType.Local):
        (localMspConfigPath, nodeAdminTuple) = self._getPathAndUserInfo(directory=directory, composition=composition, compose_service=compose_service, pathType=pathType)
        return "{0}/cacerts/{1}.pem".format(localMspConfigPath, nodeAdminTuple.organization)

    def _createCryptoMaterial(self,directory , composition, compose_service, network):
        self._writeMspFiles(directory , composition, compose_service, network)
        self._writeTLSFiles(directory , composition, compose_service, network)

    def _writeMspFiles(self, directory , composition, compose_service, network):
        localMspConfigPath = self.getLocalMspConfigPath(composition, compose_service)
        os.makedirs("{0}/{1}".format(localMspConfigPath, "signcerts"))
        os.makedirs("{0}/{1}".format(localMspConfigPath, "admincerts"))
        os.makedirs("{0}/{1}".format(localMspConfigPath, "cacerts"))
        os.makedirs("{0}/{1}".format(localMspConfigPath, "keystore"))
        # Loop through directory and place Organization Certs into cacerts folder
        for targetOrg in [org for orgName, org in directory.organizations.items() if network in org.networks]:
            with open("{0}/cacerts/{1}.pem".format(localMspConfigPath, targetOrg.name), "w") as f:
                f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, targetOrg.getSelfSignedCert()))

        # Loop through directory and place Organization Certs into admincerts folder
        # TODO: revisit this, ASO recommended for now
        for targetOrg in [org for orgName, org in directory.organizations.items() if network in org.networks]:
            with open("{0}/admincerts/{1}.pem".format(localMspConfigPath, targetOrg.name), "w") as f:
                f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, targetOrg.getSelfSignedCert()))

        # Find the peer signer Tuple for this peer and add to signcerts folder
        for pnt, cert in [(peerNodeTuple, cert) for peerNodeTuple, cert in directory.ordererAdminTuples.items() if
                          compose_service in peerNodeTuple.user and "signer" in peerNodeTuple.user.lower()]:
            # Put the PEM file in the signcerts folder
            with open("{0}/signcerts/{1}.pem".format(localMspConfigPath, pnt.user), "w") as f:
                f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, cert))
            # Put the associated private key into the keystore folder
            user = directory.getUser(pnt.user, shouldCreate=False)
            with open("{0}/keystore/{1}.pem".format(localMspConfigPath, pnt.user), "w") as f:
                f.write(user.ecdsaSigningKey.to_pem())

    def _writeTLSFiles(self, directory , composition, compose_service, network):
        localTLSConfigPath = self.getLocalTLSConfigPath(composition, compose_service)
        os.makedirs(localTLSConfigPath)
        # Find the peer signer Tuple for this peer and add to signcerts folder
        for pnt, cert in [(peerNodeTuple, cert) for peerNodeTuple, cert in directory.ordererAdminTuples.items() if
                          compose_service in peerNodeTuple.user and "signer" in peerNodeTuple.user.lower()]:
            user = directory.getUser(userName=pnt.user)
            userTLSCert = directory.getOrganization(pnt.organization).createCertificate(user.createTLSCertRequest(pnt.nodeName))
            (keyPath, certPath) = self.getTLSKeyPaths(pnt=pnt, composition=composition, compose_service=compose_service, pathType=PathType.Local)
            with open(keyPath, 'w') as f:
                f.write(crypto.dump_privatekey(crypto.FILETYPE_PEM, user.rsaSigningKey))
            with open(certPath, 'w') as f:
                f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, userTLSCert))

    def _getMspId(self, compose_service, directory):
        matchingNATs = [nat for nat in directory.getNamedCtxTuples() if ((compose_service in nat.user) and ("Signer" in nat.user) and ((compose_service in nat.nodeName)))]
        assert len(matchingNATs)==1, "Unexpected number of matching NodeAdminTuples: {0}".format(matchingNATs)
        return matchingNATs[0].organization


class OrdererGensisBlockCompositionCallback(compose.CompositionCallback, CallbackHelper):
    'Responsible for setting the GensisBlock for the Orderer nodes upon composition'

    def __init__(self, context, genesisBlock, genesisFileName="genesis_file"):
        CallbackHelper.__init__(self, discriminator="orderer")
        self.context = context
        self.genesisFileName = genesisFileName
        self.genesisBlock = genesisBlock
        compose.Composition.RegisterCallbackInContext(context, self)

    def getGenesisFilePath(self, composition, pathType=PathType.Local):
        return "{0}/{1}".format(self.getVolumePath(composition, pathType), self.genesisFileName)

    def getOrdererList(self, composition):
        return [serviceName for serviceName in composition.getServiceNames() if "orderer" in serviceName]

    def composing(self, composition, context):
        print("Will copy gensisiBlock over at this point ")
        os.makedirs(self.getVolumePath(composition))
        with open(self.getGenesisFilePath(composition), "wb") as f:
            f.write(self.genesisBlock.SerializeToString())
        directory = getDirectory(context)

        for ordererService in self.getOrdererList(composition):
            self._createCryptoMaterial(directory=directory,
                                       compose_service=ordererService,
                                       composition=composition,
                                       network=Network.Orderer)

    def decomposing(self, composition, context):
        'Will remove the orderer volume path folder for the context'
        shutil.rmtree(self.getVolumePath(composition))

    def getEnv(self, composition, context, env):
        directory = getDirectory(context)
        env["ORDERER_GENERAL_GENESISMETHOD"] = "file"
        env["ORDERER_GENERAL_GENESISFILE"] = self.getGenesisFilePath(composition, pathType=PathType.Container)
        for ordererService in self.getOrdererList(composition):
            localMspConfigPath = self.getLocalMspConfigPath(composition, ordererService, pathType=PathType.Container)
            env["{0}_ORDERER_GENERAL_LOCALMSPDIR".format(ordererService.upper())] = localMspConfigPath
            env["{0}_ORDERER_GENERAL_LOCALMSPID".format(ordererService.upper())] = self._getMspId(compose_service=ordererService, directory=directory)
            # TLS Settings
            (_, pnt) = self._getPathAndUserInfo(directory=directory, composition=composition, compose_service=ordererService, pathType=PathType.Container)
            (keyPath, certPath) = self.getTLSKeyPaths(pnt=pnt, composition=composition, compose_service=ordererService, pathType=PathType.Container)
            env["{0}_ORDERER_GENERAL_TLS_CERTIFICATE".format(ordererService.upper())] = certPath
            env["{0}_ORDERER_GENERAL_TLS_PRIVATEKEY".format(ordererService.upper())] = keyPath
            # env["{0}_ORDERER_GENERAL_TLS_ROOTCAS".format(ordererService.upper())] = "[{0}]".format(self.getLocalMspConfigRootCertPath(
            #     directory=directory, composition=composition, compose_service=ordererService, pathType=PathType.Container))

class PeerCompositionCallback(compose.CompositionCallback, CallbackHelper):
    'Responsible for setting up Peer nodes upon composition'

    def __init__(self, context):
        CallbackHelper.__init__(self, discriminator="peer")
        self.context = context
        compose.Composition.RegisterCallbackInContext(context, self)

    def getPeerList(self, composition):
        return [serviceName for serviceName in composition.getServiceNames() if "peer" in serviceName]

    def composing(self, composition, context):
        'Will copy local MSP info over at this point for each peer node'

        directory = getDirectory(context)

        for peerService in self.getPeerList(composition):
            self._createCryptoMaterial(directory=directory,
                                       compose_service=peerService,
                                       composition=composition,
                                       network=Network.Peer)

    def decomposing(self, composition, context):
        'Will remove the orderer volume path folder for the context'
        shutil.rmtree(self.getVolumePath(composition))

    def getEnv(self, composition, context, env):
        directory = getDirectory(context)
        # First figure out which organization provided the signer cert for this
        for peerService in self.getPeerList(composition):
            localMspConfigPath = self.getLocalMspConfigPath(composition, peerService, pathType=PathType.Container)
            env["{0}_CORE_PEER_MSPCFGPATH".format(peerService.upper())] = localMspConfigPath
            env["{0}_CORE_PEER_LOCALMSPID".format(peerService.upper())] = self._getMspId(compose_service=peerService, directory=directory)
            # TLS Settings
            # env["{0}_CORE_PEER_TLS_ENABLED".format(peerService.upper())] = self._getMspId(compose_service=peerService, directory=directory)
            (_, pnt) = self._getPathAndUserInfo(directory=directory, composition=composition, compose_service=peerService, pathType=PathType.Container)
            (keyPath, certPath) = self.getTLSKeyPaths(pnt=pnt, composition=composition, compose_service=peerService, pathType=PathType.Container)
            env["{0}_CORE_PEER_TLS_CERT_FILE".format(peerService.upper())] = certPath
            env["{0}_CORE_PEER_TLS_KEY_FILE".format(peerService.upper())] = keyPath
            env["{0}_CORE_PEER_TLS_ROOTCERT_FILE".format(peerService.upper())] = self.getLocalMspConfigRootCertPath(
                directory=directory, composition=composition, compose_service=peerService, pathType=PathType.Container)
            env["{0}_CORE_PEER_TLS_SERVERHOSTOVERRIDE".format(peerService.upper())] = peerService


def createChainCreationPolicyNames(context, chainCreationPolicyNames, chaindId):
    channel = common_dot_configtx_pb2.ConfigGroup()
    channel.groups[OrdererGroup].values[BootstrapHelper.KEY_CHAIN_CREATION_POLICY_NAMES].value = toValue(
        orderer_dot_configuration_pb2.ChainCreationPolicyNames(
            names=chainCreationPolicyNames))
    return channel


def createChainCreatorsPolicy(context, chainCreatePolicyName, chaindId, orgNames):
    'Creates the chain Creator Policy with name'
    directory = getDirectory(context)
    bootstrapHelper = BootstrapHelper(chainId=chaindId)

    # This represents the domain of organization which can create channels for the orderer
    # First create org MSPPrincicpal

    # Collect the orgs from the table
    mspPrincipalList = []
    for org in [directory.getOrganization(orgName) for orgName in orgNames]:
        mspPrincipalList.append(msp_principal_pb2.MSPPrincipal(
            principal_classification=msp_principal_pb2.MSPPrincipal.Classification.Value("IDENTITY"),
            principal=crypto.dump_certificate(crypto.FILETYPE_ASN1, org.getSelfSignedCert())))
    policy = common_dot_policies_pb2.Policy(
        type=common_dot_policies_pb2.Policy.PolicyType.Value("SIGNATURE"),
        policy=AuthDSLHelper.Envelope(
            signaturePolicy=AuthDSLHelper.NOutOf(
                0, []),
            identities=mspPrincipalList).SerializeToString())
    channel = common_dot_configtx_pb2.ConfigGroup()
    channel.policies[chainCreatePolicyName].policy.CopyFrom(policy)

    # print("signed Config Item:\n{0}\n".format(chainCreationPolicyNamesSignedConfigItem))
    # print("chain Creation orgs signed Config Item:\n{0}\n".format(chainCreatorsOrgsPolicySignedConfigItem))
    return channel


def setOrdererBootstrapGenesisBlock(genesisBlock):
    'Responsible for setting the GensisBlock for the Orderer nodes upon composition'


def broadcastCreateChannelConfigTx(context, certAlias, composeService, chainId, configTxEnvelope, user):
    dataFunc = lambda x: configTxEnvelope
    user.broadcastMessages(context=context, numMsgsToBroadcast=1, composeService=composeService, chainID=chainId,
                           dataFunc=dataFunc)


def getArgsFromContextForUser(context, userName):
    directory = getDirectory(context)
    # Update the chaincodeSpec ctorMsg for invoke
    args = []
    if 'table' in context:
        if context.table:
            # There are function arguments
            user = directory.getUser(userName)
            # Allow the user to specify expressions referencing tags in the args list
            pattern = re.compile('\{(.*)\}$')
            for arg in context.table[0].cells:
                m = pattern.match(arg)
                if m:
                    # tagName reference found in args list
                    tagName = m.groups()[0]
                    # make sure the tagName is found in the users tags
                    assert tagName in user.tags, "TagName '{0}' not found for user '{1}'".format(tagName,
                                                                                                 user.getUserName())
                    args.append(user.tags[tagName])
                else:
                    # No tag referenced, pass the arg
                    args.append(arg)
    return args


def getChannelIdFromConfigUpdateEnvelope(config_update_envelope):
    config_update = common_dot_configtx_pb2.ConfigUpdate()
    config_update.ParseFromString(config_update_envelope.config_update)
    return config_update
