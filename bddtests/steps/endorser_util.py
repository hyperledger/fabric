
import os
import json
from contexthelper import ContextHelper
import bdd_test_util
import bdd_grpc_util
import bootstrap_util
from peer import chaincode_pb2
from peer import chaincode_proposal_pb2
from peer import proposal_pb2
from peer import peer_pb2_grpc
import identities_pb2

from common import common_pb2 as common_dot_common_pb2

from OpenSSL import crypto

def getChaincodeSpec(ccType, path, name, args):
	# make chaincode spec for chaincode to be deployed
	ccSpec = chaincode_pb2.ChaincodeSpec(type=chaincode_pb2.ChaincodeSpec.Type.Value(ccType),
		chaincode_id = chaincode_pb2.ChaincodeID(path=path, name=name, version="test"),
		input = chaincode_pb2.ChaincodeInput(args = args))
	return ccSpec

def createPropsalId():
	return 'TODO proposal Id'

def createInvokeProposalForBDD(context, ccSpec, chainID, signersCert, Mspid, type):
    import binascii

    "Returns a deployment proposal of chaincode type"
    lc_chaincode_invocation_spec = chaincode_pb2.ChaincodeInvocationSpec(chaincode_spec = ccSpec)

    # Create
    ccHdrExt = proposal_pb2.ChaincodeHeaderExtension(chaincode_id=ccSpec.chaincode_id)

    ccProposalPayload = proposal_pb2.ChaincodeProposalPayload(input=lc_chaincode_invocation_spec.SerializeToString())

    bootstrapHelper = ContextHelper.GetHelper(context=context).getBootrapHelper(chainId=chainID)

    serializedIdentity = identities_pb2.SerializedIdentity(Mspid=Mspid, IdBytes=crypto.dump_certificate(crypto.FILETYPE_PEM, signersCert))

    nonce = bootstrap_util.BootstrapHelper.getNonce()

    sigHdr = bootstrapHelper.makeSignatureHeader(serializedIdentity.SerializeToString(), nonce)
    
    # Calculate the transaction ID
    tx_id = binascii.hexlify(bootstrap_util.computeCryptoHash(nonce + serializedIdentity.SerializeToString()))

    chainHdr = bootstrapHelper.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value(type),
                                               txID=tx_id, extension=ccHdrExt.SerializeToString())

    header = common_dot_common_pb2.Header(channel_header=chainHdr, signature_header=sigHdr)

    # make proposal
    proposal = proposal_pb2.Proposal(header=header.SerializeToString(), payload=ccProposalPayload.SerializeToString())


    return proposal


def signProposal(proposal, entity, signersCert):
	import binascii
	# Sign the proposal
	proposalBytes = proposal.SerializeToString()
	signature = entity.sign(proposalBytes)
	#Verify the signature
	entity.verifySignature(signature=signature, signersCert=signersCert, data=proposalBytes)
	# print("Proposal Bytes signature= \n{0}\n\n".format(binascii.hexlify(bytearray(signature))))
	signedProposal = proposal_pb2.SignedProposal(proposal_bytes=proposalBytes, signature=signature)
	return signedProposal


def createDeploymentChaincodeSpecForBDD(ccDeploymentSpec, chainID):
    lc_chaincode_spec = getChaincodeSpec(ccType="GOLANG", path="", name="lccc",
                                         args=['deploy', chainID, ccDeploymentSpec.SerializeToString()])
	# lc_chaincode_spec = chaincode_pb2.ChaincodeSpec(type = chaincode_pb2.ChaincodeSpec.GOLANG,
	# 									 chaincode_id = chaincode_pb2.ChaincodeID(name="lccc"),
	# 									 input = chaincode_pb2.ChaincodeInput(args = ['deploy', chainID, ccDeploymentSpec.SerializeToString()]))
    return lc_chaincode_spec

def getEndorserStubs(context, composeServices):
    stubs = []
    for composeService in composeServices:
        ipAddress = bdd_test_util.ipFromContainerNamePart(composeService, context.compose_containers)
        channel = bdd_grpc_util.getGRPCChannel(ipAddress)
        newEndorserStub = peer_pb2_grpc.EndorserStub(channel)
        stubs.append(newEndorserStub)
    return stubs

def getExample02ChaincodeSpec():
    return getChaincodeSpec("GOLANG", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "example02", ["init","a","100","b","200"])



def _createDeploymentSpecAsFile(ccSpec, outputPath):
    '''peer chaincode package -n myCC -c '{"Args":["init","a","100","b","200"]}' -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 --logging-level=DEBUG test.file'''
    myEnv = os.environ.copy()
    myEnv['CORE_PEER_MSPCONFIGPATH'] = "./../msp/sampleConfig"
    nameArgs = ["-n", ccSpec.chaincode_id.name]
    ctorArgs = ["-c", json.dumps({'Args' : [item for item in ccSpec.input.args]})]
    pathArgs = ["-p", ccSpec.chaincode_id.path]
    versionArgs = ["-v", ccSpec.chaincode_id.version]
    output, error, returncode = \
        bdd_test_util.cli_call(["peer","chaincode","package"] + nameArgs + ctorArgs + pathArgs + versionArgs + [outputPath], expect_success=True, env=myEnv)
    return output


def createDeploymentSpec(context, ccSpec):
    contextHelper = ContextHelper.GetHelper(context=context)
    contextHelper.getBootrapHelper(chainId="test")
    cacheDeploymentSpec = contextHelper.isConfigEnabled("cache-deployment-spec")
    fileName = "deploymentspec-{0}-{1}-{2}".format(chaincode_pb2.ChaincodeSpec.Type.Name(ccSpec.type), ccSpec.chaincode_id.path, ccSpec.chaincode_id.name)
    outputPath, fileExists = contextHelper.getTmpPathForName(name=fileName,
                                                             copyFromCache=cacheDeploymentSpec)
    if not fileExists:
        _createDeploymentSpecAsFile(ccSpec=ccSpec, outputPath=outputPath)
        if cacheDeploymentSpec:
            contextHelper.copyToCache(fileName)
    ccDeploymentSpec = chaincode_pb2.ChaincodeDeploymentSpec()
    with open(outputPath, 'rb') as f:
        ccDeploymentSpec.ParseFromString(f.read())
    return ccDeploymentSpec
