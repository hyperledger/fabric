
import os
import json
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
		chaincodeID = chaincode_pb2.ChaincodeID(path=path, name=name),
		input = chaincode_pb2.ChaincodeInput(args = args))
	return ccSpec

def createPropsalId():
	return 'TODO proposal Id'

def createInvokeProposalForBDD(ccSpec, chainID, signersCert, Mspid, type):
	"Returns a deployment proposal of chaincode type"
	lc_chaincode_invocation_spec = chaincode_pb2.ChaincodeInvocationSpec(chaincodeSpec = ccSpec)

	# Create
	ccHdrExt = chaincode_proposal_pb2.ChaincodeHeaderExtension(chaincodeID=ccSpec.chaincodeID)

	ccProposalPayload = chaincode_proposal_pb2.ChaincodeProposalPayload(Input=lc_chaincode_invocation_spec.SerializeToString())

	bootstrapHelper = bootstrap_util.BootstrapHelper(chainId=chainID)

	chainHdr = bootstrapHelper.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value(type),
											   txID=bootstrap_util.GetUUID(), extension=ccHdrExt.SerializeToString())
	serializedIdentity = identities_pb2.SerializedIdentity(Mspid=Mspid, IdBytes=crypto.dump_certificate(crypto.FILETYPE_PEM, signersCert))


	sigHdr = bootstrapHelper.makeSignatureHeader(serializedIdentity.SerializeToString(), bootstrap_util.BootstrapHelper.getNonce())

	header = common_dot_common_pb2.Header(chainHeader=chainHdr, signatureHeader=sigHdr)

	# make proposal
	proposal = proposal_pb2.Proposal(header=header.SerializeToString(), payload=ccProposalPayload.SerializeToString())


	return proposal


def signProposal(proposal, entity, signersCert):
	import hashlib
	from ecdsa import util
	from ecdsa import VerifyingKey
	import binascii
	# Sign the proposal
	proposalBytes = proposal.SerializeToString()
	#print("Proposal Bytes = \n{0}\n\n".format(binascii.hexlify(bytearray(proposalBytes))))

	# calculate sha2_256 and dump for info only
	digest = hashlib.sha256(proposalBytes).digest()
	print("Proposal Bytes digest= \n{0}\n\n".format( binascii.hexlify(bytearray(digest))  ))
	signature = entity.sign(proposalBytes)
	# signature = signingKey.sign(proposalBytes, hashfunc=hashlib.sha256, sigencode=util.sigencode_der)

	#Verify the signature
	entity.verifySignature(signature=signature, signersCert=signersCert, data=proposalBytes)
	# vk = VerifyingKey.from_der(crypto.dump_publickey(crypto.FILETYPE_ASN1, signersCert.get_pubkey()))
	# assert vk.verify(signature, proposalBytes, hashfunc=hashlib.sha256, sigdecode=util.sigdecode_der), "Invalid signature!!"

	print("Proposal Bytes signature= \n{0}\n\n".format(binascii.hexlify(bytearray(signature))))
	print("")

	signedProposal = proposal_pb2.SignedProposal(proposalBytes=proposalBytes, signature=signature)
	return signedProposal


def createDeploymentChaincodeSpecForBDD(ccDeploymentSpec, chainID):
	"Returns a deployment proposal of chaincode type"
	lc_chaincode_spec = chaincode_pb2.ChaincodeSpec(type = chaincode_pb2.ChaincodeSpec.GOLANG,
										 chaincodeID = chaincode_pb2.ChaincodeID(name="lccc"),
										 input = chaincode_pb2.ChaincodeInput(args = ['deploy', chainID, ccDeploymentSpec.SerializeToString()]))
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
    nameArgs = ["-n", ccSpec.chaincodeID.name]
    ctorArgs = ["-c", json.dumps({'Args' : [item for item in ccSpec.input.args]})]
    pathArgs = ["-p", ccSpec.chaincodeID.path]
    output, error, returncode = \
        bdd_test_util.cli_call(["peer","chaincode","package"] + nameArgs + ctorArgs + pathArgs + [outputPath], expect_success=True, env=myEnv)
    return output


def createDeploymentSpec(context, ccSpec):
    contextHelper = bootstrap_util.ContextHelper.GetHelper(context=context)
    cacheDeploymentSpec = contextHelper.isConfigEnabled("cache-deployment-spec")
    fileName = "deploymentspec_{0}".format(ccSpec.chaincodeID.name)
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
