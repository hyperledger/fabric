// Copyright IBM 2017 All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//          http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package example;

import static java.lang.String.format;
import static java.nio.charset.StandardCharsets.UTF_8;
import static org.hyperledger.fabric.shim.Chaincode.Response.Status.INTERNAL_SERVER_ERROR;

import java.util.Arrays;
import java.util.List;

import javax.json.Json;

import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class Example04 extends ChaincodeBase {

	@Override
	public Response init(ChaincodeStub stub) {
		// expects to be called with: { "init", key, value }
		try {

			final List<String> args = stub.getStringArgs();
			if (args.size() != 3) {
				return newErrorResponse("Incorrect number of arguments. Expecting \"init\" plus 2 more.");
			}

			final String key = args.get(1);
			final int value = Integer.parseInt(args.get(2));
			stub.putStringState(key, String.valueOf(value));

		} catch (NumberFormatException e) {
			return newErrorResponse("Expecting integer value for sum");
		} catch (Throwable t) {
			return newErrorResponse(t);
		}

		return newSuccessResponse();
	}

	@Override
	public Response invoke(ChaincodeStub stub) {
		// expects to be called with: { "invoke"|"query", chaincodeName, key }
		try {
			final String function = stub.getFunction();
			final String[] args = stub.getParameters().stream().toArray(String[]::new);

			switch (function) {
			case "invoke":
				return doInvoke(stub, args);
			case "query":
				return doQuery(stub, args);
			default:
				return newErrorResponse(format("Unknown function: %s", function));
			}
		} catch (Throwable e) {
			return newErrorResponse(e);
		}
	}

	private Response doQuery(ChaincodeStub stub, String[] args) {
		switch (args.length) {
		case 1:
		case 2:
		case 3: {
			final String key = args[0];
			final int value = Integer.parseInt(stub.getStringState(key));
			return newSuccessResponse(
					Json.createObjectBuilder()
						.add("Name", key)
						.add("Amount", value)
						.build().toString().getBytes(UTF_8)
					);
		}
		case 4: {
			final String chaincodeToCall = args[1];
			final String key = args[2];
			final String channel = args[3];

			// invoke other chaincode
			final Response response = stub.invokeChaincodeWithStringArgs(chaincodeToCall, Arrays.asList(new String[] { "query", key }), channel);

			// check for error
			if (response.getStatus().getCode() >= INTERNAL_SERVER_ERROR.getCode()) {
				return response;
			}

			// return payload
			return newSuccessResponse(response.getPayload());
		}
		default:
			throw new AssertionError("Incorrect number of arguments. Expecting 1 or 4.");
		}
	}

	private Response doInvoke(ChaincodeStub stub, String[] args) {
		if (args.length != 3 && args.length != 4) throw new AssertionError("Incorrect number of arguments. Expecting 3 or 4");

		// the other chaincode's name
		final String nameOfChaincodeToCall = args[0];

		// other chaincode's channel
		final String channel;
		if (args.length == 4) {
			channel = args[3];
		} else {
			channel = null;
		}

		// state key to be updated
		final String key = args[1];

		// state value to be stored upon success
		final int value = Integer.parseInt(args[2]);

		// invoke other chaincode
		final Response response = stub.invokeChaincodeWithStringArgs(nameOfChaincodeToCall, Arrays.asList("invoke", "a", "b", "10"), channel);

		// check for error
		if (response.getStatus().getCode() >= INTERNAL_SERVER_ERROR.getCode()) {
			return newErrorResponse(format("Failed to query chaincode: %s", response.getMessage()), response.getPayload());
		}

		// update the ledger to indicate a successful invoke
		stub.putStringState(key, String.valueOf(value));

		// return the called chaincode's response
		return response;
	}

	public static void main(String[] args) throws Exception {
		new Example04().start(args);
	}

}
