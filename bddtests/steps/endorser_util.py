import chaincode_pb2
import fabric_next_pb2
import bdd_test_util
import bdd_grpc_util

def getChaincodeSpec(ccType, path, args):
	# make chaincode spec for chaincode to be deployed
	ccSpec = chaincode_pb2.ChaincodeSpec(type = ccType,
		chaincodeID = chaincode_pb2.ChaincodeID(path=path),
		ctorMsg = chaincode_pb2.ChaincodeInput(args = args))
	return ccSpec

def createPropsalId():
	return 'TODO proposal Id'

def createDeploymentProposalForBDD(ccSpec):
	"Returns a deployment proposal of chaincode type"
	lc_chaincode_spec = getChaincodeSpec(chaincode_pb2.ChaincodeSpec.GOLANG, "lccc", ['deployBDD', ccSpec.SerializeToString()])
	# make proposal
	proposal = fabric_next_pb2.Proposal(type = fabric_next_pb2.Proposal.CHAINCODE, id = createPropsalId())
	proposal.payload = lc_chaincode_spec.SerializeToString()
	return proposal

def getEndorserStubs(context, composeServices):
	stubs = []
	for composeService in composeServices:
		ipAddress = bdd_test_util.ipFromContainerNamePart(composeService, context.compose_containers)
		channel = bdd_grpc_util.getGRPCChannel(ipAddress)
		newEndorserStub = fabric_next_pb2.beta_create_Endorser_stub(channel)
		stubs.append(newEndorserStub)
	return stubs
