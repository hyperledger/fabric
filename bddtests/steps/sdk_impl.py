import os
import re
import time
import copy
import base64
from datetime import datetime, timedelta

import sys, requests, json

import bdd_test_util

from grpc.beta import implementations

import fabric_pb2
import chaincode_pb2
import devops_pb2

SDK_NODE_APP_REST_PORT = 8080

def buildUrl(context, ipAddress, path):
    schema = "http"
    if 'TLS' in context.tags:
        schema = "https"
    return "{0}://{1}:{2}{3}".format(schema, ipAddress, SDK_NODE_APP_REST_PORT, path)


@given(u'I register thru the sample SDK app supplying username "{enrollId}" and secret "{enrollSecret}" on "{composeService}"')
def step_impl(context, enrollId, enrollSecret, composeService):
    assert 'compose_containers' in context, "compose_containers not found in context"

    # Get the sampleApp IP Address
    containerDataList = bdd_test_util.getContainerDataValuesFromContext(context, [composeService], lambda containerData: containerData)
    sampleAppIpAddress = containerDataList[0].ipAddress
    secretMsg = {
        "enrollId": enrollId,
        "enrollSecret" : enrollSecret
    }
    request_url = buildUrl(context, sampleAppIpAddress, "/")
    resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
    assert resp.status_code == 200, "Failed to GET url %s:  %s" % (request_url,resp.text)
    context.response = resp
    print("")

