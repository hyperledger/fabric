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

package example;

import static java.lang.String.format;
import static org.hyperledger.fabric.shim.Chaincode.Response.Status.INTERNAL_SERVER_ERROR;

import java.util.List;

import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class LinkExample extends ChaincodeBase {

	/**
	 * Default name for map chaincode in dev mode. Can be set to a hash location
	 * via init or setMap
	 */
	private static final String DEFAULT_MAP_CHAINCODE_NAME = "map";

	private String mapChaincodeName = DEFAULT_MAP_CHAINCODE_NAME;

	@Override
	public Response init(ChaincodeStub stub) {
		return invoke(stub);
	}

	@Override
	public Response invoke(ChaincodeStub stub) {
		try {
			final String function = stub.getFunction();
			final List<String> args = stub.getParameters();

			switch (function) {
			case "init":
			case "setMap":
				this.mapChaincodeName = args.get(0);
				return newSuccessResponse();
			case "put":
				stub.invokeChaincodeWithStringArgs(this.mapChaincodeName, args);
			case "query":
				return doQuery(stub, args);
			default:
				return newErrorResponse(format("Unknown function: %s", function));
			}
		} catch (Throwable e) {
			return newErrorResponse(e);
		}
	}

	private Response doQuery(ChaincodeStub stub, List<String> args) {
		final Response response = stub.invokeChaincodeWithStringArgs(this.mapChaincodeName, args);
		if (response.getStatus().getCode() >= INTERNAL_SERVER_ERROR.getCode()) {
			return response;
		} else {
			return newSuccessResponse(String.format("\"%s\" (queried from %s chaincode)", response.getPayload(), this.mapChaincodeName));
		}
	}

	public static void main(String[] args) throws Exception {
		new LinkExample().start(args);
	}

}
