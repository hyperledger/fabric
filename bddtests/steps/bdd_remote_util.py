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


from bdd_request_util import httpGet, httpPost, getTokenedHeaders, getToken, getSchema
from bdd_test_util import bdd_log


def getNetwork(context):
    """ Get the Network ID."""
    if hasattr(context, 'network_id'):
        return context.network_id

    headers = getTokenedHeaders(context)
    url = "{0}://{1}/api/com.ibm.zBlockchain/networks".format(getSchema(context.tls), context.remote_ip)
    response = httpGet(url, headers=headers)
    context.network_id = response.json()[0]
    return response


def stopNode(context, peer):
    """Stops the peer node on a specific network."""
    headers = getTokenedHeaders(context)
    getNetwork(context)

    url = "{0}://{1}/api/com.ibm.zBlockchain/networks/{2}/nodes/{3}/stop".format(getSchema(context.tls), context.remote_ip, context.network_id, peer)
    body = {}
    response = httpPost(url, body, headers=headers)


def restartNode(context, peer):
    """Restart the peer node on a specific network."""
    headers = getTokenedHeaders(context)
    getNetwork(context)
    url = "{0}://{1}/api/com.ibm.zBlockchain/networks/{2}/nodes/{3}/restart".format(getSchema(context.tls), context.remote_ip, context.network_id, peer)
    body = {}
    response = httpPost(url, body, headers=headers)


def getNodeStatus(context, peer):
    """ Get the Node status."""
    headers = getTokenedHeaders(context)
    getNetwork(context)
    url = "{0}://{1}/api/com.ibm.zBlockchain/networks/{2}/nodes/{3}/status".format(getSchema(context.tls), context.remote_ip, context.network_id, peer)
    response = httpGet(url, headers=headers)
    return response


def getNodeLogs(context):
    """ Get the Node logs."""
    headers = getTokenedHeaders(context)
    getNetwork(context)
    url = "{0}://{1}/api/com.ibm.zBlockchain/networks/{2}/nodes/{3}/logs".format(getSchema(context.tls), context.remote_ip, context.network_id, peer)
    response = httpGet(url, headers=headers)
    return response


def getChaincodeLogs(context, peer):
    """ Get the Chaincode logs."""
    headers = getTokenedHeaders(context)
    getNetwork(context)
    # /api/com.ibm.zBlockchain/networks/{network_id}/nodes/{node_id}/chaincodes/{chaincode_id}/logs
    #url = "{0}://{1}/api/com.ibm.zBlockchain/networks/{2}/nodes/{3}/chaincodes/{4}/logs".format(getSchema(context.tls), context.remote_ip, context.network_id, peer, context.chaincodeSpec['chaincodeID']['name'])
    if hasattr(context, 'chaincodeSpec'):
        url = "{0}://{1}/api/com.ibm.zBlockchain/networks/{2}/nodes/{3}/chaincodes/{4}/logs".format(getSchema(context.tls), context.remote_ip, context.network_id, peer, context.chaincodeSpec.get('chaincodeID', {}).get('name', ''))
        response = httpGet(url, headers=headers)
    else:
        response = "No chaincode has been deployed"
    return response

