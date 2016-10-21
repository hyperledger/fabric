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

import endorser_util
import bdd_grpc_util
import bdd_test_util
import devops_pb2

@when(u'user "{enrollId}" creates a chaincode spec "{ccSpecAlias}" of type "{ccType}" for chaincode "{chaincodePath}" with args')
def step_impl(context, enrollId, ccType, chaincodePath, ccSpecAlias):
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	args =  bdd_grpc_util.getArgsFromContextForUser(context, enrollId)
	ccSpec = endorser_util.getChaincodeSpec(ccType, chaincodePath, bdd_grpc_util.toStringArray(args))
	print("ccSpec = {0}".format(ccSpec))
	userRegistration.tags[ccSpecAlias] = ccSpec


@when(u'user "{enrollId}" creates a deployment spec "{ccDeploymentSpecAlias}" using chaincode spec "{ccSpecAlias}" and devops on peer "{devopsComposeService}"')
def step_impl(context, enrollId, ccDeploymentSpecAlias, ccSpecAlias, devopsComposeService):
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	assert ccSpecAlias in userRegistration.tags, "ChaincodeSpec alias '{0}' not found for user '{1}'".format(ccSpecAlias, enrollId)

	ipAddress = bdd_test_util.ipFromContainerNamePart(devopsComposeService, context.compose_containers)
	channel = bdd_grpc_util.getGRPCChannel(ipAddress)
	devopsStub = devops_pb2.beta_create_Devops_stub(channel)
	deploymentSpec = devopsStub.Build(userRegistration.tags[ccSpecAlias],20)
	userRegistration.tags[ccDeploymentSpecAlias] = deploymentSpec


@when(u'user "{enrollId}" creates a deployment proposal "{proposalAlias}" using chaincode deployment spec "{ccDeploymentSpecAlias}"')
def step_impl(context, enrollId, proposalAlias, ccDeploymentSpecAlias):
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	assert ccDeploymentSpecAlias in userRegistration.tags, "ChaincodeDeploymentSpec alias '{0}' not found for user '{1}'".format(ccDeploymentSpecAlias, enrollId)
	ccDeploymentSpec = userRegistration.tags[ccDeploymentSpecAlias]
	proposal = endorser_util.createDeploymentProposalForBDD(ccDeploymentSpec)
	assert not proposalAlias in userRegistration.tags, "Proposal alias '{0}' already exists for '{1}'".format(proposalAlias, enrollId)
	userRegistration.tags[proposalAlias] = proposal



@when(u'user "{enrollId}" sends proposal "{proposalAlias}" to endorsers with timeout of "{timeout}" seconds')
def step_impl(context, enrollId, proposalAlias, timeout):
	assert 'table' in context, "Expected table of endorsers"
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	assert proposalAlias in userRegistration.tags, "Proposal alias '{0}' not found for user '{1}'".format(proposalAlias, enrollId)
	proposal = userRegistration.tags[proposalAlias]

	# Send proposal to each specified endorser, waiting 'timeout' seconds for response/error
	endorsers = context.table.headings
	proposalResponseFutures = [endorserStub.ProcessProposal.future(proposal, int(timeout)) for endorserStub in endorser_util.getEndorserStubs(context, endorsers)]
	resultsDict =  dict(zip(endorsers, [respFuture.result() for respFuture in proposalResponseFutures]))
	userRegistration.lastResult = resultsDict


@then(u'user "{enrollId}" expects proposal responses "{proposalResponsesAlias}" with status "{statusCode}" from endorsers')
def step_impl(context, enrollId, proposalResponsesAlias, statusCode):
	assert 'table' in context, "Expected table of endorsers"
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	# Make sure proposalResponseAlias not already defined
	assert proposalResponsesAlias in userRegistration.tags, "Expected proposal responses at tag '{0}', for user '{1}'".format(proposalResponsesAlias, enrollId)
	proposalRespDict = userRegistration.tags[proposalResponsesAlias]

	# Loop through endorser proposal Responses
	endorsers = context.table.headings
	for respSatusCode in [proposalRespDict[endorser].response.status for endorser in endorsers]:
		assert int(statusCode) == respSatusCode, "Expected proposal response status code of {0} from {1}, received {2}".format(statusCode, endorser, respSatusCode)
