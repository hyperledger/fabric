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

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.stream.Collectors;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.fabric.protos.peer.ChaincodeEventPackage.ChaincodeEvent;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;

import com.google.protobuf.ByteString;

public class ChaincodeStub {

	private static Log logger = LogFactory.getLog(ChaincodeStub.class);

	private final String uuid;
	private final Handler handler;
	private final List<ByteString> args;
	private ChaincodeEvent event;

	public ChaincodeStub(String uuid, Handler handler, List<ByteString> args) {
		this.uuid = uuid;
		this.handler = handler;
		this.args = Collections.unmodifiableList(args);
	}

	public List<byte[]> getArgs() {
		return args.stream().map(x -> x.toByteArray()).collect(Collectors.toList());
	}

	public List<String> getArgsAsStrings() {
		return args.stream().map(x -> x.toStringUtf8()).collect(Collectors.toList());
	}

	/**
	 * Defines the CHAINCODE type event that will be posted to interested
	 * clients when the chaincode's result is committed to the ledger.
	 * 
	 * @param name
	 *            Name of event. Cannot be null or empty string.
	 * @param payload
	 *            Optional event payload.
	 */
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

	ChaincodeEvent getEvent() {
		return event;
	}

	/**
	 * Gets the UUID of this stub
	 *
	 * @return the id used to identify this communication channel
	 */
	public String getUuid() {
		return uuid;
	}

	/**
	 * Get the state of the provided key from the ledger, and returns is as a
	 * string
	 *
	 * @param key
	 *            the key of the desired state
	 * @return the String value of the requested state
	 */
	public String getState(String key) {
		return handler.handleGetState(key, uuid).toStringUtf8();
	}

	/**
	 * Puts the given state into a ledger, automatically wrapping it in a
	 * ByteString
	 *
	 * @param key
	 *            reference key
	 * @param value
	 *            value to be put
	 */
	public void putState(String key, String value) {
		handler.handlePutState(key, ByteString.copyFromUtf8(value), uuid);
	}

	/**
	 * Deletes the state of the given key from the ledger
	 *
	 * @param key
	 *            key of the state to be deleted
	 */
	public void delState(String key) {
		handler.handleDeleteState(key, uuid);
	}
	/**
	 * Given a start key and end key, this method returns a map of items with
	 * value converted to UTF-8 string.
	 *
	 * @param startKey
	 * @param endKey
	 * @return
	 */
	// TODO: Uncomment and fix range query with new proto type
	/*
	public Map<String, String> getStateByRange(String startKey, String endKey) {
		Map<String, String> retMap = new HashMap<>();
		for (Map.Entry<String, ByteString> item : getStateByRangeRaw(startKey, endKey).entrySet()) {
			retMap.put(item.getKey(), item.getValue().toStringUtf8());
		}
		return retMap;
	}
	*/
	/**
	 * This method is same as getStateByRange, except it returns value in
	 * ByteString, useful in cases where serialized object can be retrieved.
	 *
	 * @param startKey
	 * @param endKey
	 * @return
	 */
	// TODO: Uncomment and fix range query with new proto type
	/*
	public Map<String, ByteString> getStateByRangeRaw(String startKey, String endKey) {
		Map<String, ByteString> map = new HashMap<>();
		for (ChaincodeShim.QueryStateKeyValue mapping : handler.handleGetStateByRange(startKey, endKey, uuid).getKeysAndValuesList()) {
			map.put(mapping.getKey(), mapping.getValue());
		}
		return map;
	}
	*/

	/**
	 * Given a partial composite key, this method returns a map of items (whose
	 * key's prefix matches the given partial composite key) with value
	 * converted to UTF-8 string and this methid should be used only for a
	 * partial composite key; For a full composite key, an iter with empty
	 * response would be returned.
	 * 
	 * @param startKey
	 * @param endKey
	 * @return
	 */

	// TODO: Uncomment and fix range query with new proto type
	/*
	public Map<String, String> getStateByPartialCompositeKey(String objectType, String[] attributes) {
		String partialCompositeKey = new String();
		partialCompositeKey = createCompositeKey(objectType, attributes);
		return getStateByRange(partialCompositeKey + "1", partialCompositeKey + ":");
	}
	*/

	/**
	 * Given a set of attributes, this method combines these attributes to
	 * return a composite key.
	 *
	 * @param objectType
	 * @param attributes
	 * @return
	 */
	public String createCompositeKey(String objectType, String[] attributes) {
		String compositeKey = new String();
		compositeKey = compositeKey + objectType;
		for (String attribute : attributes) {
			compositeKey = compositeKey + attribute.length() + attribute;
		}
		return compositeKey;
	}

	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @param channel If not specified, the caller's channel is assumed.  
	 * @return
	 */
	public Response invokeChaincode(final String chaincodeName, final List<byte[]> args, final String channel) {
		// internally we handle chaincode name as a composite name
		final String compositeName;
		if(channel != null && channel.trim().length() > 0) {
			compositeName = chaincodeName + "/" + channel;
		} else {
			compositeName = chaincodeName;
		}
		return handler.handleInvokeChaincode(compositeName, args, this.uuid);
	}

	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @return
	 */
	public Response invokeChaincode(final String chaincodeName, final List<byte[]> args) {
		return invokeChaincode(chaincodeName, args, null);
	}
	
	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * This is a convenience version of {@link #invokeChaincode(String, List, String)}.
	 * The string args will be encoded into as UTF-8 bytes. 
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @param channel If not specified, the caller's channel is assumed.  
	 * @return
	 */
	public Response invokeChaincodeWithStringArgs(final String chaincodeName, final List<String> args, final String channel) {
		return invokeChaincode(chaincodeName, args.stream().map(x->x.getBytes(StandardCharsets.UTF_8)).collect(Collectors.toList()), channel);
	}
	
	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * This is a convenience version of {@link #invokeChaincode(String, List)}.
	 * The string args will be encoded into as UTF-8 bytes. 
	 * 
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @return
	 */
	public Response invokeChaincodeWithStringArgs(final String chaincodeName, final List<String> args) {
		return invokeChaincodeWithStringArgs(chaincodeName, args, null);
	}

	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * This is a convenience version of {@link #invokeChaincode(String, List)}.
	 * The string args will be encoded into as UTF-8 bytes. 
	 * 
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @return
	 */
	public Response invokeChaincodeWithStringArgs(final String chaincodeName, final String... args) {
		return invokeChaincodeWithStringArgs(chaincodeName, Arrays.asList(args), null);
	}

	// ------RAW CALLS------

	/**
	 * @param key
	 * @return
	 */
	public ByteString getRawState(String key) {
		return handler.handleGetState(key, uuid);
	}

	/**
	 * @param key
	 * @param value
	 */
	public void putRawState(String key, ByteString value) {
		handler.handlePutState(key, value, uuid);
	}

	/**
	 *
	 * @param startKey
	 * @param endKey
	 * @param limit
	 * @return
	 */
//	public GetStateByRangeResponse getStateByRangeRaw(String startKey, String endKey, int limit) {
//		return handler.handleGetStateByRange(startKey, endKey, limit, uuid);
//	}

}
