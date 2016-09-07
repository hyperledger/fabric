#
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
import os.path
import time
import copy
from behave import *
from datetime import datetime, timedelta
import base64

import requests, json

import bdd_compose_util
import bdd_test_util
from bdd_test_util import bdd_log
from bdd_rest_util import buildUrl
from bdd_json_util import getAttributeFromJSON

JSONRPC_VERSION = "2.0"

@given(u'we compose "{composeYamlFile}"')
def step_impl(context, composeYamlFile):
    context.compose_yaml = composeYamlFile
    fileArgsToDockerCompose = bdd_compose_util.getDockerComposeFileArgsFromYamlFile(context.compose_yaml)
    context.compose_output, context.compose_error, context.compose_returncode = \
        bdd_test_util.cli_call(["docker-compose"] + fileArgsToDockerCompose + ["up","--force-recreate", "-d"], expect_success=True)
    assert context.compose_returncode == 0, "docker-compose failed to bring up {0}".format(composeYamlFile)

    bdd_compose_util.parseComposeOutput(context)

    timeoutSeconds = 15
    assert bdd_compose_util.allContainersAreReadyWithinTimeout(context, timeoutSeconds), \
        "Containers did not come up within {} seconds, aborting".format(timeoutSeconds)

    context.containerAliasMap = bdd_compose_util.mapAliasesToContainers(context)
    context.containerNameMap = bdd_compose_util.mapContainerNamesToContainers(context)

@when(u'requesting "{path}" from "{containerAlias}"')
def step_impl(context, path, containerAlias):
    ipAddress = context.containerAliasMap[containerAlias].ipAddress
    request_url = buildUrl(context, ipAddress, path)
    bdd_log("Requesting path = {0}".format(request_url))
    resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
    assert resp.status_code == 200, "Failed to GET url %s:  %s" % (request_url,resp.text)
    context.response = resp
    bdd_log("")

@then(u'I should get a JSON response containing "{attribute}" attribute')
def step_impl(context, attribute):
    getAttributeFromJSON(attribute, context.response.json(), "Attribute not found in response (%s)" %(attribute))

@then(u'I should get a JSON response containing no "{attribute}" attribute')
def step_impl(context, attribute):
    try:
        getAttributeFromJSON(attribute, context.response.json(), "")
        assert None, "Attribute found in response (%s)" %(attribute)
    except AssertionError:
        bdd_log("Attribute not found as was expected.")

def formatStringToCompare(value):
    # double quotes are replaced by simple quotes because is not possible escape double quotes in the attribute parameters.
    return str(value).replace("\"", "'")

@then(u'I should get a JSON response with "{attribute}" = "{expectedValue}"')
def step_impl(context, attribute, expectedValue):
    foundValue = getAttributeFromJSON(attribute, context.response.json(), "Attribute not found in response (%s)" %(attribute))
    assert (formatStringToCompare(foundValue) == expectedValue), "For attribute %s, expected (%s), instead found (%s)" % (attribute, expectedValue, foundValue)

@then(u'I should get a JSON response with array "{attribute}" contains "{expectedValue}" elements')
def step_impl(context, attribute, expectedValue):
    foundValue = getAttributeFromJSON(attribute, context.response.json(), "Attribute not found in response (%s)" %(attribute))
    assert (len(foundValue) == int(expectedValue)), "For attribute %s, expected array of size (%s), instead found (%s)" % (attribute, expectedValue, len(foundValue))

@given(u'I wait "{seconds}" seconds')
def step_impl(context, seconds):
    time.sleep(float(seconds))

@when(u'I wait "{seconds}" seconds')
def step_impl(context, seconds):
    time.sleep(float(seconds))

@then(u'I wait "{seconds}" seconds')
def step_impl(context, seconds):
    time.sleep(float(seconds))

@when(u'I deploy lang chaincode "{chaincodePath}" of "{chainLang}" with ctor "{ctor}" to "{containerAlias}"')
def step_impl(context, chaincodePath, chainLang, ctor, containerAlias):
    bdd_log("Printing chaincode language " + chainLang)

    chaincode = {
        "path": chaincodePath,
        "language": chainLang,
        "constructor": ctor,
        "args": getArgsFromContext(context),
    }

    deployChainCodeToContainer(context, chaincode, containerAlias)

