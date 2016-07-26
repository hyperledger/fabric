import os
import re
import time
import copy
import base64
from datetime import datetime, timedelta

import sys, requests, json

import bdd_test_util

import bdd_grpc_util
from grpc.beta import implementations

import fabric_pb2
import chaincode_pb2
import devops_pb2

LAST_REQUESTED_TCERT="lastRequestedTCert"


@when(u'user "{enrollId}" requests a new application TCert')
def step_impl(context, enrollId):
	assert 'users' in context, "users not found in context. Did you register a user?"
	(channel, userRegistration) = bdd_grpc_util.getGRPCChannelAndUser(context, enrollId)
	
	stub = devops_pb2.beta_create_Devops_stub(channel)

	secret =  bdd_grpc_util.getSecretForUserRegistration(userRegistration)
	response = stub.EXP_GetApplicationTCert(secret,2)
	assert response.status == fabric_pb2.Response.SUCCESS, 'Failure getting TCert from {0}, for user "{1}":  {2}'.format(userRegistration.composeService,enrollId, response.msg)
	tcert = response.msg

	userRegistration.lastResult = tcert

@when(u'user "{enrollId}" stores their last result as "{tagName}"')
def step_impl(context, enrollId, tagName):
	assert 'users' in context, "users not found in context. Did you register a user?"
	# Retrieve the userRegistration from the context
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	userRegistration.tags[tagName] = userRegistration.lastResult

@when(u'user "{enrollId}" sets metadata to their stored value "{tagName}"')
def step_impl(context, enrollId, tagName):
	assert 'users' in context, "users not found in context. Did you register a user?"
	# Retrieve the userRegistration from the context
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	assert tagName in userRegistration.tags, 'Tag "{0}" not found in user "{1}" tags'.format(tagName, enrollId)
	context.metadata = userRegistration.tags[tagName] 


@when(u'user "{enrollId}" deploys chaincode "{chaincodePath}" aliased as "{ccAlias}" with ctor "{ctor}" and args')
def step_impl(context, enrollId, chaincodePath, ccAlias, ctor):
	bdd_grpc_util.deployChaincode(context, enrollId, chaincodePath, ccAlias, ctor)


@when(u'user "{enrollId}" gives stored value "{tagName}" to "{recipientEnrollId}"')
def step_impl(context, enrollId, tagName, recipientEnrollId):
	assert 'users' in context, "users not found in context. Did you register a user?"
	# Retrieve the userRegistration from the context
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	recipientUserRegistration = bdd_test_util.getUserRegistration(context, recipientEnrollId)
	# Copy value from target to recipient
	recipientUserRegistration.tags[tagName] = userRegistration.tags[tagName]


