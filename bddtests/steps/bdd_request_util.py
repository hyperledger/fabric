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
    return httpGet(request_url, expectSuccess)

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
    return httpPost(request_url, body, expectSuccess)

def buildContainerAliasUrl(context, containerAlias, endpoint, port=CORE_REST_PORT):
    """ Build a URL to do a HTTP request to the given container looking up the
        alias first. Optionally provide a port too which defaults to the peer
        rest port """
    container = context.containerAliasMap[containerAlias]
    return buildContainerUrl(context, container, endpoint, port=port)

def buildContainerUrl(context, container, endpoint, port=CORE_REST_PORT):
    """ Build a URL to do a HTTP request to the given container. Optionally
        provide a port too which defaults to the peer rest port """
    return buildUrl(context, container.ipAddress, port, endpoint)

def buildUrl(context, ipAddress, port, endpoint):
    schema = "http"
    if 'TLS' in context.tags:
        schema = "https"
    return "{0}://{1}:{2}{3}".format(schema, ipAddress, port, endpoint)

def httpGet(url, expectSuccess=True):
    return _request("GET", url, expectSuccess=expectSuccess)

def httpPost(url, body, expectSuccess=True):
    return _request("POST", url, json=body, expectSuccess=expectSuccess)

def _request(method, url, expectSuccess=True, **kwargs):
    bdd_log("HTTP {} to url = {}".format(method, url))

    response = requests.request(method, url, \
        headers=ACCEPT_JSON_HEADER, verify=False, **kwargs)

    if expectSuccess:
        assert response.status_code == 200, \
            "Failed to {} to {}: {}".format(method, url, response.text)

    bdd_log("Response from {}:".format(url))
    bdd_log(formatResponseText(response))

    return response

def formatResponseText(response):
    # Limit to 300 chars because of the size of the Base64 encoded Certs
    return json.dumps(response.json(), indent = 4)[:300]
