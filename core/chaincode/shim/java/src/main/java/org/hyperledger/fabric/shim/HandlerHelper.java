/*
 *  Copyright 2017 IBM - All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *    http://www.apache.org/licenses/LICENSE-2.0
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
package org.hyperledger.fabric.shim;

import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.COMPLETED;
import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.ERROR;
import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.GET_STATE;
import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.INVOKE_CHAINCODE;

import java.io.PrintWriter;
import java.io.StringWriter;

import org.hyperledger.fabric.protos.peer.ChaincodeEventPackage.ChaincodeEvent;
import org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage;
import org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;

import com.google.protobuf.ByteString;

abstract class HandlerHelper {

	static ChaincodeMessage newGetStateEventMessage(final String txid, final String key) {
		return newEventMessage(GET_STATE, txid, ByteString.copyFromUtf8(key));
	}

	static ChaincodeMessage newErrorEventMessage(final String txid, final Throwable throwable) {
		return newErrorEventMessage(txid, printStackTrace(throwable));
	}

	static ChaincodeMessage newErrorEventMessage(final String txid, final String message) {
		return newErrorEventMessage(txid, message, null);
	}

	static ChaincodeMessage newErrorEventMessage(final String txid, final String message, final ChaincodeEvent event) {
		return newEventMessage(ERROR, txid, ByteString.copyFromUtf8(message), event);
	}

	static ChaincodeMessage newCompletedEventMessage(final String txid, final Response response, final ChaincodeEvent event) {
		return newEventMessage(COMPLETED, txid, response.toByteString(), event);
	}
	
	static ChaincodeMessage newInvokeChaincodeMessage(final String txid, final ByteString payload) {
		return newEventMessage(INVOKE_CHAINCODE, txid, payload, null);
	}

	private static ChaincodeMessage newEventMessage(final Type type, final String txid, final ByteString payload) {
		return newEventMessage(type, txid, payload, null);
	}

	private static ChaincodeMessage newEventMessage(final Type type, final String txid, final ByteString payload, final ChaincodeEvent event) {
		if (event == null) {
			return ChaincodeMessage.newBuilder()
					.setType(type)
					.setTxid(txid)
					.setPayload(payload)
					.build();
		} else {
			return ChaincodeMessage.newBuilder()
					.setType(type)
					.setTxid(txid)
					.setPayload(payload)
					.setChaincodeEvent(event)
					.build();
		}
	}

	private static String printStackTrace(Throwable throwable) {
		if (throwable == null) return null;
		final StringWriter buffer = new StringWriter();
		throwable.printStackTrace(new PrintWriter(buffer));
		return buffer.toString();
	}

}
