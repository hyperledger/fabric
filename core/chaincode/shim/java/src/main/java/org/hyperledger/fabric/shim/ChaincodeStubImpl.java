/*
Copyright DTCC, IBM 2016, 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

         http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package org.hyperledger.fabric.shim;

import java.util.Collections;
import java.util.List;
import java.util.stream.Collectors;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.fabric.protos.peer.ChaincodeEventPackage.ChaincodeEvent;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;

import com.google.protobuf.ByteString;

class ChaincodeStubImpl implements ChaincodeStub {

	private static Log logger = LogFactory.getLog(ChaincodeStubImpl.class);

	private final String txid;
	private final Handler handler;
	private final List<ByteString> args;
	private ChaincodeEvent event;

	public ChaincodeStubImpl(String uuid, Handler handler, List<ByteString> args) {
		this.txid = uuid;
		this.handler = handler;
		this.args = Collections.unmodifiableList(args);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getArgs()
	 */
	@Override
	public List<byte[]> getArgs() {
		return args.stream().map(x -> x.toByteArray()).collect(Collectors.toList());
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getArgsAsStrings()
	 */
	@Override
	public List<String> getArgsAsStrings() {
		return args.stream().map(x -> x.toStringUtf8()).collect(Collectors.toList());
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#setEvent(java.lang.String, byte[])
	 */
	@Override
	public void setEvent(String name, byte[] payload) {
		if(name == null || name.trim().length() == 0) throw new IllegalArgumentException("Event name cannot be null or empty string.");
		if(payload != null) {
			this.event = ChaincodeEvent.newBuilder()
					.setEventName(name)
					.setPayload(ByteString.copyFrom(payload))
					.build();
		} else {
			this.event = ChaincodeEvent.newBuilder()
					.setEventName(name)
					.build();
		}
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getEvent()
	 */
	@Override
	public ChaincodeEvent getEvent() {
		return event;
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getTxId()
	 */
	@Override
	public String getTxId() {
		return txid;
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getState(java.lang.String)
	 */
	@Override
	public String getState(String key) {
		return handler.handleGetState(key, txid).toStringUtf8();
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#putState(java.lang.String, java.lang.String)
	 */
	@Override
	public void putState(String key, String value) {
		handler.handlePutState(key, ByteString.copyFromUtf8(value), txid);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#delState(java.lang.String)
	 */
	@Override
	public void delState(String key) {
		handler.handleDeleteState(key, txid);
	}
	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#createCompositeKey(java.lang.String, java.lang.String[])
	 */
	@Override
	public String createCompositeKey(String objectType, String[] attributes) {
		String compositeKey = new String();
		compositeKey = compositeKey + objectType;
		for (String attribute : attributes) {
			compositeKey = compositeKey + attribute.length() + attribute;
		}
		return compositeKey;
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#invokeChaincode(java.lang.String, java.util.List, java.lang.String)
	 */
	@Override
	public Response invokeChaincode(final String chaincodeName, final List<byte[]> args, final String channel) {
		// internally we handle chaincode name as a composite name
		final String compositeName;
		if(channel != null && channel.trim().length() > 0) {
			compositeName = chaincodeName + "/" + channel;
		} else {
			compositeName = chaincodeName;
		}
		return handler.handleInvokeChaincode(compositeName, args, this.txid);
	}

	// ------RAW CALLS------

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getRawState(java.lang.String)
	 */
	@Override
	public ByteString getRawState(String key) {
		return handler.handleGetState(key, txid);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#putRawState(java.lang.String, com.google.protobuf.ByteString)
	 */
	@Override
	public void putRawState(String key, ByteString value) {
		handler.handlePutState(key, value, txid);
	}

}
