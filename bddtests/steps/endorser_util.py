
import fabric_next_pb2
import bdd_test_util
import bdd_grpc_util
import bootstrap_util
from peer import chaincode_pb2
from peer import chaincode_proposal_pb2
from peer import fabric_proposal_pb2
from peer import fabric_service_pb2
import identities_pb2

from common import common_pb2 as common_dot_common_pb2

from OpenSSL import crypto

def getChaincodeSpec(ccType, path, name, args):
	# make chaincode spec for chaincode to be deployed
	ccSpec = chaincode_pb2.ChaincodeSpec(type=chaincode_pb2.ChaincodeSpec.Type.Value(ccType),
		chaincodeID = chaincode_pb2.ChaincodeID(path=path, name=name),
		ctorMsg = chaincode_pb2.ChaincodeInput(args = args))
	return ccSpec

def createPropsalId():
	return 'TODO proposal Id'

def createInvokeProposalForBDD(ccSpec, chainID, signersCert, Mspid):
	"Returns a deployment proposal of chaincode type"
	lc_chaincode_invocation_spec = chaincode_pb2.ChaincodeInvocationSpec(chaincodeSpec = ccSpec)

	# Create
	ccHdrExt = chaincode_proposal_pb2.ChaincodeHeaderExtension(chaincodeID=ccSpec.chaincodeID)

	ccProposalPayload = chaincode_proposal_pb2.ChaincodeProposalPayload(Input=lc_chaincode_invocation_spec.SerializeToString())

	bootstrapHelper = bootstrap_util.BootstrapHelper(chainId=chainID)

	chainHdr = bootstrapHelper.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value("CONFIGURATION_TRANSACTION"),
											   txID=bootstrap_util.GetUUID(), extension=ccHdrExt.SerializeToString())
	serializedIdentity = identities_pb2.SerializedIdentity(Mspid=Mspid, IdBytes=crypto.dump_certificate(crypto.FILETYPE_PEM, signersCert))


	sigHdr = bootstrapHelper.makeSignatureHeader(serializedIdentity.SerializeToString(), bootstrap_util.BootstrapHelper.getNonce())

	header = common_dot_common_pb2.Header(chainHeader=chainHdr, signatureHeader=sigHdr)

	# make proposal
	proposal = fabric_proposal_pb2.Proposal(header=header.SerializeToString(), payload=ccProposalPayload.SerializeToString())

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

	signedProposal = fabric_proposal_pb2.SignedProposal(proposalBytes=proposalBytes, signature=signature)
	return signedProposal


def createDeploymentProposalForBDD(ccDeploymentSpec):
	"Returns a deployment proposal of chaincode type"
	lc_chaincode_spec = chaincode_pb2.ChaincodeSpec(type = chaincode_pb2.ChaincodeSpec.GOLANG,
										 chaincodeID = chaincode_pb2.ChaincodeID(name="lccc"),
										 ctorMsg = chaincode_pb2.ChaincodeInput(args = ['deploy', 'default', ccDeploymentSpec.SerializeToString()]))
	lc_chaincode_invocation_spec = chaincode_pb2.ChaincodeInvocationSpec(chaincodeSpec = lc_chaincode_spec)
	# make proposal
	proposal = fabric_next_pb2.Proposal(type = fabric_next_pb2.Proposal.CHAINCODE, id = createPropsalId())
	proposal.payload = lc_chaincode_invocation_spec.SerializeToString()
	return proposal

def getEndorserStubs(context, composeServices):
	stubs = []
	for composeService in composeServices:
		ipAddress = bdd_test_util.ipFromContainerNamePart(composeService, context.compose_containers)
		channel = bdd_grpc_util.getGRPCChannel(ipAddress)
		newEndorserStub = fabric_service_pb2.beta_create_Endorser_stub(channel)
		stubs.append(newEndorserStub)
	return stubs
