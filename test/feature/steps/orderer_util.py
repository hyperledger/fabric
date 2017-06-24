# Copyright IBM Corp. 2017 All Rights Reserved.
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
import sys
import datetime

try:
    pbFilePath = "../../bddtests"
    sys.path.insert(0, pbFilePath)
    from common import common_pb2
except:
    print("ERROR! Unable to import the protobuf libraries from the hyperledger/fabric/bddtests directory: {0}".format(sys.exc_info()[0]))
    sys.exit(1)


# The default chain ID when the system is statically bootstrapped for testing
TEST_CHAIN_ID = "testchainid"


def _testAccessPBMethods():
    channel_header = common_pb2.ChannelHeader(channel_id=TEST_CHAIN_ID,
                                              type=common_pb2.ENDORSER_TRANSACTION)
    header = common_pb2.Header(channel_header=channel_header.SerializeToString(),
                               signature_header=common_pb2.SignatureHeader().SerializeToString())
    payload = common_pb2.Payload(header=header,
                                 data=str.encode("Functional test: {0}".format(datetime.datetime.utcnow())) )
    envelope = common_pb2.Envelope(payload=payload.SerializeToString())
    return envelope


envelope = _testAccessPBMethods()
assert isinstance(envelope, common_pb2.Envelope)
print("Successfully imported protobufs from bddtests directory")
