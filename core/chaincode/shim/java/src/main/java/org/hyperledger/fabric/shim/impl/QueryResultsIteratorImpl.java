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
package org.hyperledger.fabric.shim.impl;

import java.util.Collections;
import java.util.Iterator;
import java.util.NoSuchElementException;
import java.util.function.Function;

import org.hyperledger.fabric.protos.peer.ChaincodeShim.QueryResponse;
import org.hyperledger.fabric.protos.peer.ChaincodeShim.QueryResultBytes;
import org.hyperledger.fabric.shim.ledger.QueryResultsIterator;

class QueryResultsIteratorImpl<T> implements QueryResultsIterator<T> {
		
	private final Handler handler;
	private final String txId;
	private Iterator<QueryResultBytes> currentIterator;
	private QueryResponse currentQueryResponse;
	private Function<QueryResultBytes, T> mapper;
	
	public QueryResultsIteratorImpl(final Handler handler, final String txId, final QueryResponse queryResponse, Function<QueryResultBytes, T> mapper) {
		this.handler = handler;
		this.txId = txId;
		this.currentQueryResponse = queryResponse;
		this.currentIterator = currentQueryResponse.getResultsList().iterator();
		this.mapper = mapper;
	}

	@Override
	public Iterator<T> iterator() {
		return new Iterator<T>() {

			@Override
			public boolean hasNext() {
				return currentIterator.hasNext() || currentQueryResponse.getHasMore();
			}

			@Override
			public T next() {
				
				// return next fetched result, if any
				if(currentIterator.hasNext()) return mapper.apply(currentIterator.next());
				
				// throw exception if there are no more expected results
				if(!currentQueryResponse.getHasMore()) throw new NoSuchElementException();
				
				// get more results from peer
				currentQueryResponse = handler.queryStateNext(txId, currentQueryResponse.getId());
				currentIterator = currentQueryResponse.getResultsList().iterator();
				
				// return next fetched result
				return mapper.apply(currentIterator.next());
				
			}
			
		};
	}

	@Override
	public void close() throws Exception {
		this.handler.queryStateClose(txId, currentQueryResponse.getId());
		this.currentIterator = Collections.emptyIterator();
		this.currentQueryResponse = QueryResponse.newBuilder().setHasMore(false).build();
	}

}
