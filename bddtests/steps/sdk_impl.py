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

import requests
import bdd_test_util
from bdd_test_util import bdd_log

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
    sampleAppIpAddress = context.containerAliasMap[composeService].ipAddress
    secretMsg = {
        "enrollId": enrollId,
        "enrollSecret" : enrollSecret
    }
    request_url = buildUrl(context, sampleAppIpAddress, "/")
    resp = requests.get(request_url, headers={'Accept': 'application/json'}, verify=False)
    assert resp.status_code == 200, "Failed to GET url %s:  %s" % (request_url,resp.text)
    context.response = resp
    bdd_log("")

