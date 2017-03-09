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

import static org.hyperledger.fabric.protos.common.Common.Status.BAD_REQUEST;
import static org.hyperledger.fabric.protos.common.Common.Status.FORBIDDEN;
import static org.hyperledger.fabric.protos.common.Common.Status.INTERNAL_SERVER_ERROR;
import static org.hyperledger.fabric.protos.common.Common.Status.NOT_FOUND;
import static org.hyperledger.fabric.protos.common.Common.Status.REQUEST_ENTITY_TOO_LARGE;
import static org.hyperledger.fabric.protos.common.Common.Status.SERVICE_UNAVAILABLE;
import static org.hyperledger.fabric.protos.common.Common.Status.SUCCESS;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.nio.charset.StandardCharsets;

import org.hyperledger.fabric.protos.common.Common.Status;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response.Builder;

import com.google.protobuf.ByteString;

public abstract class ChaincodeHelper {
    public static final int UNKNOWN_VALUE = 0;
    
    public static Response newResponse(Status status, String message, byte[] payload) {
        final Builder builder = Response.newBuilder();
        builder.setStatus(status.getNumber());
        if(message != null) {
        	builder.setMessage(message);
        }
        if(payload != null) {
            builder.setPayload(ByteString.copyFrom(payload));
        }
        return builder.build();
    }
    
    public static Response newSuccessResponse(String message, byte[] payload) {
        return newResponse(SUCCESS, message, payload);
    }
    
    public static Response newSuccessResponse() {
    	return newSuccessResponse(null, null);
    }
    
    public static Response newSuccessResponse(String message) {
        return newSuccessResponse(message, null);
    }

    public static Response newSuccessResponse(byte[] payload) {
        return newSuccessResponse(null, payload);
    }

    public static Response newBadRequestResponse(String message, byte[] payload) {
        return newResponse(BAD_REQUEST, message, payload);
    }
    
    public static Response newBadRequestResponse() {
    	return newBadRequestResponse(null, null);
    }
    
    public static Response newBadRequestResponse(String message) {
        return newBadRequestResponse(message, null);
    }

    public static Response newBadRequestResponse(byte[] payload) {
        return newBadRequestResponse(null, payload);
    }

    public static Response newForbiddenResponse(byte[] payload) {
        return newForbiddenResponse(null, payload);
    }

    public static Response newForbiddenResponse(String message, byte[] payload) {
        return newResponse(FORBIDDEN, message, payload);
    }
    
    public static Response newForbiddenResponse() {
    	return newForbiddenResponse(null, null);
    }
    
    public static Response newForbiddenResponse(String message) {
        return newForbiddenResponse(message, null);
    }

    public static Response newNotFoundResponse(String message, byte[] payload) {
        return newResponse(NOT_FOUND, message, payload);
    }
    
    public static Response newNotFoundResponse() {
    	return newNotFoundResponse(null, null);
    }
    
    public static Response newNotFoundResponse(String message) {
        return newNotFoundResponse(message, null);
    }

    public static Response newNotFoundResponse(byte[] payload) {
        return newNotFoundResponse(null, payload);
    }
     
    public static Response newRequestEntityTooLargeResponse(String message, byte[] payload) {
        return newResponse(REQUEST_ENTITY_TOO_LARGE, message, payload);
    }
    
    public static Response newRequestEntityTooLargeResponse() {
    	return newRequestEntityTooLargeResponse(null, null);
    }
    
    public static Response newRequestEntityTooLargeResponse(String message) {
        return newRequestEntityTooLargeResponse(message, null);
    }

    public static Response newRequestEntityTooLargeResponse(byte[] payload) {
        return newRequestEntityTooLargeResponse(null, payload);
    }
    
    public static Response newInternalServerErrorResponse(String message, byte[] payload) {
        return newResponse(INTERNAL_SERVER_ERROR, message, payload);
    }
    
    public static Response newInternalServerErrorResponse() {
    	return newInternalServerErrorResponse(null, null);
    }
    
    public static Response newInternalServerErrorResponse(String message) {
        return newInternalServerErrorResponse(message, null);
    }

    public static Response newInternalServerErrorResponse(byte[] payload) {
        return newInternalServerErrorResponse(null, payload);
    }
     
    public static Response newInternalServerErrorResponse(Throwable throwable) {
        return newInternalServerErrorResponse(throwable.getMessage(), printStackTrace(throwable));
    }
     
    public static Response newServiceUnavailableErrorResponse(String message, byte[] payload) {
        return newResponse(SERVICE_UNAVAILABLE, message, payload);
    }
    
    public static Response newServiceUnavailableErrorResponse() {
    	return newServiceUnavailableErrorResponse(null, null);
    }
    
    public static Response newServiceUnavailableErrorResponse(String message) {
        return newServiceUnavailableErrorResponse(message, null);
    }

    public static Response newServiceUnavailableErrorResponse(byte[] payload) {
        return newServiceUnavailableErrorResponse(null, payload);
    }

    static byte[] printStackTrace(Throwable throwable) {
        if(throwable == null) return null;
        final StringWriter buffer = new StringWriter();
        throwable.printStackTrace(new PrintWriter(buffer));
        return buffer.toString().getBytes(StandardCharsets.UTF_8);
    }
    
}
