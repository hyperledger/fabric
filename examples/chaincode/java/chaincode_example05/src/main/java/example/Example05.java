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

import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class Example05 extends ChaincodeBase {

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
		// query is the same as invoke, but with response payload wrapped in json
		final Response result = doInvoke(stub, args);
		if (result.getStatus().getCode() >= INTERNAL_SERVER_ERROR.getCode()) {
			return result;
		} else {
			return newSuccessResponse(format("{\"Name\":\"%s\",\"Value\":%s}", args[0], result.getStringPayload()));
		}
	}

	private Response doInvoke(ChaincodeStub stub, String[] args) {
		if (args.length != 2) throw new AssertionError("Incorrect number of arguments. Expecting 2");

		// the other chaincode's id
		final String chaincodeName = args[0];

		// key containing the sum
		final String key = args[1];

		// query other chaincode for value of key "a"
		final Response queryResponseA = stub.invokeChaincodeWithStringArgs(chaincodeName, Arrays.asList(new String[] { "query", "a" }));

		// check for error
		if (queryResponseA.getStatus().getCode() >= INTERNAL_SERVER_ERROR.getCode()) {
			return newErrorResponse(format("Failed to query chaincode: %s", queryResponseA.getMessage()), queryResponseA.getPayload());
		}

		// parse response
		final int a = Integer.parseInt(queryResponseA.getStringPayload());

		// query other chaincode for value of key "b"
		final Response queryResponseB = stub.invokeChaincodeWithStringArgs(chaincodeName, "query", "b");

		// check for error
		if (queryResponseB.getStatus().getCode() >= INTERNAL_SERVER_ERROR.getCode()) {
			return newErrorResponse(format("Failed to query chaincode: %s", queryResponseB.getMessage()), queryResponseB.getPayload());
		}

		// parse response
		final int b = Integer.parseInt(queryResponseB.getStringPayload());

		// calculate sum
		final int sum = a + b;

		// write new sum to the ledger
		stub.putStringState(key, String.valueOf(sum));

		// return sum as string in payload
		return newSuccessResponse(String.valueOf(sum).getBytes(UTF_8));
	}

	public static void main(String[] args) throws Exception {
		new Example05().start(args);
	}

}
