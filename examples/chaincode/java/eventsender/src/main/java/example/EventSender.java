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

package example;

import static java.lang.String.format;
import static java.nio.charset.StandardCharsets.UTF_8;
import static java.util.stream.Collectors.joining;

import java.util.List;

import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class EventSender extends ChaincodeBase {

	private static final String EVENT_COUNT = "noevents";

	@Override
	public Response init(ChaincodeStub stub) {
		stub.putStringState(EVENT_COUNT, Integer.toString(0));
		return newSuccessResponse();
	}

	@Override
	public Response invoke(ChaincodeStub stub) {

		try {
			final String function = stub.getFunction();

			switch (function) {
			case "invoke":
				return doInvoke(stub, stub.getParameters());
			case "query":
				return doQuery(stub);
			default:
				return newErrorResponse(format("Unknown function: %s", function));
			}

		} catch (Throwable e) {
			return newErrorResponse(e);
		}

	}

	private Response doInvoke(ChaincodeStub stub, List<String> args) {

		// get number of events sent
		final int eventNumber = Integer.parseInt(stub.getStringState(EVENT_COUNT));

		// increment number of events sent
		stub.putStringState(EVENT_COUNT, Integer.toString(eventNumber + 1));

		// create event payload
		final String payload = args.stream().collect(joining(",", "Event " + String.valueOf(eventNumber), ""));

		// indicate event to post with the transaction
		stub.setEvent("evtsender", payload.getBytes(UTF_8));

		return newSuccessResponse();
	}

	private Response doQuery(ChaincodeStub stub) {
		return newSuccessResponse(String.format("{\"NoEvents\":%d}", Integer.parseInt(stub.getStringState(EVENT_COUNT))));
	}

	public static void main(String[] args) throws Exception {
		new EventSender().start(args);
	}

}
