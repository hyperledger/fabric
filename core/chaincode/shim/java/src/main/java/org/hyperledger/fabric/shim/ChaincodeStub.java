/*
Copyright IBM 2017 All Rights Reserved.

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

import static java.nio.charset.StandardCharsets.UTF_8;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.List;
import java.util.stream.Collectors;

import org.hyperledger.fabric.protos.peer.ChaincodeEventPackage.ChaincodeEvent;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;

public interface ChaincodeStub {

	/**
	 * Returns the arguments corresponding to the call to
	 * {@link Chaincode#init(ChaincodeStub)} or
	 * {@link Chaincode#invoke(ChaincodeStub)}.
	 * 
	 * @return a list of arguments
	 */
	List<byte[]> getArgs();

	/**
	 * Returns the arguments corresponding to the call to
	 * {@link Chaincode#init(ChaincodeStub)} or
	 * {@link Chaincode#invoke(ChaincodeStub)}.
	 * 
	 * @return a list of arguments cast to a UTF-8 string
	 */
	List<String> getArgsAsStrings();

	/**
	 * Returns the transaction id
	 *
	 * @return the transaction id
	 */
	String getTxId();

	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @param channel If not specified, the caller's channel is assumed.  
	 * @return
	 */
	Response invokeChaincode(String chaincodeName, List<byte[]> args, String channel);

	/**
	 * Returns the byte array value specified by the key, from the ledger.
	 *
	 * @param key
	 *            name of the value
	 * @return value 
	 *            the value read from the ledger
	 */
	byte[] getState(String key);

	/**
	 * Writes the specified value and key into the ledger
	 *
	 * @param key
	 *            name of the value
	 * @param value
	 *            the value to write to the ledger
	 */
	void putState(String key, byte[] value);

	/**
	 * Removes the specified key from the ledger
	 *
	 * @param key
	 *            name of the value to be deleted
	 */
	void delState(String key);

	/**
	 * Given a set of attributes, this method combines these attributes to
	 * return a composite key.
	 *
	 * @param objectType
	 * @param attributes
	 * @return
	 */
	String createCompositeKey(String objectType, String[] attributes);

	/**
	 * Defines the CHAINCODE type event that will be posted to interested
	 * clients when the chaincode's result is committed to the ledger.
	 * 
	 * @param name
	 *            Name of event. Cannot be null or empty string.
	 * @param payload
	 *            Optional event payload.
	 */
	void setEvent(String name, byte[] payload);

	/**
	 * Invoke another chaincode using the same transaction context.
	 * 
	 * @param chaincodeName Name of chaincode to be invoked.
	 * @param args Arguments to pass on to the called chaincode.
	 * @return
	 */
	default Response invokeChaincode(String chaincodeName, List<byte[]> args) {
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
	default Response invokeChaincodeWithStringArgs(String chaincodeName, List<String> args, String channel) {
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
	default Response invokeChaincodeWithStringArgs(String chaincodeName, List<String> args) {
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
	default Response invokeChaincodeWithStringArgs(final String chaincodeName, final String... args) {
		return invokeChaincodeWithStringArgs(chaincodeName, Arrays.asList(args), null);
	}

	/**
	 * Returns the byte array value specified by the key and decoded as a UTF-8
	 * encoded string, from the ledger.
	 *
	 * @param key
	 *            name of the value
	 * @return value 
	 *            the value read from the ledger
	 */
	default String getStringState(String key) {
		return new String(getState(key), UTF_8);
	}

	/**
	 * Writes the specified value and key into the ledger
	 *
	 * @param key
	 *            name of the value
	 * @param value
	 *            the value to write to the ledger
	 */
	default void putStringState(String key, String value) {
		putState(key, value.getBytes(UTF_8));
	}

	/**
	 * Returns the CHAINCODE type event that will be posted to interested
	 * clients when the chaincode's result is committed to the ledger.
	 * @return the chaincode event or null
	 */
	ChaincodeEvent getEvent();

}