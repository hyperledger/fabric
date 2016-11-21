
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
from orderer import ab_pb2
from common import common_pb2

import bdd_test_util
import bdd_grpc_util

from grpc.beta import implementations
from grpc.framework.interfaces.face.face import NetworkError
from grpc.framework.interfaces.face.face import AbortionError
from grpc.beta.interfaces import StatusCode
from common.common_pb2 import Payload


class StreamHelper:

    def __init__(self, ordererStub):
        self.streamClosed = False
        self.ordererStub = ordererStub
        self.sendQueue = Queue.Queue()
        self.receivedMessages = []
        self.replyGenerator = None

    def setReplyGenerator(self, replyGenerator):
        assert self.replyGenerator == None, "reply generator already set!!"
        self.replyGenerator = replyGenerator

    def createSendGenerator(self, firstMsg, timeout = 2):
      yield firstMsg
      while True:
        try:
            nextMsg = self.sendQueue.get(True, timeout)
            if nextMsg:
              yield nextMsg
            else:
              #None indicates desire to close send
              return
        except Queue.Empty:
            return

    def readMessages(self, expectedCount):
        msgsReceived = []
        counter = 0
        try:
            for reply in self.replyGenerator:
                counter += 1
                print("{0} received reply: {1}, counter = {2}".format("DeliverStreamHelper", reply, counter))
                msgsReceived.append(reply)
                if counter == int(expectedCount):
                    break
        except AbortionError as networkError:
            self.handleNetworkError(networkError)
        return msgsReceived

    def handleNetworkError(self, networkError):
        if networkError.code == StatusCode.OUT_OF_RANGE and networkError.details == "EOF":
            print("Error received and ignored: {0}".format(networkError))
            print()
            self.streamClosed = True
        else:
            raise Exception("Unexpected NetworkError: {0}".format(networkError))


class DeliverStreamHelper(StreamHelper):

    def __init__(self, ordererStub, sendAck, Start, SpecifiedNumber, WindowSize, timeout = 1):
        StreamHelper.__init__(self, ordererStub)
        #Set the ack flag
        trueOptions = ['true', 'True','yes','Yes']
        falseOptions = ['false', 'False', 'no', 'No']
        assert sendAck in trueOptions + falseOptions, "sendAck of '{0}' not recognized, expected one of '{1}'".format(sendAck, trueOptions + falseOptions)
        self.sendAck = sendAck in trueOptions
        # Set the UpdateMessage and start the stream
        self.deliverUpdateMsg = createDeliverUpdateMsg(Start, SpecifiedNumber, WindowSize)
        sendGenerator = self.createSendGenerator(self.deliverUpdateMsg, timeout)
        replyGenerator = ordererStub.Deliver(sendGenerator, timeout + 1)
        self.replyGenerator = replyGenerator

    def seekToBlock(self, blockNum):
        deliverUpdateMsg = ab_pb2.DeliverUpdate()
        deliverUpdateMsg.CopyFrom(self.deliverUpdateMsg)
        deliverUpdateMsg.Seek.SpecifiedNumber = blockNum
        self.sendQueue.put(deliverUpdateMsg)

    def sendAcknowledgment(self, blockNum):
        deliverUpdateMsg = ab_pb2.DeliverUpdate(Acknowledgement = ab_pb2.Acknowledgement(Number = blockNum))
        self.sendQueue.put(deliverUpdateMsg)

    def getWindowSize(self):
        return self.deliverUpdateMsg.Seek.WindowSize

    def readDeliveredMessages(self, expectedCount):
        'Read the expected number of messages, being sure to supply the ACK if sendAck is True'
        if not self.sendAck:
            return self.readMessages(expectedCount)
        else:
            # This block assumes the expectedCount is a multiple of the windowSize
            msgsRead = []
            while len(msgsRead) < expectedCount and self.streamClosed == False:
                numToRead = self.getWindowSize() if self.getWindowSize() < expectedCount else expectedCount
                msgsRead.extend(self.readMessages(numToRead))
                # send the ack
                self.sendAcknowledgment(msgsRead[-1].Block.Header.Number)
                print('SentACK!!')
                print('')
            return msgsRead



