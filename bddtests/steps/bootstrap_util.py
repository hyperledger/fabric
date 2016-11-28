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
from collections import namedtuple
from enum import Enum

from google.protobuf import timestamp_pb2
from common import common_pb2 as common_dot_common_pb2
from common import configuration_pb2 as common_dot_configuration_pb2
# import orderer
from orderer import configuration_pb2 as orderer_dot_configuration_pb2

import os
import shutil
import compose

# Type to represent tuple of user, nodeName, ogranization
NodeAdminTuple = namedtuple("NodeAdminTuple", ['user', 'nodeName', 'organization'])



def createKey():
    #Create RSA key, 2048 bit
    pk = crypto.PKey()
    pk.generate_key(crypto.TYPE_RSA,2048)
    assert pk.check()==True
    return pk

def computeCryptoHash(data):
    s = hashlib.sha3_256()
    s.update(data)
    return  s.hexdigest()


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
        self.pKey = createKey()

    def createCertRequest(self, nodeName):
        req = createCertRequest(self.pKey, CN=nodeName)
        print("request => {0}".format(crypto.dump_certificate_request(crypto.FILETYPE_PEM, req)))
        return req



class User(Entity):
    def __init__(self, name):
        Entity.__init__(self, name)

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

        print("new Certificate for '{0}' on orderer '{1}' signed by '{2}': \n{3}".format(userName, ordererName, organizationName, crypto.dump_certificate(crypto.FILETYPE_PEM, userCert)))
        self.ordererAdminTuples[ordererAdminTuple] = userCert


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
    KEY_CHAIN_CREATORS = "ChainCreators"
    KEY_ACCEPT_ALL_POLICY = "AcceptAllPolicy"
    KEY_BATCH_SIZE = "BatchSize"

    DEFAULT_MODIFICATION_POLICY_ID = "DefaultModificationPolicy"
    DEFAULT_CHAIN_CREATORS = [KEY_ACCEPT_ALL_POLICY]

    DEFAULT_NONCE_SIZE = 24

    def __init__(self, chainId = "TestChain", lastModified = 0, msgVersion = 1, epoch = 0, consensusType = "solo", batchSize = 10):
        self.chainId = str(chainId)
        self.lastModified = lastModified
        self.msgVersion = msgVersion
        self.epoch = epoch
        self.consensusType = consensusType
        self.batchSize = batchSize

    def getNonce(self):
        return rand.bytes(self.DEFAULT_NONCE_SIZE)

    def makeChainHeader(self, type = common_dot_common_pb2.HeaderType.Value("CONFIGURATION_ITEM"), version = 1,
                        timestamp = timestamp_pb2.Timestamp(seconds = int(time.time()), nanos = 0)):
        return common_dot_common_pb2.ChainHeader(type = type,
                                      version = version,
                                      timestamp = timestamp,
                                      chainID = self.chainId,
                                      epoch = self.epoch)
    def makeSignatureHeader(self, serializeCertChain, nonce):
        return common_dot_common_pb2.ChainHeader(type = type,
                                                 version = version,
                                                 timestamp = timestamp,
                                                 chainID = self.chainId,
                                                 epoch = self.epoch)

    def signConfigItem(self, configItem):
        signedConfigItem = common_dot_configuration_pb2.SignedConfigurationItem(ConfigurationItem=configItem.SerializeToString(), Signatures=None)
        return signedConfigItem

    def getConfigItem(self, commonConfigType, key, value):
        configItem = common_dot_configuration_pb2.ConfigurationItem(
            Header=self.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value("CONFIGURATION_ITEM")),
            Type=commonConfigType,
            LastModified=self.lastModified,
            ModificationPolicy=BootstrapHelper.DEFAULT_MODIFICATION_POLICY_ID,
            Key=key,
            Value=value)
        return configItem

    def encodeBatchSize(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_BATCH_SIZE,
            value=orderer_dot_configuration_pb2.BatchSize(messages=self.batchSize).SerializeToString())
        return self.signConfigItem(configItem)

    def  encodeConsensusType(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_CONSENSUS_TYPE,
            value=orderer_dot_configuration_pb2.ConsensusType(type=self.consensusType).SerializeToString())
        return self.signConfigItem(configItem)

    def  encodeChainCreators(self):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.KEY_CHAIN_CREATORS,
            value=orderer_dot_configuration_pb2.ChainCreators(policies=BootstrapHelper.DEFAULT_CHAIN_CREATORS).SerializeToString())
        return self.signConfigItem(configItem)


    def _encodeSignaturePolicyEnvelope(self, signaturePolicyEnvelope):
        configItem = self.getConfigItem(
            commonConfigType=common_dot_configuration_pb2.ConfigurationItem.ConfigurationType.Value("Orderer"),
            key=BootstrapHelper.DEFAULT_MODIFICATION_POLICY_ID,
            value=signaturePolicyEnvelope.SerializeToString())
        return self.signConfigItem(configItem)

    def encodeAcceptAllPolicy(self):
        acceptAllPolicy =  AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(0,[]), identities=[])
        return self._encodeSignaturePolicyEnvelope(acceptAllPolicy)

    def lockDefaultModificationPolicy(self):
        rejectAllPolicy =  AuthDSLHelper.Envelope(signaturePolicy=AuthDSLHelper.NOutOf(1,[]), identities=[])
        return self._encodeSignaturePolicyEnvelope(rejectAllPolicy)

    def computeBlockDataHash(self, blockData):
        return computeCryptoHash(blockData.SerializeToString())

