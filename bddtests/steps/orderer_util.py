
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

import time
import datetime
import Queue
from concurrent.futures import ThreadPoolExecutor
from orderer import ab_pb2, ab_pb2_grpc
from common import common_pb2

import bdd_test_util
import bootstrap_util
import bdd_grpc_util


from grpc.beta import implementations
from grpc.framework.interfaces.face.face import AbortionError
from grpc.beta.interfaces import StatusCode

# The default chain ID when the system is statically bootstrapped for testing
TEST_CHAIN_ID = "testchainid"

def _defaultDataFunction(index):
    payload = common_pb2.Payload(
        header = common_pb2.Header(
            chainHeader = common_pb2.ChainHeader(
                chainID = TEST_CHAIN_ID,
                type = common_pb2.ENDORSER_TRANSACTION,
            ),
            signatureHeader = common_pb2.SignatureHeader(),
        ),
        data = str("BDD test: {0}".format(datetime.datetime.utcnow())),
    )
    envelope = common_pb2.Envelope(
        payload = payload.SerializeToString()
    )
    return envelope


class StreamHelper:

    def __init__(self):
        self.streamClosed = False
        self.sendQueue = Queue.Queue()
        self.receiveQueue = Queue.Queue()
        self.receivedMessages = []
        self.replyGenerator = None

    def setReplyGenerator(self, replyGenerator):
        assert self.replyGenerator == None, "reply generator already set!!"
        self.replyGenerator = replyGenerator

    def createSendGenerator(self, timeout = 2):
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

    def readMessage(self):
        for reply in self.readMessages(1):
            return reply
        assert False, "Received no messages"

    def readMessages(self, expectedCount):
        msgsReceived = []
        counter = 0
        try:
            for reply in self.replyGenerator:
                counter += 1
                #print("received reply: {0}, counter = {1}".format(reply, counter))
                msgsReceived.append(reply)
                if counter == int(expectedCount):
                    break
        except AbortionError as networkError:
            self.handleNetworkError(networkError)
        return msgsReceived

    def _start_receive(self):
        counter = 0
        try:
            for reply in self.replyGenerator:
                counter += 1
                #print("received reply: {0}, counter = {1}".format(reply, counter))
                self.receiveQueue.put(reply)
        except AbortionError as networkError:
            self.handleNetworkError(networkError)
        finally:
            self.streamClosed = True
        return

    def handleNetworkError(self, networkError):
        if networkError.code == StatusCode.OUT_OF_RANGE and networkError.details == "EOF":
            print("Error received and ignored: {0}".format(networkError))
            print()
            self.streamClosed = True
        else:
            raise Exception("Unexpected NetworkError: {0}".format(networkError))

    def send(self, msg):
        if msg:
            if self.streamClosed:
                raise Exception("Stream is closed.")
        self.sendQueue.put(msg)


class DeliverStreamHelper(StreamHelper):

    def __init__(self, ordererStub, entity, directory, nodeAdminTuple, timeout = 600):
        pool = ThreadPoolExecutor(1)
        StreamHelper.__init__(self)
        self.nodeAdminTuple = nodeAdminTuple
        self.directory = directory
        self.entity = entity
        # Set the UpdateMessage and start the stream
        sendGenerator = self.createSendGenerator(timeout)
        self.replyGenerator = ordererStub.Deliver(sendGenerator, timeout + 1)
        pool.submit(self._start_receive)

    def createSeekInfo(self, chainID, start = 'Oldest', end = 'Newest',  behavior = 'FAIL_IF_NOT_READY'):
        seekInfo = ab_pb2.SeekInfo(
            start = seekPosition(start),
            stop = seekPosition(end),
            behavior = ab_pb2.SeekInfo.SeekBehavior.Value(behavior),
        )
        return seekInfo

    def seekToRange(self, chainID = TEST_CHAIN_ID, start = 'Oldest', end = 'Newest'):
        seekInfo = self.createSeekInfo(start = start, end = end, chainID = chainID)
        envelope = bootstrap_util.createEnvelopeForMsg(directory=self.directory, chainId=chainID, msg=seekInfo, typeAsString="DELIVER_SEEK_INFO", nodeAdminTuple=self.nodeAdminTuple)
        self.send(envelope)

    def getBlocks(self, timeout=.5, max_wait_seconds=.5):
        blocks = []
        error_reply = None
        max_wait_time = datetime.datetime.now() + datetime.timedelta(seconds=max_wait_seconds)
        try:
            while datetime.datetime.now() < max_wait_time:
                try:
                    reply = self.receiveQueue.get(True, timeout)
                    if reply.HasField("block"):
                        blocks.append(reply.block)
                        #print("received reply: {0}, len(blocks) = {1}".format(reply, len(blocks)))
                    else:
                        if reply.status != common_pb2.SUCCESS:
                            print("Got error: {0}".format(reply.status))
                            error_reply = reply
                            break
                except Queue.Empty:
                    pass
        except Exception as e:
            print("getBlocks got error: {0}".format(e) )
        return blocks