class UserRegistration:


    def __init__(self, secretMsg, composeService):
        self.enrollId = secretMsg['enrollId']
        self.secretMsg = secretMsg
        self.composeService = composeService
        self.tags = {}
        # Dictionary of composeService->atomic broadcast grpc Stub
        self.atomicBroadcastStubsDict = {}
        # composeService->StreamHelper
        self.abDeliversStreamHelperDict = {}

    def getUserName(self):
        return self.secretMsg['enrollId']


    def connectToDeliverFunction(self, context, sendAck, start, SpecifiedNumber, WindowSize, composeService):
        'Connect to the deliver function and drain messages to associated orderer queue'
        assert not composeService in self.abDeliversStreamHelperDict, "Already connected to deliver stream on {0}".format(composeService)
        streamHelper = DeliverStreamHelper(self.getABStubForComposeService(context, composeService), sendAck, start, SpecifiedNumber, WindowSize)
        self.abDeliversStreamHelperDict[composeService] = streamHelper
        return streamHelper


    def getDelivererStreamHelper(self, context, composeService):
        assert composeService in self.abDeliversStreamHelperDict, "NOT connected to deliver stream on {0}".format(composeService)
        return self.abDeliversStreamHelperDict[composeService]


    def broadcastMessages(self, context, numMsgsToBroadcast, composeService):
		abStub = self.getABStubForComposeService(context, composeService)
		replyGenerator = abStub.Broadcast(generateBroadcastMessages(int(numMsgsToBroadcast)),2)
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
		newABStub = ab_pb2.beta_create_AtomicBroadcast_stub(channel)
		self.atomicBroadcastStubsDict[composeService] = newABStub
		return newABStub


# Registerses a user on a specific composeService
def registerUser(context, secretMsg, composeService):
    userName = secretMsg['enrollId']
    if 'ordererUsers' in context:
        pass
    else:
        context.ordererUsers = {}
    if userName in context.ordererUsers:
        raise Exception("Orderer user already registered: {0}".format(userName))
    userRegistration = UserRegistration(secretMsg, composeService)
    context.ordererUsers[userName] = userRegistration
    return userRegistration

def getUserRegistration(context, enrollId):
    userRegistration = None
    if 'ordererUsers' in context:
        pass
    else:
        ordererContext.ordererUsers = {}
    if enrollId in context.ordererUsers:
        userRegistration = context.ordererUsers[enrollId]
    else:
        raise Exception("Orderer user has not been registered: {0}".format(enrollId))
    return userRegistration

def createDeliverUpdateMsg(Start, SpecifiedNumber, WindowSize):
    seek = ab_pb2.SeekInfo()
    startVal = seek.__getattribute__(Start)
    seekInfo = ab_pb2.SeekInfo(Start = startVal, SpecifiedNumber = SpecifiedNumber, WindowSize = WindowSize)
    deliverUpdateMsg = ab_pb2.DeliverUpdate(Seek = seekInfo)
    return deliverUpdateMsg


def generateBroadcastMessages(numToGenerate = 1, timeToHoldOpen = 1):
    messages = []
    for i in range(0, numToGenerate):
        envelope = common_pb2.Envelope()
        payload = common_pb2.Payload(header = common_pb2.Header(chainHeader = common_pb2.ChainHeader()))
        # TODO, appropriately set the header type
        payload.data = str("BDD test: {0}".format(datetime.datetime.utcnow()))
        envelope.payload = payload.SerializeToString()
        messages.append(envelope)
    for msg in messages:
        yield msg
    time.sleep(timeToHoldOpen)


def getGRPCChannel(ipAddress):
    channel = implementations.insecure_channel(ipAddress, 7050)
    print("Returning GRPC for address: {0}".format(ipAddress))
    return channel