def getArgsFromContext(context):
    args = []
    if 'table' in context:
       # There is ctor arguments
       args = context.table[0].cells
    return args

@when(u'I deploy chaincode "{chaincodePath}" with ctor "{ctor}" to "{containerAlias}"')
def step_impl(context, chaincodePath, ctor, containerAlias):
    chaincode = {
        "path": chaincodePath,
        "language": "GOLANG",
        "constructor": ctor,
        "args": getArgsFromContext(context),
    }

    deployChainCodeToContainer(context, chaincode, containerAlias)

@when(u'I deploy chaincode with name "{chaincodeName}" and with ctor "{ctor}" to "{containerAlias}"')
def step_impl(context, chaincodeName, ctor, containerAlias):
    chaincode = {
        "name": chaincodeName,
        "language": "GOLANG",
        "constructor": ctor,
        "args": getArgsFromContext(context),
    }

    deployChainCodeToContainer(context, chaincode, containerAlias)
    time.sleep(2.0) # After #2068 implemented change this to only apply after a successful ping

def deployChainCodeToContainer(context, chaincode, containerAlias):
    ipAddress = context.containerAliasMap[containerAlias].ipAddress
    request_url = buildUrl(context, ipAddress, "/chaincode")
    bdd_log("Requesting path = {0}".format(request_url))

    chaincodeSpec = createChaincodeSpec(context, chaincode)
    chaincodeOpPayload = createChaincodeOpPayload("deploy", chaincodeSpec)

    resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(chaincodeOpPayload), verify=False)
    assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
    context.response = resp
    chaincodeName = resp.json()['result']['message']
    chaincodeSpec['chaincodeID']['name'] = chaincodeName
    context.chaincodeSpec = chaincodeSpec
    bdd_log(json.dumps(chaincodeSpec, indent=4))
    bdd_log("")

def createChaincodeSpec(context, chaincode):
    chaincode = validateChaincodeDictionary(chaincode)
    args = prepend(chaincode["constructor"], chaincode["args"])
    # Create a ChaincodeSpec structure
    chaincodeSpec = {
        "type": getChaincodeTypeValue(chaincode["language"]),
        "chaincodeID": {
            "path" : chaincode["path"],
            "name" : chaincode["name"]
        },
        "ctorMsg":  {
            "args" : args
        },
    }

    if 'userName' in context:
        chaincodeSpec["secureContext"] = context.userName
    if 'metadata' in context:
        chaincodeSpec["metadata"] = context.metadata

    return chaincodeSpec

def validateChaincodeDictionary(chaincode):
    chaincodeFields = ["path", "name", "language", "constructor", "args"]

    for field in chaincodeFields:
        if field not in chaincode:
            chaincode[field] = ""

    return chaincode

def getChaincodeTypeValue(chainLang):
    if chainLang == "GOLANG":
        return 1
    elif chainLang =="JAVA":
        return 4
    elif chainLang == "NODE":
        return 2
    elif chainLang == "CAR":
        return 3
    elif chainLang == "UNDEFINED":
        return 0
    return 1

@when(u'I mock deploy chaincode with name "{chaincodeName}"')
def step_impl(context, chaincodeName):
    chaincode = {
        "name": chaincodeName,
        "language": "GOLANG"
    }

    context.chaincodeSpec = createChaincodeSpec(context, chaincode)

@then(u'I should have received a chaincode name')
def step_impl(context):
    if 'chaincodeSpec' in context:
        assert context.chaincodeSpec['chaincodeID']['name'] != ""
        # Set the current transactionID to the name passed back
        context.transactionID = context.chaincodeSpec['chaincodeID']['name']
    elif 'grpcChaincodeSpec' in context:
        assert context.grpcChaincodeSpec.chaincodeID.name != ""
        # Set the current transactionID to the name passed back
        context.transactionID = context.grpcChaincodeSpec.chaincodeID.name
    else:
        fail('chaincodeSpec not in context')

@when(u'I invoke chaincode "{chaincodeName}" function name "{functionName}" on "{containerAlias}" with "{idGenAlg}"')
def step_impl(context, chaincodeName, functionName, containerAlias, idGenAlg):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    invokeChaincode(context, "invoke", functionName, containerAlias, idGenAlg)

