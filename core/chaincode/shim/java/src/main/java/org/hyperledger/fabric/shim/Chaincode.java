// Copyright IBM 2017 All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package org.hyperledger.fabric.shim;

import static java.nio.charset.StandardCharsets.UTF_8;

import java.util.HashMap;
import java.util.Map;

/**
 * Defines methods that all chaincodes must implement.
 */
public interface Chaincode {
	/**
	 * Called during an instantiate transaction after the container has been
	 * established, allowing the chaincode to initialize its internal data.
	 */
	public Response init(ChaincodeStub stub);

	/**
	 * Called for every Invoke transaction. The chaincode may change its state
	 * variables.
	 */
	public Response invoke(ChaincodeStub stub);

	public static class Response {

		private final Status status;
		private final String message;
		private final byte[] payload;

		public Response(Status status, String message, byte[] payload) {
			this.status = status;
			this.message = message;
			this.payload = payload;
		}

		public Status getStatus() {
			return status;
		}

		public String getMessage() {
			return message;
		}

		public byte[] getPayload() {
			return payload;
		}

		public String getStringPayload() {
			return new String(payload, UTF_8);
		}

		public enum Status {
			SUCCESS(200),
			INTERNAL_SERVER_ERROR(500);

			private static final Map<Integer, Status> codeToStatus = new HashMap<>();
			private final int code;

			private Status(int code) {
				this.code = code;
			}

			public int getCode() {
				return code;
			}

			public static Status forCode(int code) {
				final Status result = codeToStatus.get(code);
				if(result == null) throw new IllegalArgumentException("no status for code " + code);
				return result;
			}

			static {
				for (Status status : Status.values()) {
					codeToStatus.put(status.code, status);
				}
			}

		}

	}
}
