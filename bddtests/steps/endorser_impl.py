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
import endorser_util
import bdd_grpc_util
import bdd_test_util
import bootstrap_util
import devops_pb2

@when(u'user "{userName}" creates a chaincode spec "{ccSpecAlias}" with name "{chaincodeName}" of type "{ccType}" for chaincode "{chaincodePath}" with args')
def step_impl(context, userName, ccType, chaincodeName, chaincodePath, ccSpecAlias):
	directory = bootstrap_util.getDirectory(context=context)
	user = directory.getUser(userName)
	args =  bootstrap_util.getArgsFromContextForUser(context, userName)
	ccSpec = endorser_util.getChaincodeSpec(ccType=ccType, path=chaincodePath, name=chaincodeName, args=bdd_grpc_util.toStringArray(args))
	print("ccSpec = {0}".format(ccSpec))
	user.tags[ccSpecAlias] = ccSpec


@when(u'user "{userName}" creates a deployment spec "{ccDeploymentSpecAlias}" using chaincode spec "{ccSpecAlias}" and devops on peer "{devopsComposeService}"')
def step_impl(context, userName, ccDeploymentSpecAlias, ccSpecAlias, devopsComposeService):
    directory = bootstrap_util.getDirectory(context=context)
    user = directory.getUser(userName=userName)
    assert ccSpecAlias in user.tags, "ChaincodeSpec alias '{0}' not found for user '{1}'".format(ccSpecAlias, userName)

    ipAddress = bdd_test_util.ipFromContainerNamePart(devopsComposeService, context.compose_containers)
    channel = bdd_grpc_util.getGRPCChannel(ipAddress)
    devopsStub = devops_pb2.beta_create_Devops_stub(channel)
    deploymentSpec = devopsStub.Build(user.tags[ccSpecAlias],20)
    user.tags[ccDeploymentSpecAlias] = deploymentSpec


@when(u'user "{userName}" using cert alias "{certAlias}" creates a deployment proposal "{proposalAlias}" for channel "{channelName}" using chaincode spec "{ccSpecAlias}"')
def step_impl(context, userName, certAlias, proposalAlias, channelName, ccSpecAlias):
        directory = bootstrap_util.getDirectory(context=context)
        user = directory.getUser(userName=userName)
        assert ccSpecAlias in user.tags, "ChaincodeSpec alias '{0}' not found for user '{1}'".format(ccSpecAlias, userName)
        ccSpec = user.tags[ccSpecAlias]


        ccDeploymentSpec = endorser_util.createDeploymentSpec(context=context, ccSpec=ccSpec)
        lcChaincodeSpec = endorser_util.createDeploymentChaincodeSpecForBDD(ccDeploymentSpec=ccDeploymentSpec, chainID=str(channelName))
        # Find the cert using the cert tuple information saved for the user under certAlias
        nodeAdminTuple = user.tags[certAlias]
        signersCert = directory.findCertForNodeAdminTuple(nodeAdminTuple)
        mspID = nodeAdminTuple.organization

        proposal = endorser_util.createInvokeProposalForBDD(ccSpec=lcChaincodeSpec, chainID=channelName,signersCert=signersCert, Mspid=mspID, type="ENDORSER_TRANSACTION")

        signedProposal = endorser_util.signProposal(proposal=proposal, entity=user, signersCert=signersCert)

        # proposal = endorser_util.createDeploymentProposalForBDD(ccDeploymentSpec)
        assert not proposalAlias in user.tags, "Proposal alias '{0}' already exists for '{1}'".format(proposalAlias, userName)
        user.tags[proposalAlias] = signedProposal



@when(u'user "{userName}" sends proposal "{proposalAlias}" to endorsers with timeout of "{timeout}" seconds with proposal responses "{proposalResponsesAlias}"')
def step_impl(context, userName, proposalAlias, timeout, proposalResponsesAlias):
    assert 'table' in context, "Expected table of endorsers"
    directory = bootstrap_util.getDirectory(context=context)
    user = directory.getUser(userName=userName)

    assert proposalAlias in user.tags, "Proposal alias '{0}' not found for user '{1}'".format(proposalAlias, userName)
    signedProposal = user.tags[proposalAlias]

    # Send proposal to each specified endorser, waiting 'timeout' seconds for response/error
    endorsers = [row['Endorser'] for row in context.table.rows]
    endorserStubs = endorser_util.getEndorserStubs(context, endorsers)
    proposalResponseFutures = [endorserStub.ProcessProposal.future(signedProposal, int(timeout)) for endorserStub in endorserStubs]
    resultsDict =  dict(zip(endorsers, [respFuture.result() for respFuture in proposalResponseFutures]))
    user.tags[proposalResponsesAlias] = resultsDict


@then(u'user "{userName}" expects proposal responses "{proposalResponsesAlias}" with status "{statusCode}" from endorsers')
def step_impl(context, userName, proposalResponsesAlias, statusCode):
    assert 'table' in context, "Expected table of endorsers"
    directory = bootstrap_util.getDirectory(context=context)
    user = directory.getUser(userName=userName)
    # Make sure proposalResponseAlias not already defined
    assert proposalResponsesAlias in user.tags, "Expected proposal responses at tag '{0}', for user '{1}'".format(proposalResponsesAlias, userName)
    proposalRespDict = user.tags[proposalResponsesAlias]

    # Loop through endorser proposal Responses
    endorsers = [row['Endorser'] for row in context.table.rows]
    print("Endorsers = {0}, rsults keys = {1}".format(endorsers, proposalRespDict.keys()))
    for respSatusCode in [proposalRespDict[endorser].response.status for endorser in endorsers]:
        assert int(statusCode) == respSatusCode, "Expected proposal response status code of {0} from {1}, received {2}".format(statusCode, endorser, respSatusCode)

@then(u'user "{userName}" expects proposal responses "{proposalResponsesAlias}" each have the same value from endorsers')
def step_impl(context, userName, proposalResponsesAlias):
    directory = bootstrap_util.getDirectory(context=context)
    user = directory.getUser(userName=userName)
    assert proposalResponsesAlias in user.tags, "Expected proposal responses at tag '{0}', for user '{1}'".format(proposalResponsesAlias, userName)
    proposalRespDict = user.tags[proposalResponsesAlias]
    assert len(proposalRespDict) > 0, "Expected at least 1 proposal response, found none in proposal responses dictionary"
    if len(proposalRespDict) == 1:
        pass
    else:
        endorsers = [row['Endorser'] for row in context.table.rows]
        endorserToProposalResponseHashDict = dict(zip(endorsers, [user.computeHash(proposalRespDict[endorser].payload) for endorser in endorsers]))
        setOfHashes = set(endorserToProposalResponseHashDict.values())
        assert len(setOfHashes) == 1, "Hashes from endorsers did NOT match: {0}".format(endorserToProposalResponseHashDict)