@when(u'I invoke chaincode "{chaincodeName}" function name "{functionName}" on "{containerAlias}" "{times}" times')
def step_impl(context, chaincodeName, functionName, containerAlias, times):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    ipAddress = context.containerAliasMap[containerAlias].ipAddress
    request_url = buildUrl(context, ipAddress, "/chain")
    resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
    assert resp.status_code == 200, "Failed to get chain height %s:  %s" % (request_url,resp.text)
    context.chainheight = getAttributeFromJSON("height", resp.json(), "Height not found in response.")
    context.txcount = times
    for i in range(int(times)):
        invokeChaincode(context, "invoke", functionName, containerAlias)

@when(u'I invoke chaincode "{chaincodeName}" function name "{functionName}" with attributes "{attrs}" on "{containerAlias}"')
def step_impl(context, chaincodeName, functionName, attrs, containerAlias):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    assert attrs, "attrs were not specified"
    invokeChaincode(context, "invoke", functionName, containerAlias, None, attrs.split(","))

@when(u'I invoke chaincode "{chaincodeName}" function name "{functionName}" on "{containerAlias}"')
def step_impl(context, chaincodeName, functionName, containerAlias):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    invokeChaincode(context, "invoke", functionName, containerAlias)

@when(u'I invoke master chaincode "{chaincodeName}" function name "{functionName}" on "{containerAlias}"')
def step_impl(context, chaincodeName, functionName, containerAlias):
    invokeMasterChaincode(context, "invoke", chaincodeName, functionName, containerAlias)

@then(u'I should have received a transactionID')
def step_impl(context):
    assert 'transactionID' in context, 'transactionID not found in context'
    assert context.transactionID != ""
    pass

@when(u'I unconditionally query chaincode "{chaincodeName}" function name "{functionName}" on "{containerAlias}"')
def step_impl(context, chaincodeName, functionName, containerAlias):
    invokeChaincode(context, "query", functionName, containerAlias)

@when(u'I query chaincode "{chaincodeName}" function name "{functionName}" on "{containerAlias}"')
def step_impl(context, chaincodeName, functionName, containerAlias):
    invokeChaincode(context, "query", functionName, containerAlias)

def createChaincodeOpPayload(method, chaincodeSpec):
    chaincodeOpPayload = {
        "jsonrpc": JSONRPC_VERSION,
        "method" : method,
        "params" : chaincodeSpec,
        "id"     : 1
    }
    return chaincodeOpPayload

def invokeChaincode(context, devopsFunc, functionName, containerAlias, idGenAlg=None, attributes=[]):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    # Update the chaincodeSpec ctorMsg for invoke
    args = []
    if 'table' in context:
       # There is ctor arguments
       args = context.table[0].cells
    args = prepend(functionName, args)
    for idx, attr in enumerate(attributes):
        attributes[idx] = attr.strip()

    context.chaincodeSpec['attributes'] = attributes

    #If idGenAlg is passed then, we still using the deprecated devops API because this parameter can't be passed in the new API.
    if idGenAlg != None:
        context.chaincodeSpec['ctorMsg']['args'] = to_bytes(args)
        invokeUsingDevopsService(context, devopsFunc, functionName, containerAlias, idGenAlg)
    else:
        context.chaincodeSpec['ctorMsg']['args'] = args
        invokeUsingChaincodeService(context, devopsFunc, functionName, containerAlias)

def invokeUsingChaincodeService(context, devopsFunc, functionName, containerAlias):
    # Invoke the POST
    chaincodeOpPayload = createChaincodeOpPayload(devopsFunc, context.chaincodeSpec)

    ipAddress = context.containerAliasMap[containerAlias].ipAddress

    request_url = buildUrl(context, ipAddress, "/chaincode")
    bdd_log("POSTing path = {}".format(request_url))
    bdd_log("Using attributes {0}".format(context.chaincodeSpec['attributes']))

    resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(chaincodeOpPayload), verify=False)
    assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
    context.response = resp
    bdd_log("RESULT from {0} of chaincode from peer {1}".format(functionName, containerAlias))
    bdd_log(json.dumps(context.response.json(), indent = 4))
    if 'result' in resp.json():
        result = resp.json()['result']
        if 'message' in result:
            transactionID = result['message']
            context.transactionID = transactionID

