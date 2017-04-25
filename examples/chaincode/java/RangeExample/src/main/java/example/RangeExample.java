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

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.hyperledger.fabric.shim.ChaincodeBase;
import org.hyperledger.fabric.shim.ChaincodeStub;

import java.util.Map;

/**
 * Created by cadmin on 6/30/16.
 */
public class RangeExample extends ChaincodeBase {
	private static Log log = LogFactory.getLog(RangeExample.class);

	@java.lang.Override
	public String run(ChaincodeStub stub, String function, String[] args) {
		log.info("In run, function:" + function);
		switch (function) {
		case "put":
			for (int i = 0; i < args.length; i += 2)
				stub.putState(args[i], args[i + 1]);
			break;
		case "del":
			for (String arg : args)
				stub.delState(arg);
			break;
		default:
			log.error("No matching case for function:" + function);

		}
		return null;
	}

	@java.lang.Override
	public String query(ChaincodeStub stub, String function, String[] args) {
		log.info("query");
		switch (function) {
		case "get": {
			return stub.getState(args[0]);
		}
		case "keys": {
			Map<String, String> keysIter = null;
			if (args.length >= 2) {
				keysIter = stub.getStateByRange(args[0], args[1]);
			} else {
				keysIter = stub.getStateByRange("", "");
			}

			return keysIter.keySet().toString();
		}
		default:
			log.error("No matching case for function:" + function);
			return "";
		}

	}

	public static void main(String[] args) throws Exception {
		log.info("starting");
		new RangeExample().start(args);
	}

}
