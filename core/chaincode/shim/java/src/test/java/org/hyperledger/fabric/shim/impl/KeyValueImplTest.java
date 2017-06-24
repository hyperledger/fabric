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

import static java.nio.charset.StandardCharsets.UTF_8;
import static org.hamcrest.Matchers.equalTo;
import static org.hamcrest.Matchers.is;
import static org.junit.Assert.assertThat;

import org.hyperledger.fabric.protos.ledger.queryresult.KvQueryResult.KV;
import org.junit.Test;

import com.google.protobuf.ByteString;

public class KeyValueImplTest {

	@Test
	public void testKeyValueImpl() {
		new KeyValueImpl(KV.newBuilder()
				.setKey("key")
				.setValue(ByteString.copyFromUtf8("value"))
				.build());
	}

	@Test
	public void testGetKey() {
		KeyValueImpl kv = new KeyValueImpl(KV.newBuilder()
				.setKey("key")
				.setValue(ByteString.copyFromUtf8("value"))
				.build());
		assertThat(kv.getKey(), is(equalTo("key")));
	}

	@Test
	public void testGetValue() {
		KeyValueImpl kv = new KeyValueImpl(KV.newBuilder()
				.setKey("key")
				.setValue(ByteString.copyFromUtf8("value"))
				.build());
		assertThat(kv.getValue(), is(equalTo("value".getBytes(UTF_8))));
	}

	@Test
	public void testGetStringValue() {
		KeyValueImpl kv = new KeyValueImpl(KV.newBuilder()
				.setKey("key")
				.setValue(ByteString.copyFromUtf8("value"))
				.build());
		assertThat(kv.getStringValue(), is(equalTo("value")));
	}

}
