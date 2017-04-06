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
import static org.hyperledger.java.shim.ChaincodeHelper.newBadRequestResponse;

import org.hyperledger.fabric.protos.peer.ProposalResponsePackage.Response;
import org.hyperledger.java.shim.ChaincodeBase;
import org.hyperledger.java.shim.ChaincodeStub;

public class Example06 extends ChaincodeBase {
	
	@Override
	public Response run(ChaincodeStub stub, String function, String[] args) {
		switch (function) {
		case "runtimeException":
			throw new RuntimeException("Exception thrown as requested.");
		default:
			return newBadRequestResponse(format("Unknown function: %s", function));
		}
	}
	
	@Override
	public String getChaincodeID() {
		return "Example03";
	}

	public static void main(String[] args) throws Exception {
		new Example06().start(args);
	}

}
