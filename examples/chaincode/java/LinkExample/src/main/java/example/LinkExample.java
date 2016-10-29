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
import org.hyperledger.java.shim.ChaincodeBase;
import org.hyperledger.java.shim.ChaincodeStub;

public class LinkExample extends ChaincodeBase {

	//Default name for map chaincode in dev mode
	//Can be set to a hash location via init or setMap 
	private static Log log = LogFactory.getLog(LinkExample.class);
	private int id = 0;
	
	@Override
	public String run(ChaincodeStub stub, String function, String[] args) {
		log.info("In run, function:" + function);
		switch (function) {
//		case "del":
//			for (String arg : args)
//				stub.delState(arg);
//			break;
//		case "put":
//			for (int i = 0; i < args.length; i += 2)
//				stub.putState(args[i], args[i + 1]);
//			break;
		case "deploy":
			for (String arg : args)
				stub.putState(Integer.toString(id++), arg);
			break;
		}
		log.error("No matching case for function:" + function);
		return null;
	}

	@Override
	public String query(ChaincodeStub stub, String function, String[] args) {
		log.info("query");
		log.debug("query:" + args[0] + "=" + stub.getState(args[0]));
		if (stub.getState(args[0]) != null && !stub.getState(args[0]).isEmpty()) {
			log.trace("returning: message " + stub.getState(args[0]));
			return stub.getState(args[0]);
		} else {
			log.debug("No value found " + args[0] + "'");
			return "Hello " + args[0] + "!";
		}
//		String tmp = stub.queryChaincode("map", function, toByteStringList(args));
//		if (tmp.isEmpty()) tmp = "NULL";
//		else tmp = "\"" + tmp + "\"";
//		tmp += " (queried from map chaincode)";
//		return tmp;
	}

	public static void main(String[] args) throws Exception {
		new LinkExample().start(args);
		//new Example().start();
	}

	@Override
	public String getChaincodeID() {
		return "e-voting";
	}
}
