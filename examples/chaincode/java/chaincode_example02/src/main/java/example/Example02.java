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
import static java.nio.charset.StandardCharsets.UTF_8;

import javax.json.Json;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class Example02 extends ChaincodeBase {

	private static Log log = LogFactory.getLog(Example02.class);

	@Override
	public Response init(ChaincodeStub stub) {
		try {
			final String function = stub.getFunction();
			switch (function) {
			case "init":
				return init(stub, stub.getParameters().stream().toArray(String[]::new));
			default:
				return newErrorResponse(format("Unknown function: %s", function));
			}
		} catch (Throwable e) {
			return newErrorResponse(e);
		}
	}

	@Override
	public Response invoke(ChaincodeStub stub) {

		try {
			final String function = stub.getFunction();
			final String[] args = stub.getParameters().stream().toArray(String[]::new);

			switch (function) {
			case "invoke":
				return invoke(stub, args);
			case "delete":
				return delete(stub, args);
			case "query":
				return query(stub, args);
			default:
				return newErrorResponse(format("Unknown function: %s", function));
			}
		} catch (Throwable e) {
			return newErrorResponse(e);
		}

	}

	private Response init(ChaincodeStub stub, String[] args) {
		if (args.length != 4) throw new IllegalArgumentException("Incorrect number of arguments. Expecting: init(account1, amount1, account2, amount2)");

		final String accountKey1 = args[0];
		final String accountKey2 = args[2];
		final String account1Balance = args[1];
		final String account2Balance = args[3];

		stub.putStringState(accountKey1, new Integer(account1Balance).toString());
		stub.putStringState(accountKey2, new Integer(account2Balance).toString());

		return newSuccessResponse();
	}

	private Response invoke(ChaincodeStub stub, String[] args) {
		if (args.length != 3) throw new IllegalArgumentException("Incorrect number of arguments. Expecting: transfer(from, to, amount)");

		final String fromKey = args[0];
		final String toKey = args[1];
		final String amount = args[2];

		// get state of the from/to keys
		final String fromKeyState = stub.getStringState(fromKey);
		final String toKeyState = stub.getStringState(toKey);

		// parse states as integers
		int fromAccountBalance = Integer.parseInt(fromKeyState);
		int toAccountBalance = Integer.parseInt(toKeyState);

		// parse the transfer amount as an integer
		int transferAmount = Integer.parseInt(amount);

		// make sure the transfer is possible
		if (transferAmount > fromAccountBalance) {
			throw new IllegalArgumentException("Insufficient asset holding value for requested transfer amount.");
		}

		// perform the transfer
		log.info(format("Tranferring %d holdings from %s to %s", transferAmount, fromKey, toKey));
		int newFromAccountBalance = fromAccountBalance - transferAmount;
		int newToAccountBalance = toAccountBalance + transferAmount;
		log.info(format("New holding values will be: %s = %d, %s = %d", fromKey, newFromAccountBalance, toKey, newToAccountBalance));
		stub.putStringState(fromKey, Integer.toString(newFromAccountBalance));
		stub.putStringState(toKey, Integer.toString(newToAccountBalance));
		log.info("Transfer complete.");

		return newSuccessResponse(format("Successfully transferred %d assets from %s to %s.", transferAmount, fromKey, toKey));
	}

	private Response delete(ChaincodeStub stub, String args[]) {
		if (args.length != 1) throw new IllegalArgumentException("Incorrect number of arguments. Expecting: delete(account)");

		final String account = args[0];

		stub.delState(account);

		return newSuccessResponse();
	}

	private Response query(ChaincodeStub stub, String[] args) {
		if (args.length != 1) throw new IllegalArgumentException("Incorrect number of arguments. Expecting: query(account)");

		final String accountKey = args[0];

		return newSuccessResponse(Json.createObjectBuilder()
				.add("Name", accountKey)
				.add("Amount", Integer.parseInt(stub.getStringState(accountKey)))
				.build().toString().getBytes(UTF_8));

	}

	public static void main(String[] args) throws Exception {
		new Example02().start(args);
	}

}