def invokeUsingDevopsService(context, devopsFunc, functionName, containerAlias, idGenAlg):
    # Invoke the POST
    chaincodeInvocationSpec = {
        "chaincodeSpec" : context.chaincodeSpec
    }
    ipAddress = context.containerAliasMap[containerAlias].ipAddress
    if idGenAlg is not None:
	    chaincodeInvocationSpec['idGenerationAlg'] = idGenAlg
    request_url = buildUrl(context, ipAddress, "/devops/{0}".format(devopsFunc))
    bdd_log("POSTing path = {}".format(request_url))

    resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(chaincodeInvocationSpec), verify=False)
    assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
    context.response = resp
    bdd_log("RESULT from {0} of chaincode from peer {1}".format(functionName, containerAlias))
    bdd_log(json.dumps(context.response.json(), indent = 4))
    if 'message' in resp.json():
        transactionID = context.response.json()['message']
        context.transactionID = transactionID

def invokeMasterChaincode(context, devopsFunc, chaincodeName, functionName, containerAlias):
    args = []
    if 'table' in context:
       args = context.table[0].cells
    args = prepend(functionName, args)
    typeGolang = 1
    chaincodeSpec = {
        "type": typeGolang,
        "chaincodeID": {
            "name" : chaincodeName
        },
        "ctorMsg":  {
            "args" : args
        }
    }
    if 'userName' in context:
        chaincodeSpec["secureContext"] = context.userName

    chaincodeOpPayload = createChaincodeOpPayload(devopsFunc, chaincodeSpec)

    ipAddress = context.containerAliasMap[containerAlias].ipAddress
    request_url = buildUrl(context, ipAddress, "/chaincode")
    bdd_log("POSTing path = {}".format(request_url))

    resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(chaincodeOpPayload), verify=False)
    assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
    context.response = resp
    bdd_log("RESULT from {0} of chaincode from peer {1}".format(functionName, containerAlias))
    bdd_log(json.dumps(context.response.json(), indent = 4))
    if 'result' in resp.json():
        result = resp.json()['result']
        if 'message' in result:
            transactionID = result['message']
            context.transactionID = transactionID

@then(u'I wait "{seconds}" seconds for chaincode to build')
def step_impl(context, seconds):
    """ This step takes into account the chaincodeImagesUpToDate tag, in which case the wait is reduce to some default seconds"""
    reducedWaitTime = 4
    if 'chaincodeImagesUpToDate' in context.tags:
        bdd_log("Assuming images are up to date, sleeping for {0} seconds instead of {1} in scenario {2}".format(reducedWaitTime, seconds, context.scenario.name))
        time.sleep(float(reducedWaitTime))
    else:
        time.sleep(float(seconds))

@then(u'I wait "{seconds}" seconds for transaction to be committed to block on "{containerAlias}"')
def step_impl(context, seconds, containerAlias):
    assert 'transactionID' in context, "transactionID not found in context"
    ipAddress = context.containerAliasMap[containerAlias].ipAddress
    request_url = buildUrl(context, ipAddress, "/transactions/{0}".format(context.transactionID))
    bdd_log("GETing path = {}".format(request_url))

    resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
    assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
    context.response = resp

def multiRequest(context, seconds, containerDataList, pathBuilderFunc):
    """Perform a multi request against the system"""
    # Build map of "containerName" : response
    respMap = {container.name:None for container in containerDataList}
    # Set the max time before stopping attempts
    maxTime = datetime.now() + timedelta(seconds = int(seconds))
    for container in containerDataList:
        ipAddress = container.ipAddress
        request_url = buildUrl(context, ipAddress, pathBuilderFunc(context, container))

        # Loop unless failure or time exceeded
        while (datetime.now() < maxTime):
            bdd_log("GETing path = {}".format(request_url))
            resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
            respMap[container.name] = resp
        else:
            raise Exception("Max time exceeded waiting for multiRequest with current response map = {0}".format(respMap))

