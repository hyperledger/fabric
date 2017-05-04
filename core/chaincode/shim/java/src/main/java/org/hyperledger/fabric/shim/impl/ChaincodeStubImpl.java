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

package org.hyperledger.fabric.shim.impl;

import static java.util.stream.Collectors.toList;

import java.util.Collections;
import java.util.List;
import java.util.function.Function;
import java.util.stream.Collectors;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.fabric.protos.ledger.queryresult.KvQueryResult;
import org.hyperledger.fabric.protos.ledger.queryresult.KvQueryResult.KV;
import org.hyperledger.fabric.protos.peer.ChaincodeEventPackage.ChaincodeEvent;
import org.hyperledger.fabric.protos.peer.ChaincodeShim.QueryResultBytes;
import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;
import org.hyperledger.fabric.shim.ChaincodeStub;
import org.hyperledger.fabric.shim.ledger.CompositeKey;
import org.hyperledger.fabric.shim.ledger.KeyModification;
import org.hyperledger.fabric.shim.ledger.KeyValue;
import org.hyperledger.fabric.shim.ledger.QueryResultsIterator;

import com.google.protobuf.ByteString;
import com.google.protobuf.InvalidProtocolBufferException;

class ChaincodeStubImpl implements ChaincodeStub {

	private static Log logger = LogFactory.getLog(ChaincodeStubImpl.class);

	private final String txid;
	private final Handler handler;
	private final List<ByteString> args;
	private ChaincodeEvent event;

	ChaincodeStubImpl(String uuid, Handler handler, List<ByteString> args) {
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
	public List<String> getStringArgs() {
		return args.stream().map(x -> x.toStringUtf8()).collect(Collectors.toList());
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getFunction()
	 */
	@Override
	public String getFunction() {
		return getStringArgs().size() > 0 ? getStringArgs().get(0) : null;
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getParameters()
	 */
	@Override
	public List<String> getParameters() {
		return getStringArgs().stream().skip(1).collect(toList());
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
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getState(String)
	 */
	@Override
	public byte[] getState(String key) {
		return handler.handleGetState(key, txid).toByteArray();
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#putRawState(java.lang.String, com.google.protobuf.ByteString)
	 */
	@Override
	public void putState(String key, byte[] value) {
		handler.handlePutState(key, ByteString.copyFrom(value), txid);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#delState(java.lang.String)
	 */
	@Override
	public void delState(String key) {
		handler.handleDeleteState(key, txid);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getStateByRange(java.lang.String, java.lang.String)
	 */
	@Override
	public QueryResultsIterator<KeyValue> getStateByRange(String startKey, String endKey) {
		return new QueryResultsIteratorImpl<KeyValue>(this.handler, getTxId(),
				handler.handleGetStateByRange(getTxId(), startKey, endKey),
				queryResultBytesToKv.andThen(KeyValueImpl::new)
				);
	}

	private Function<QueryResultBytes, KV> queryResultBytesToKv = new Function<QueryResultBytes, KV>() {
		public KV apply(QueryResultBytes queryResultBytes) {
			try {
				return KV.parseFrom(queryResultBytes.getResultBytes());
			} catch (InvalidProtocolBufferException e) {
				throw new RuntimeException(e);
			}
		};
	};

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getStateByPartialCompositeKey(java.lang.String)
	 */
	@Override
	public QueryResultsIterator<KeyValue> getStateByPartialCompositeKey(String compositeKey) {
		return getStateByRange(compositeKey, compositeKey + "\udbff\udfff");
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#createCompositeKey(java.lang.String, java.lang.String[])
	 */
	@Override
	public CompositeKey createCompositeKey(String objectType, String... attributes) {
		return new CompositeKey(objectType, attributes);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#splitCompositeKey(java.lang.String)
	 */
	@Override
	public CompositeKey splitCompositeKey(String compositeKey) {
		return CompositeKey.parseCompositeKey(compositeKey);
	}

	/* (non-Javadoc)
	 * @see org.hyperledger.fabric.shim.ChaincodeStub#getQueryResult(java.lang.String)
	 */
	@Override
	public QueryResultsIterator<KeyValue> getQueryResult(String query) {
		return new QueryResultsIteratorImpl<KeyValue>(this.handler, getTxId(),
				handler.handleGetQueryResult(getTxId(), query),
				queryResultBytesToKv.andThen(KeyValueImpl::new)
				);
	}

	@Override
	public QueryResultsIterator<KeyModification> getHistoryForKey(String key) {
		return new QueryResultsIteratorImpl<KeyModification>(this.handler, getTxId(),
				handler.handleGetHistoryForKey(getTxId(), key),
				queryResultBytesToKeyModification.andThen(KeyModificationImpl::new)
				);
	}

	private Function<QueryResultBytes, KvQueryResult.KeyModification> queryResultBytesToKeyModification = new Function<QueryResultBytes, KvQueryResult.KeyModification>() {
		public KvQueryResult.KeyModification apply(QueryResultBytes queryResultBytes) {
			try {
				return KvQueryResult.KeyModification.parseFrom(queryResultBytes.getResultBytes());
			} catch (InvalidProtocolBufferException e) {
				throw new RuntimeException(e);
			}
		};
	};

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

}
