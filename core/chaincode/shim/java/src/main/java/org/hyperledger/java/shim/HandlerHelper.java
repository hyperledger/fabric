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
package org.hyperledger.java.shim;

import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.COMPLETED;
import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.ERROR;
import static org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type.GET_STATE;

import org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage;
import org.hyperledger.fabric.protos.peer.ChaincodeShim.ChaincodeMessage.Type;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;

import com.google.protobuf.ByteString;

abstract class HandlerHelper {

    private static ChaincodeMessage newEventMessage(final Type type, final String txid, final ByteString payload) {
        return ChaincodeMessage.newBuilder()
                .setType(type)
                .setTxid(txid)
                .setPayload(payload)
                .build();
    }
    
    static ChaincodeMessage newGetStateEventMessage(final String txid, final String key) {
        return newEventMessage(GET_STATE, txid, ByteString.copyFromUtf8(key));
    }
    
    static ChaincodeMessage newErrorEventMessage(final String txid, final Response payload) {
        return newEventMessage(ERROR, txid, payload.toByteString());
    }
    
    static ChaincodeMessage newErrorEventMessage(final String txid, final Throwable throwable) {
        return newErrorEventMessage(txid, ChaincodeHelper.newInternalServerErrorResponse(throwable));
    }

    static ChaincodeMessage newErrorEventMessage(final String txid, final String message) {
        return newErrorEventMessage(txid, ChaincodeHelper.newInternalServerErrorResponse(message));
    }

    static ChaincodeMessage newCompletedEventMessage(final String txid, Response response) {
        return ChaincodeMessage.newBuilder()
                .setType(COMPLETED)
                .setTxid(txid)
                .setPayload(response.toByteString())
                .build();
    }


}