@then(u'I wait up to "{seconds}" seconds for transaction to be committed to all peers')
def step_impl(context, seconds):
    assert 'transactionID' in context, "transactionID not found in context"
    assert 'compose_containers' in context, "compose_containers not found in context"

    # Build map of "containerName" : resp.statusCode
    respMap = {container.name:0 for container in context.compose_containers}

    # Set the max time before stopping attempts
    maxTime = datetime.now() + timedelta(seconds = int(seconds))
    for container in context.compose_containers:
        ipAddress = container.ipAddress
        request_url = buildUrl(context, ipAddress, "/transactions/{0}".format(context.transactionID))

        # Loop unless failure or time exceeded
        while (datetime.now() < maxTime):
            bdd_log("GETing path = {}".format(request_url))
            resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
            if resp.status_code == 404:
                # Pause then try again
                respMap[container.name] = 404
                time.sleep(1)
                continue
            elif resp.status_code == 200:
                # Success, continue
                respMap[container.name] = 200
                break
            else:
                raise Exception("Error requesting {0}, returned result code = {1}".format(request_url, resp.status_code))
        else:
            raise Exception("Max time exceeded waiting for transactions with current response map = {0}".format(respMap))
    bdd_log("Result of request to all peers = {0}".format(respMap))
    bdd_log("")

@then(u'I check the transaction ID if it is "{tUUID}"')
def step_impl(context, tUUID):
    assert 'transactionID' in context, "transactionID not found in context"
    assert context.transactionID == tUUID, "transactionID is not tUUID"

@then(u'I wait up to "{seconds}" seconds for transaction to be committed to peers')
def step_impl(context, seconds):
    assert 'transactionID' in context, "transactionID not found in context"
    assert 'compose_containers' in context, "compose_containers not found in context"
    assert 'table' in context, "table (of peers) not found in context"

    # Set the max time before stopping attempts
    maxTime = datetime.now() + timedelta(seconds = int(seconds))
    respMap = {}

    aliases =  context.table.headings
    for containerAlias in aliases:
        container = context.containerAliasMap[containerAlias]
        respMap[container.name] = 0

        ipAddress = container.ipAddress
        request_url = buildUrl(context, ipAddress, "/transactions/{0}".format(context.transactionID))

        # Loop unless failure or time exceeded
        while (datetime.now() < maxTime):
            bdd_log("GETing path = {}".format(request_url))
            resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
            if resp.status_code == 404:
                # Pause then try again
                respMap[container.name] = 404
                time.sleep(1)
                continue
            elif resp.status_code == 200:
                # Success, continue
                respMap[container.name] = 200
                break
            else:
                raise Exception("Error requesting {0}, returned result code = {1}".format(request_url, resp.status_code))
        else:
            raise Exception("Max time exceeded waiting for transactions with current response map = {0}".format(respMap))
    bdd_log("Result of request to all peers = {0}".format(respMap))
    bdd_log("")

@then(u'I wait up to "{seconds}" seconds for transactions to be committed to peers')
def step_impl(context, seconds):
    assert 'chainheight' in context, "chainheight not found in context"
    assert 'txcount' in context, "txcount not found in context"
    assert 'compose_containers' in context, "compose_containers not found in context"
    assert 'table' in context, "table (of peers) not found in context"

    # Set the max time before stopping attempts
    maxTime = datetime.now() + timedelta(seconds = int(seconds))
    responseStatusCodeMap = {}

    aliases =  context.table.headings
    for containerAlias in aliases:
        container = context.containerAliasMap[containerAlias]
        ipAddress = container.ipAddress
        request_url = buildUrl(context, ipAddress, "/chain")

        # Loop unless failure or time exceeded
        while (datetime.now() < maxTime):
            bdd_log("GETing path = {}".format(request_url))
            resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
            if resp.status_code == 404:
                # Pause then try again
                responseStatusCodeMap[container.name] = 404
                time.sleep(1)
                continue
            elif resp.status_code == 200:
                height = getAttributeFromJSON("height", resp.json(), "Height not found in response.")
		if height >= int(context.chainheight) + int(context.txcount):
                        # Success, continue
                        responseStatusCodeMap[container.name] = 200
                        break
		else:
		        continue
            else:
                raise Exception("Error requesting {0}, returned result code = {1}".format(request_url, resp.status_code))
        else:
            raise Exception("Max time exceeded waiting for transactions with current response map = {0}".format(respMap))
    bdd_log("Result of request to all peers = {0}".format(responseStatusCodeMap))
    bdd_log("")


@then(u'I should get a rejection message in the listener after stopping it')
def step_impl(context):
    assert "eventlistener" in context, "no eventlistener is started"
    context.eventlistener.terminate()
    output = context.eventlistener.stdout.read()
    rejection = "Received rejected transaction"
    assert rejection in output, "no rejection message was found"
    assert output.count(rejection) == 1, "only one rejection message should be found"


