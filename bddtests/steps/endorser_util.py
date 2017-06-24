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

import os
import json
from contexthelper import ContextHelper
import bdd_test_util
import bdd_grpc_util
import bootstrap_util
from peer import chaincode_pb2
from peer import transaction_pb2
from peer import proposal_pb2
from peer import peer_pb2_grpc
from msp import identities_pb2

from common import common_pb2 as common_dot_common_pb2

from OpenSSL import crypto

def getChaincodeSpec(ccType, path, name, args):
	# make chaincode spec for chaincode to be deployed
	ccSpec = chaincode_pb2.ChaincodeSpec(type=chaincode_pb2.ChaincodeSpec.Type.Value(ccType),
		chaincode_id = chaincode_pb2.ChaincodeID(path=path, name=name, version="test"),
		input = chaincode_pb2.ChaincodeInput(args = args))
	return ccSpec

def getChaincodeSpecUsingTemplate(template_chaincode_spec, args):
    # make chaincode spec for chaincode to be deployed
    ccSpec = chaincode_pb2.ChaincodeSpec()
    input = chaincode_pb2.ChaincodeInput(args = args)
    ccSpec.CopyFrom(template_chaincode_spec)
    ccSpec.input.CopyFrom(input)
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

    serializedIdentity = identities_pb2.SerializedIdentity(mspid=Mspid, id_bytes=crypto.dump_certificate(crypto.FILETYPE_PEM, signersCert))

    nonce = bootstrap_util.BootstrapHelper.getNonce()

    sigHdr = bootstrapHelper.makeSignatureHeader(serializedIdentity.SerializeToString(), nonce)

    # Calculate the transaction ID
    tx_id = binascii.hexlify(bootstrap_util.computeCryptoHash(nonce + serializedIdentity.SerializeToString()))

    chainHdr = bootstrapHelper.makeChainHeader(type=common_dot_common_pb2.HeaderType.Value(type),
                                               txID=tx_id, extension=ccHdrExt.SerializeToString())

    header = common_dot_common_pb2.Header(channel_header=chainHdr.SerializeToString(), signature_header=sigHdr.SerializeToString())

    # make proposal
    proposal = proposal_pb2.Proposal(header=header.SerializeToString(), payload=ccProposalPayload.SerializeToString())


    return proposal

def createSignedTx(user, signed_proposal, proposal_responses):
    assert len(proposal_responses) > 0, "Expected at least 1 ProposalResponse"
    #Unpack signed proposal
    proposal = proposal_pb2.Proposal()
    proposal.ParseFromString(signed_proposal.proposal_bytes)
    header = common_dot_common_pb2.Header()
    header.ParseFromString(proposal.header)
    ccProposalPayload = proposal_pb2.ChaincodeProposalPayload()
    ccProposalPayload.ParseFromString(proposal.payload)
    # Be sure to clear the TransientMap
    ccProposalPayload.TransientMap.clear()

    endorsements = [p.endorsement for p in proposal_responses]
    ccEndorsedAction = transaction_pb2.ChaincodeEndorsedAction(proposal_response_payload=proposal_responses[0].payload, endorsements=endorsements)

    ccActionPayload = transaction_pb2.ChaincodeActionPayload(chaincode_proposal_payload=ccProposalPayload.SerializeToString(), action=ccEndorsedAction)

    transaction = transaction_pb2.Transaction()
    action = transaction.actions.add()
    action.header = header.signature_header
    action.payload = ccActionPayload.SerializeToString()
    payload = common_dot_common_pb2.Payload(header=header, data=transaction.SerializeToString())
    payloadBytes = payload.SerializeToString()
    signature = user.sign(payloadBytes)
    envelope = common_dot_common_pb2.Envelope(payload=payloadBytes, signature=signature)
    return envelope



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
    lc_chaincode_spec = getChaincodeSpec(ccType="GOLANG", path="", name="lscc",
                                         args=['deploy', chainID, ccDeploymentSpec.SerializeToString()])
    return lc_chaincode_spec

def createInstallChaincodeSpecForBDD(ccDeploymentSpec, chainID):
    lc_chaincode_spec = getChaincodeSpec(ccType="GOLANG", path="", name="lscc",
                                         args=['install', ccDeploymentSpec.SerializeToString()])
    return lc_chaincode_spec

def getEndorserStubs(context, composeServices, directory, nodeAdminTuple):
    stubs = []
    user = directory.getUser(nodeAdminTuple.user)
    signingOrg = directory.getOrganization(nodeAdminTuple.organization)

    for composeService in composeServices:
        ipAddress, port = bdd_test_util.getPortHostMapping(context.compose_containers, composeService, 7051)
        # natForPeerSigner = directory.findNodeAdminTuple(userName="{0}Signer".format(composeService), contextName=composeService, orgName="peerOrg0")
        # signerCert = directory.getCertAsPEM(natForPeerSigner)
        root_certificates = directory.getTrustedRootsForPeerNetworkAsPEM()
        channel = bdd_grpc_util.getGRPCChannel(ipAddress=ipAddress, port=port, root_certificates=root_certificates,
                                               ssl_target_name_override=composeService)
        newEndorserStub = peer_pb2_grpc.EndorserStub(channel)
        stubs.append(newEndorserStub)
    return stubs

def getExample02ChaincodeSpec():
    return getChaincodeSpec("GOLANG", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "example02", ["init","a","100","b","200"])



def _createDeploymentSpecAsFile(ccSpec, outputPath):
    '''peer chaincode package -n myCC -c '{"Args":["init","a","100","b","200"]}' -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 --logging-level=DEBUG test.file'''
    myEnv = os.environ.copy()
    myEnv['CORE_PEER_MSPCONFIGPATH'] = "./../sampleconfig/msp"
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