# Registerses a user on a specific composeService
def getDirectory(context):
    if 'bootstrapDirectory' not in context:
        context.bootstrapDirectory = Directory()
    return context.bootstrapDirectory


def createGenesisBlock(context, chainId, networkConfigPolicy, consensusType):
    'Generates the genesis block for starting the oderers and for use in the chain config transaction by peers'
    #assert not "bootstrapGenesisBlock" in context,"Genesis block already created:\n{0}".format(context.bootstrapGenesisBlock)
    directory = getDirectory(context)
    assert len(directory.ordererAdminTuples) > 0, "No orderer admin tuples defined!!!"

    bootstrapHelper = BootstrapHelper(chainId = chainId, consensusType=consensusType)
    configItems = []
    configItems.append(bootstrapHelper.encodeBatchSize())
    configItems.append(bootstrapHelper.encodeConsensusType())
    configItems.append(bootstrapHelper.encodeChainCreators())
    configItems.append(bootstrapHelper.encodeAcceptAllPolicy())
    configItems.append(bootstrapHelper.lockDefaultModificationPolicy())
    configEnvelope = common_dot_configuration_pb2.ConfigurationEnvelope(Items=configItems)


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
    payload = common_dot_common_pb2.Payload(header=payloadHeader, data=configEnvelope.SerializeToString())
    envelope = common_dot_common_pb2.Envelope(payload=payload.SerializeToString(), signature=None)

    blockData = common_dot_common_pb2.BlockData(Data=[envelope.SerializeToString()])


    block = common_dot_common_pb2.Block(
        Header=common_dot_common_pb2.BlockHeader(
            Number=0,
            PreviousHash=None,
            DataHash=bootstrapHelper.computeBlockDataHash(blockData),
        ),
        Data=blockData,
        Metadata=None,
    )

    # Add this back once crypto certs are required
    for nodeAdminTuple in directory.ordererAdminTuples:
        userCert = directory.ordererAdminTuples[nodeAdminTuple]
        certAsPEM = crypto.dump_certificate(crypto.FILETYPE_PEM, userCert)
        # print("UserCert for orderer genesis:\n{0}\n".format(certAsPEM))
        # print("")

    return block

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
        env["ORDERER_GENERAL_GENESISIFILE"]=self.getGenesisFilePath(composition, pathType=PathType.Container)



def setOrdererBootstrapGenesisBlock(genesisBlock):
    'Responsible for setting the GensisBlock for the Orderer nodes upon composition'



