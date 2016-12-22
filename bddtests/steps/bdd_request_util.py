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

import requests, json
from bdd_test_util import bdd_log
from bdd_json_util import getAttributeFromJSON

CORE_REST_PORT = "7050"
ACCEPT_JSON_HEADER = {
    'Accept': 'application/json'
}

def httpGetToContainerAlias(context, containerAlias, endpoint, \
        port=CORE_REST_PORT, expectSuccess=True):
    """ Performs a GET to the given container doing a lookup by alias first.
        Optional args are port (defaults to the default peer rest port) and the
        expectSuccess which validates the response returned a 200 """
    container = context.containerAliasMap[containerAlias]
    return httpGetToContainer(context, container, endpoint, port, expectSuccess)

def httpGetToContainer(context, container, endpoint, \
        port=CORE_REST_PORT, expectSuccess=True):
    """ Performs a GET to the given container. Optional args are port (defaults
        to the default peer rest port) and the expectSuccess which validates the
        response returned a 200 """
    request_url = buildContainerUrl(context, container, endpoint, port)

    headers = {'Accept': 'application/json', 'Content-type': 'application/json'}
    #if context.byon:
    #    headers = getTokenedHeaders(context)

    return httpGet(request_url, headers=headers, expectSuccess=expectSuccess)

def httpPostToContainerAlias(context, containerAlias, endpoint, body, \
        port=CORE_REST_PORT, expectSuccess=True):
    """ Performs a POST to the given container doing a lookup by alias first.
        Optional args are port (defaults to the default peer rest port) and the
        expectSuccess which validates the response returned a 200 """
    container = context.containerAliasMap[containerAlias]
    return httpPostToContainer(context, container, endpoint, body, port, expectSuccess)

def httpPostToContainer(context, container, endpoint, body, \
        port=CORE_REST_PORT, expectSuccess=True):
    """ Performs a GET to the given container. Optional args are port (defaults
        to the default peer rest port) and the expectSuccess which validates the
        response returned a 200 """
    request_url = buildContainerUrl(context, container, endpoint, port)
    bdd_log("Request URL: {}".format(request_url))

    headers = {'Accept': 'application/json', 'Content-type': 'application/json'}
    #if context.byon:
    #    headers = getTokenedHeaders(context)
    bdd_log("Headers: {}".format(headers))

    return httpPost(request_url, body, headers=headers, expectSuccess=expectSuccess)

def buildContainerAliasUrl(context, containerAlias, endpoint, port=CORE_REST_PORT):
    """ Build a URL to do a HTTP request to the given container looking up the
        alias first. Optionally provide a port too which defaults to the peer
        rest port """
    container = context.containerAliasMap[containerAlias]
    return buildContainerUrl(context, container, endpoint, port=port)

def buildContainerUrl(context, container, endpoint, port=CORE_REST_PORT):
    """ Build a URL to do a HTTP request to the given container. Optionally
        provide a port too which defaults to the peer rest port """
    bdd_log("container: {}".format(container))
    bdd_log("name: {}".format(container.name))
    bdd_log("ipAddress: {}".format(container.ipAddress))

    peer = container.name.split("_")[1]
    #if context.byon:
    #    bdd_log("remote map: {}".format(context.remote_map))
    #    port = context.remote_map[peer]['port']
    return buildUrl(context, container.ipAddress, port, endpoint)

def buildUrl(context, ipAddress, port, endpoint):
    schema = "http"
    #if 'TLS' in context.tags or context.tls:
    if hasattr(context.tags, 'TLS') or (hasattr(context, 'tls') and context.tls):
        schema = "https"
    return "{0}://{1}:{2}{3}".format(schema, ipAddress, port, endpoint)

def httpGet(url, headers=ACCEPT_JSON_HEADER, expectSuccess=True):
    return _request("GET", url, headers, expectSuccess=expectSuccess)

def httpPost(url, body, headers=ACCEPT_JSON_HEADER, expectSuccess=True):
    return _request("POST", url, headers=headers, expectSuccess=expectSuccess, json=body)

def _request(method, url, headers, expectSuccess=True, **kwargs):
    bdd_log("HTTP {} to url = {}".format(method, url))

    response = requests.request(method, url, \
        headers=headers, verify=False, **kwargs)

    if expectSuccess:
        assert response.status_code == 200, \
            "Failed to {} to {}: {}".format(method, url, response.text)

    bdd_log("Response from {}:".format(url))
    bdd_log(formatResponseText(response))

    response.connection.close()
    return response

def formatResponseText(response):
    # Limit to 300 chars because of the size of the Base64 encoded Certs
    bdd_log("Response: {}".format(response))
    try:
        return json.dumps(response.json(), indent = 4)[:300]
    except:
        return ""


def getSchema(tls):
    schema = "http"
    if tls:
        schema = "https"
    return schema


def getTokenedHeaders(context):
    getToken(context)
    headers = {'Accept': 'application/json', 'Content-type': 'application/json'}
    #if context.byon:
    #    headers = {'Accept': 'application/vnd.ibm.zaci.payload+json;version=1.0',
    #               'Content-type': 'application/vnd.ibm.zaci.payload+json;version=1.0',
    #               'Authorization': 'Bearer %s' % context.token,
    #               'zACI-API': 'com.ibm.zaci.system/1.0'}
    return headers


def getToken(context):
    #if 'token' in context:
    if hasattr(context, 'token'):
        return context.token

    headers = {'Accept': 'application/json', 'Content-type': 'application/json'}
    #if context.byon:
    #    headers = {'Accept': 'application/vnd.ibm.zaci.payload+json;version=1.0',
    #               'Content-type': 'application/vnd.ibm.zaci.payload+json;version=1.0',
    #               'zACI-API': 'com.ibm.zaci.system/1.0'}

    #url = "{0}://{1}/api/com.ibm.zaci.system/api-tokens".format(getSchema(context.tls), context.remote_ip)
    url = "https://{1}/api/com.ibm.zaci.system/api-tokens".format(getSchema(context.tls), context.remote_ip)
    body = {"kind": "request",
            "parameters": {"user": "anunez", "password": "password"}}
    response = httpPost(url, body, headers=headers)
    try:
        context.token = getAttributeFromJSON("parameters.token", response.json())
    except:
        bdd_log("Unable to get the token for the network at {}".format(context.remote_ip))
        raise Exception, "Unable to get the network token"
    return response

