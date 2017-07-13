/*
Copyright DTCC 2016 All Rights Reserved.

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

import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

public class MapExample extends ChaincodeBase {

	@Override
	public String run(ChaincodeStub stub, String function, String[] args) {
		switch (function) {
		case "put":
			for (int i = 0; i < args.length; i += 2)
				stub.putState(args[i], args[i + 1]);
			break;
		case "del":
			for (String arg : args)
				stub.delState(arg);
			break;
		}
		return null;
	}

	@Override
	public String query(ChaincodeStub stub, String function, String[] args) {
		// if ("range".equals(function)) {
		// String build = "";
		// HashMap<String, String> range = stub.getStateByRange(args[0],
		// args[1], 10);
		// for (String s : range.keySet()) {
		// build += s + ":" + range.get(s) + " ";
		// }
		// return build;
		// }
		return stub.getState(args[0]);
	}

	public static void main(String[] args) throws Exception {
		new MapExample().start(args);
	}

}