@when(u'I query chaincode "{chaincodeName}" function name "{functionName}" on all peers')
def step_impl(context, chaincodeName, functionName):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    assert 'compose_containers' in context, "compose_containers not found in context"
    # Update the chaincodeSpec ctorMsg for invoke
    args = []
    if 'table' in context:
       # There is ctor arguments
       args = context.table[0].cells
    args = prepend(functionName, args)
    context.chaincodeSpec['ctorMsg']['args'] = args #context.table[0].cells if ('table' in context) else []
    # Invoke the POST
    chaincodeOpPayload = createChaincodeOpPayload("query", context.chaincodeSpec)

    responses = []
    for container in context.compose_containers:
        request_url = buildUrl(context, container.ipAddress, "/chaincode")
        bdd_log("POSTing path = {}".format(request_url))
        resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(chaincodeOpPayload), verify=False)
        assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
        responses.append(resp)
    context.responses = responses

@when(u'I unconditionally query chaincode "{chaincodeName}" function name "{functionName}" with value "{value}" on peers')
def step_impl(context, chaincodeName, functionName, value):
    query_common(context, chaincodeName, functionName, value, False)

@when(u'I query chaincode "{chaincodeName}" function name "{functionName}" with value "{value}" on peers')
def step_impl(context, chaincodeName, functionName, value):
    query_common(context, chaincodeName, functionName, value, True)

def query_common(context, chaincodeName, functionName, value, failOnError):
    assert 'chaincodeSpec' in context, "chaincodeSpec not found in context"
    assert 'compose_containers' in context, "compose_containers not found in context"
    assert 'table' in context, "table (of peers) not found in context"
    assert 'peerToSecretMessage' in context, "peerToSecretMessage map not found in context"

    # Update the chaincodeSpec ctorMsg for invoke
    context.chaincodeSpec['ctorMsg']['args'] = [functionName, value]
    # Invoke the POST
    # Make deep copy of chaincodeSpec as we will be changing the SecurityContext per call.
    chaincodeOpPayload = createChaincodeOpPayload("query", copy.deepcopy(context.chaincodeSpec))

    responses = []
    aliases =  context.table.headings
    for containerAlias in aliases:
        container = context.containerAliasMap[containerAlias]

        # Change the SecurityContext per call
        chaincodeOpPayload['params']["secureContext"] = context.peerToSecretMessage[container.composeService]['enrollId']
        bdd_log("Container {0} enrollID = {1}".format(container.name, container.getEnv("CORE_SECURITY_ENROLLID")))
        request_url = buildUrl(context, container.ipAddress, "/chaincode")
        bdd_log("POSTing path = {}".format(request_url))
        resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(chaincodeOpPayload), timeout=30, verify=False)
        if failOnError:
            assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
        bdd_log("RESULT from {0} of chaincode from peer {1}".format(functionName, container.name))
        bdd_log(json.dumps(resp.json(), indent = 4))
        responses.append(resp)
    context.responses = responses

@then(u'I should get a JSON response from all peers with "{attribute}" = "{expectedValue}"')
def step_impl(context, attribute, expectedValue):
    assert 'responses' in context, "responses not found in context"
    for resp in context.responses:
        foundValue = getAttributeFromJSON(attribute, resp.json(), "Attribute not found in response (%s)" %(attribute))
        assert (formatStringToCompare(foundValue) == expectedValue), "For attribute %s, expected (%s), instead found (%s)" % (attribute, expectedValue, foundValue)

@then(u'I should get a JSON response from peers with "{attribute}" = "{expectedValue}"')
def step_impl(context, attribute, expectedValue):
    assert 'responses' in context, "responses not found in context"
    assert 'compose_containers' in context, "compose_containers not found in context"
    assert 'table' in context, "table (of peers) not found in context"

    for resp in context.responses:
        foundValue = getAttributeFromJSON(attribute, resp.json(), "Attribute not found in response (%s)" %(attribute))
        assert (formatStringToCompare(foundValue) == expectedValue), "For attribute %s, expected (%s), instead found (%s)" % (attribute, expectedValue, foundValue)

