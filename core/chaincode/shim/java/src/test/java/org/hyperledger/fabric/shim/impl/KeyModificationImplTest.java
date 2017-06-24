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
import static org.hamcrest.Matchers.hasProperty;
import static org.hamcrest.Matchers.is;
import static org.junit.Assert.assertThat;

import java.util.stream.Stream;

import org.hyperledger.fabric.protos.ledger.queryresult.KvQueryResult;
import org.hyperledger.fabric.shim.ledger.KeyModification;
import org.junit.Test;

import com.google.protobuf.ByteString;
import com.google.protobuf.Timestamp;

public class KeyModificationImplTest {

	@Test
	public void testKeyModificationImpl() {
		new KeyModificationImpl(KvQueryResult.KeyModification.newBuilder()
				.setTxId("txid")
				.setValue(ByteString.copyFromUtf8("value"))
				.setTimestamp(Timestamp.newBuilder()
						.setSeconds(1234567890)
						.setNanos(123456789))
				.setIsDelete(true)
				.build()
				);
	}

	@Test
	public void testGetTxId() {
		final KeyModification km = new KeyModificationImpl(KvQueryResult.KeyModification.newBuilder()
				.setTxId("txid")
				.build()
				);
		assertThat(km.getTxId(), is(equalTo("txid")));
	}

	@Test
	public void testGetValue() {
		final KeyModification km = new KeyModificationImpl(KvQueryResult.KeyModification.newBuilder()
				.setValue(ByteString.copyFromUtf8("value"))
				.build()
				);
		assertThat(km.getValue(), is(equalTo("value".getBytes(UTF_8))));
	}

	@Test
	public void testGetStringValue() {
		final KeyModification km = new KeyModificationImpl(KvQueryResult.KeyModification.newBuilder()
				.setValue(ByteString.copyFromUtf8("value"))
				.build()
				);
		assertThat(km.getStringValue(), is(equalTo("value")));
	}

	@Test
	public void testGetTimestamp() {
		final KeyModification km = new KeyModificationImpl(KvQueryResult.KeyModification.newBuilder()
				.setTimestamp(Timestamp.newBuilder()
						.setSeconds(1234567890L)
						.setNanos(123456789))
				.build()
				);
		assertThat(km.getTimestamp(), hasProperty("epochSecond", equalTo(1234567890L)));
		assertThat(km.getTimestamp(), hasProperty("nano", equalTo(123456789)));
	}

	@Test
	public void testIsDeleted() {
		Stream.of(true, false)
			.forEach(b -> {
				final KeyModification km = new KeyModificationImpl(KvQueryResult.KeyModification.newBuilder()
						.setIsDelete(b)
						.build()
						);
				assertThat(km.isDeleted(), is(b));
			});
	}

}
