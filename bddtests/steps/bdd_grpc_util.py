
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

import grpc

def getGRPCChannel(ipAddress, port, root_certificates, ssl_target_name_override):
    # channel = grpc.insecure_channel("{0}:{1}".format(ipAddress, 7051), options = [('grpc.max_message_length', 100*1024*1024)])
    # creds = grpc.ssl_channel_credentials(root_certificates=root_certificates, private_key=private_key, certificate_chain=certificate_chain)
    creds = grpc.ssl_channel_credentials(root_certificates=root_certificates)
    channel = grpc.secure_channel("{0}:{1}".format(ipAddress, port), creds,
                                  options=(('grpc.ssl_target_name_override', ssl_target_name_override,),('grpc.default_authority', ssl_target_name_override,),('grpc.max_receive_message_length', 100*1024*1024)))

    # print("Returning GRPC for address: {0}".format(ipAddress))
    return channel

def toStringArray(items):
    itemsAsStr = []
    for item in items:
        if type(item) == str:
            itemsAsStr.append(item)
        elif type(item) == unicode:
            itemsAsStr.append(str(item))
        else:
            raise Exception("Error tring to convert to string: unexpected type '{0}'".format(type(item)))
    return itemsAsStr