@given(u'I register with CA supplying username "{userName}" and secret "{secret}" on peers')
def step_impl(context, userName, secret):
    assert 'compose_containers' in context, "compose_containers not found in context"
    assert 'table' in context, "table (of peers) not found in context"

    secretMsg = {
        "enrollId": userName,
        "enrollSecret" : secret
    }

    # Login to each container specified
    aliases =  context.table.headings
    for containerAlias in aliases:
        containerData = context.containerAliasMap[containerAlias]
        request_url = buildUrl(context, containerData.ipAddress, "/registrar")
        bdd_log("POSTing path = {}".format(request_url))

        resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(secretMsg), verify=False)
        assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
        context.response = resp
        bdd_log("message = {0}".format(resp.json()))

        # Create new User entry
        bdd_test_util.registerUser(context, secretMsg, containerData.composeService)

    # Store the username in the context
    context.userName = userName
    # if we already have the chaincodeSpec, change secureContext
    if 'chaincodeSpec' in context:
        context.chaincodeSpec["secureContext"] = context.userName


@given(u'I use the following credentials for querying peers')
def step_impl(context):
    assert 'compose_containers' in context, "compose_containers not found in context"
    assert 'table' in context, "table (of peers, username, secret) not found in context"

    peerToSecretMessage = {}

    # Login to each container specified using username and secret
    for row in context.table.rows:
        peer, userName, secret = row['peer'], row['username'], row['secret']
        secretMsg = {
            "enrollId": userName,
            "enrollSecret" : secret
        }

        ipAddress = context.containerAliasMap[peer].ipAddress
        request_url = buildUrl(context, ipAddress, "/registrar")
        bdd_log("POSTing to service = {0}, path = {1}".format(peer, request_url))

        resp = requests.post(request_url, headers={'Content-type': 'application/json'}, data=json.dumps(secretMsg), verify=False)
        assert resp.status_code == 200, "Failed to POST to %s:  %s" %(request_url, resp.text)
        context.response = resp
        bdd_log("message = {0}".format(resp.json()))
        peerToSecretMessage[peer] = secretMsg
    context.peerToSecretMessage = peerToSecretMessage


@given(u'I stop peers')
def step_impl(context):
    compose_op(context, "stop")


@given(u'I start a listener')
def step_impl(context):
    gopath = os.environ.get('GOPATH')
    assert gopath is not None, "Please set GOPATH properly!"
    listener = os.path.join(gopath, "src/github.com/hyperledger/fabric/build/bin/block-listener")
    assert os.path.isfile(listener), "Please build the block-listener binary!"
    bdd_test_util.start_background_process(context, "eventlistener", [listener, "-listen-to-rejections"] )


@given(u'I start peers')
def step_impl(context):
    compose_op(context, "start")

@given(u'I pause peers')
def step_impl(context):
    compose_op(context, "pause")

@given(u'I unpause peers')
def step_impl(context):
    compose_op(context, "unpause")

def compose_op(context, op):
    assert 'table' in context, "table (of peers) not found in context"
    assert 'compose_yaml' in context, "compose_yaml not found in context"

    fileArgsToDockerCompose = bdd_compose_util.getDockerComposeFileArgsFromYamlFile(context.compose_yaml)
    services =  context.table.headings
    # Loop through services and start/stop them, and modify the container data list if successful.
    for service in services:
       context.compose_output, context.compose_error, context.compose_returncode = \
           bdd_test_util.cli_call(["docker-compose"] + fileArgsToDockerCompose + [op, service], expect_success=True)
       assert context.compose_returncode == 0, "docker-compose failed to {0} {0}".format(op, service)
       if op == "stop" or op == "pause":
           context.compose_containers = [containerData for containerData in context.compose_containers if containerData.composeService != service]
       else:
           bdd_compose_util.parseComposeOutput(context)
       bdd_log("After {0}ing, the container service list is = {1}".format(op, [containerData.composeService for  containerData in context.compose_containers]))

    context.containerAliasMap = bdd_compose_util.mapAliasesToContainers(context)
    context.containerNameMap = bdd_compose_util.mapContainerNamesToContainers(context)

def to_bytes(strlist):
    return [base64.standard_b64encode(s.encode('ascii')) for s in strlist]

def prepend(elem, l):
    if l is None or l == "":
        tail = []
    else:
        tail = l
    if elem is None:
	return tail
    return [elem] + tail

@given(u'I do nothing')
def step_impl(context):
    pass