@when(u'"{enrollId}" uses application TCert "{assignerAppTCert}" to assign role "{role}" to application TCert "{assigneeAppTCert}"')
def step_impl(context, enrollId, assignerAppTCert, role, assigneeAppTCert):
	assert 'users' in context, "users not found in context. Did you register a user?"
	assert 'compose_containers' in context, "compose_containers not found in context"

	(channel, userRegistration) = bdd_grpc_util.getGRPCChannelAndUser(context, enrollId)

	stub = devops_pb2.beta_create_Devops_stub(channel)

	# First get binding with EXP_PrepareForTx
	secret = bdd_grpc_util.getSecretForUserRegistration(userRegistration)
	response = stub.EXP_PrepareForTx(secret,2)
	assert response.status == fabric_pb2.Response.SUCCESS, 'Failure getting Binding from {0}, for user "{1}":  {2}'.format(userRegistration.composeService,enrollId, response.msg)
	binding = response.msg

	# Now produce the sigma EXP_ProduceSigma
	chaincodeInput = chaincode_pb2.ChaincodeInput(function = "addRole", args = (base64.b64encode(userRegistration.tags[assigneeAppTCert]), role) ) 
	chaincodeInputRaw = chaincodeInput.SerializeToString()
	appTCert = userRegistration.tags[assignerAppTCert]
	sigmaInput = devops_pb2.SigmaInput(secret = secret, appTCert = appTCert,  data = chaincodeInputRaw + binding)
	response = stub.EXP_ProduceSigma(sigmaInput,2)
	assert response.status == fabric_pb2.Response.SUCCESS, 'Failure prducing sigma from {0}, for user "{1}":  {2}'.format(userRegistration.composeService,enrollId, response.msg)
	sigmaOutputBytes = response.msg
	# Parse the msg bytes as a SigmaOutput message
	sigmaOutput = devops_pb2.SigmaOutput()
	sigmaOutput.ParseFromString(sigmaOutputBytes)
	print('Length of sigma = {0}'.format(len(sigmaOutput.sigma)))
	
	# Now execute the transaction with the saved binding, EXP_ExecuteWithBinding
	assert "grpcChaincodeSpec" in context, "grpcChaincodeSpec NOT found in context"
	newChaincodeSpec = chaincode_pb2.ChaincodeSpec()
	newChaincodeSpec.CopyFrom(context.grpcChaincodeSpec)
	newChaincodeSpec.metadata = sigmaOutput.asn1Encoding
	newChaincodeSpec.ctorMsg.CopyFrom(chaincodeInput)

	ccInvocationSpec = chaincode_pb2.ChaincodeInvocationSpec(chaincodeSpec = newChaincodeSpec)

	executeWithBinding = devops_pb2.ExecuteWithBinding(chaincodeInvocationSpec = ccInvocationSpec, binding = binding)

	response = stub.EXP_ExecuteWithBinding(executeWithBinding,60)
	assert response.status == fabric_pb2.Response.SUCCESS, 'Failure executeWithBinding from {0}, for user "{1}":  {2}'.format(userRegistration.composeService,enrollId, response.msg)
	context.response = response
	context.transactionID = response.msg


@then(u'"{enrollId}"\'s last transaction should have failed with message that contains "{msg}"')
def step_impl(context, enrollId, msg):
	assert 'users' in context, "users not found in context. Did you register a user?"
	assert 'compose_containers' in context, "compose_containers not found in context"
	txResult = bdd_grpc_util.getTxResult(context, enrollId)
	assert txResult.errorCode > 0, "Expected failure (errorCode > 0), instead found errorCode={0}".format(txResult.errorCode)
	assert msg in txResult.error, "Expected error to contain'{0}', instead found '{1}".format(msg, txResult.error)

@then(u'"{enrollId}"\'s last transaction should have succeeded')
def step_impl(context, enrollId):
	txResult = bdd_grpc_util.getTxResult(context, enrollId)
	assert txResult.errorCode == 0, "Expected success (errorCode == 0), instead found errorCode={0}, error={1}".format(txResult.errorCode, txResult.error)

@when(u'user "{enrollId}" invokes chaincode "{ccAlias}" function name "{functionName}" with args')
def step_impl(context, enrollId, ccAlias, functionName):
	response = bdd_grpc_util.invokeChaincode(context, enrollId, ccAlias, functionName)
	context.response = response
	context.transactionID = response.msg
	#assert response.status == fabric_pb2.Response.SUCCESS, 'Failure invoking chaincode {0} on {1}, for user "{2}":  {3}'.format(ccAlias, userRegistration.composeService,enrollId, response.msg)

@given(u'user "{enrollId}" stores a reference to chaincode "{ccAlias}" as "{tagName}"')
def step_impl(context, enrollId, ccAlias, tagName):
	# Retrieve the userRegistration from the context
	userRegistration = bdd_test_util.getUserRegistration(context, enrollId)
	deployedCcSpec = bdd_grpc_util.getDeployment(context, ccAlias)
	assert deployedCcSpec != None, "Deployment NOT found for chaincode alias '{0}'".format(ccAlias)
	userRegistration.tags[tagName] = deployedCcSpec.chaincodeID.name