class UserRegistration:

    def __init__(self, userName, directory):
        self.userName= userName
        self.directory = directory
        self.tags = {}
        # Dictionary of composeService->atomic broadcast grpc Stub
        self.atomicBroadcastStubsDict = {}
        # composeService->StreamHelper
        self.abDeliversStreamHelperDict = {}

    def getUserName(self):
        return self.userName

    def closeStreams(self):
        for compose_service, deliverStreamHelper in self.abDeliversStreamHelperDict.iteritems():
            deliverStreamHelper.send(None)
        self.abDeliversStreamHelperDict.clear()

    def connectToDeliverFunction(self, context, composeService, nodeAdminTuple, timeout=1):
        'Connect to the deliver function and drain messages to associated orderer queue'
        assert not composeService in self.abDeliversStreamHelperDict, "Already connected to deliver stream on {0}".format(composeService)
        streamHelper = DeliverStreamHelper(directory=self.directory,
                                           ordererStub=self.getABStubForComposeService(context=context,
                                                                                       composeService=composeService),
                                           entity=self, nodeAdminTuple=nodeAdminTuple)
        self.abDeliversStreamHelperDict[composeService] = streamHelper
        return streamHelper

    def getDelivererStreamHelper(self, context, composeService):
        assert composeService in self.abDeliversStreamHelperDict, "NOT connected to deliver stream on {0}".format(composeService)
        return self.abDeliversStreamHelperDict[composeService]

    def broadcastMessages(self, context, numMsgsToBroadcast, composeService, dataFunc=_defaultDataFunction):
        abStub = self.getABStubForComposeService(context, composeService)
        replyGenerator = abStub.Broadcast(generateBroadcastMessages(numToGenerate = int(numMsgsToBroadcast), dataFunc=dataFunc), 2)
        counter = 0
        try:
            for reply in replyGenerator:
                counter += 1
                print("{0} received reply: {1}, counter = {2}".format(self.getUserName(), reply, counter))
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
        root_certificates = self.directory.getTrustedRootsForOrdererNetworkAsPEM()
        ipAddress, port = bdd_test_util.getPortHostMapping(context.compose_containers, composeService, 7050)
        # print("ipAddress in getABStubForComposeService == {0}:{1}".format(ipAddress, port))
        channel = bdd_grpc_util.getGRPCChannel(ipAddress=ipAddress, port=port, root_certificates=root_certificates, ssl_target_name_override=composeService)
        newABStub = ab_pb2_grpc.AtomicBroadcastStub(channel)
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
    userRegistration = UserRegistration(secretMsg)
    context.ordererUsers[userName] = userRegistration
    return userRegistration

def getUserRegistration(context, enrollId):
    userRegistration = None
    if 'ordererUsers' in context:
        pass
    else:
        context.ordererUsers = {}
    if enrollId in context.ordererUsers:
        userRegistration = context.ordererUsers[enrollId]
    else:
        raise Exception("Orderer user has not been registered: {0}".format(enrollId))
    return userRegistration

def seekPosition(position):
    if position == 'Oldest':
        return ab_pb2.SeekPosition(oldest = ab_pb2.SeekOldest())
    elif  position == 'Newest':
        return ab_pb2.SeekPosition(newest = ab_pb2.SeekNewest())
    else:
        return ab_pb2.SeekPosition(specified = ab_pb2.SeekSpecified(number = position))

def convertSeek(utfString):
    try:
        return int(utfString)
    except ValueError:
        return str(utfString)

def createSeekInfo(chainID = TEST_CHAIN_ID, start = 'Oldest', end = 'Newest',  behavior = 'FAIL_IF_NOT_READY'):
    return common_pb2.Envelope(
        payload = common_pb2.Payload(
            header = common_pb2.Header(
                channel_header = common_pb2.ChannelHeader( channel_id = chainID ).SerializeToString(),
                signature_header = common_pb2.SignatureHeader().SerializeToString(),
            ),
            data = ab_pb2.SeekInfo(
                start = seekPosition(start),
                stop = seekPosition(end),
                behavior = ab_pb2.SeekInfo.SeekBehavior.Value(behavior),
            ).SerializeToString(),
        ).SerializeToString(),
    )


def generateBroadcastMessages(numToGenerate = 3, timeToHoldOpen = 1, dataFunc =_defaultDataFunction):
    messages = []
    for i in range(0, numToGenerate):
        messages.append(dataFunc(i))
    for msg in messages:
        yield msg
    time.sleep(timeToHoldOpen)
