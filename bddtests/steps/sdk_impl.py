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
from bdd_request_util import buildContainerAliasUrl, httpGet

SDK_NODE_APP_REST_PORT = 8080

@given(u'I register thru the sample SDK app supplying username "{enrollId}" and secret "{enrollSecret}" on "{composeService}"')
def step_impl(context, enrollId, enrollSecret, composeService):
    assert 'compose_containers' in context, "compose_containers not found in context"

    secretMsg = {
        "enrollId": enrollId,
        "enrollSecret" : enrollSecret
    }

    url = buildContainerAliasUrl(context, composeService, "/", port=SDK_NODE_APP_REST_PORT)
    context.response = httpGet(url)
