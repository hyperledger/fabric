
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
import re
import time
import datetime
import Queue
import subprocess
import devops_pb2
import fabric_pb2
import chaincode_pb2
import ab_pb2

import bdd_test_util
import bdd_grpc_util

from grpc.beta import implementations
from grpc.framework.interfaces.face.face import NetworkError
from grpc.beta.interfaces import StatusCode



class UserRegistration:
	
    
    def __init__(self, secretMsg, composeService):
        self.enrollId = secretMsg['enrollId']
        self.secretMsg = secretMsg
        self.composeService = composeService
        self.tags = {}
        self.lastResult = None
        self.abStub = None
        self.abBroadcastersDict = {}
        # composeService->Queue of messages received
        self.abDeliversQueueDict = {}
        # Dictionary of composeService->atomic broadcast grpc Stub
        self.atomicBroadcastStubsDict = {}

    def getUserName(self):
        return self.secretMsg['enrollId']

    def connectToDeliverFunction(self, context, composeService):
    	'Connect to the deliver function and drain messages to associated orderer queue'
    	replyGenerator = self.getABStubForComposeService(context, composeService).deliver(generateDeliverMessages(context, timeToHoldOpen = .25),2)
        delivererQueue = self.getDelivererQueue(context, composeService)
        counter = 0
        try:
			for reply in replyGenerator:
				counter += 1
				# Parse the msg bytes as a deliver_reply message
				print("{0} received reply: {1}, counter = {2}".format(self.enrollId, reply, counter))
				delivererQueue.append(reply)
        except NetworkError as networkError:
			if networkError.code == StatusCode.OUT_OF_RANGE and networkError.details == "EOF":
				print("Error received and ignored: {0}", networkError)
				print()
			else:
				raise Exception("Unexpected NetworkError: {0}", networkError)
    
    def getDelivererQueue(self, context, composeService):
    	if not composeService in self.abDeliversQueueDict:
    		#self.abDeliversQueueDict[composeService] = Queue.Queue()
    		self.abDeliversQueueDict[composeService] = []
    	return self.abDeliversQueueDict[composeService]
    		

    def broadcastMessages(self, context, numMsgsToBroadcast, composeService):
		abStub = self.getABStubForComposeService(context, composeService)
		replyGenerator = abStub.broadcast(generateBroadcastMessages(int(numMsgsToBroadcast)),2)
		counter = 0
		try:
			for reply in replyGenerator:
				counter += 1
				print("{0} received reply: {1}, counter = {2}".format(self.enrollId, reply, counter))
				if counter == int(numMsgsToBroadcast):
					break
		except Exception as e:
			print("Got error: {0}".format(e) )
			print("Got error")
		print("Done")
		assert counter == int(numMsgsToBroadcast), "counter = {0}, expected {1}".format(counter, numMsgsToBroadcast)
    
    def getABStubForComposeService(self, context, composeService):
		'Return a Stub for the supplied composeService, will cache'
		if composeService in self.atomicBroadcastStubsDict:
			return self.atomicBroadcastStubsDict[composeService]
		# Get the IP address of the server that the user registered on
		ipAddress = bdd_test_util.ipFromContainerNamePart(composeService, context.compose_containers)
		channel = getGRPCChannel(ipAddress)
		newABStub = ab_pb2.beta_create_atomic_broadcast_stub(channel)
		self.atomicBroadcastStubsDict[composeService] = newABStub
		return newABStub

# Registerses a user on a specific composeService
def registerUser(context, secretMsg, composeService):
    userName = secretMsg['enrollId']
    if 'users' in context:
        pass
    else:
        context.users = {}
    if userName in context.users:
        raise Exception("User already registered: {0}".format(userName))
    userRegistration = UserRegistration(secretMsg, composeService)
    context.users[userName] = userRegistration
    return userRegistration

def getUserRegistration(context, enrollId):
    userRegistration = None
    if 'users' in context:
        pass
    else:
        context.users = {}
    if enrollId in context.users:
        userRegistration = context.users[enrollId]
    else:
        raise Exception("User has not been registered: {0}".format(enrollId))
    return userRegistration

def generateDeliverMessages(context, timeToHoldOpen = 1):
	'Generator for Deliver message'
	assert 'table' in context, "table (Start | specified_number| window_size) not found in context"
	row = context.table.rows[0]
	start, specified_number, window_size = row['Start'], int(row['specified_number']), int(row['window_size'])
	# if start != str(ab_pb2.seek_info.SPECIFIED):
	# 	raise Exception("Currently only support Specified seek_info type")
	seekInfo = ab_pb2.seek_info(start = ab_pb2.seek_info.SPECIFIED, specified_number = specified_number, window_size = window_size)
	deliverUpdateMsg = ab_pb2.deliver_update(seek = seekInfo)
	yield deliverUpdateMsg
	print("sent msg {0}".format(deliverUpdateMsg))
	time.sleep(timeToHoldOpen)	

def generateBroadcastMessages(numToGenerate = 1, timeToHoldOpen = 1):
	messages = []
	for i in range(0, numToGenerate):
		messages.append(ab_pb2.broadcast_message(data = str("BDD test: {0}".format(datetime.datetime.utcnow()))))		
	for msg in messages:
		yield msg
	time.sleep(timeToHoldOpen)	


def getGRPCChannel(ipAddress):
    channel = implementations.insecure_channel(ipAddress, 5005)
    print("Returning GRPC for address: {0}".format(ipAddress))
    return channel
