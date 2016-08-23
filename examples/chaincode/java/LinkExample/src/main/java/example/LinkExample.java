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

import com.google.protobuf.ByteString;
import org.hyperledger.java.shim.ChaincodeBase;
import org.hyperledger.java.shim.ChaincodeStub;

import java.nio.charset.StandardCharsets;
import java.util.LinkedList;
import java.util.List;

public class LinkExample extends ChaincodeBase {

	//Default name for map chaincode in dev mode
	//Can be set to a hash location via init or setMap 
	private String mapChaincode = "map";
	
	@Override
	public String run(ChaincodeStub stub, String function, String[] args) {
		switch (function) {
		case "init":
		case "setMap":
			mapChaincode = args[0];
			break;
		case "put":
			stub.invokeChaincode(mapChaincode, function, toByteStringList(args));
		default:
			break;
		}
		return null;
	}

	@Override
	public String query(ChaincodeStub stub, String function, String[] args) {
		String tmp = stub.queryChaincode("map", function, toByteStringList(args));
		if (tmp.isEmpty()) tmp = "NULL";
		else tmp = "\"" + tmp + "\"";
		tmp += " (queried from map chaincode)";
		return tmp;
	}

	public static void main(String[] args) throws Exception {
		new LinkExample().start(args);
		//new Example().start();
	}

	@Override
	public String getChaincodeID() {
		return "link";
	}

	private List<ByteString> toByteStringList(String[] args) {
		LinkedList<ByteString> result = new LinkedList();
		for (int i=0; i<args.length; ++i) {
			result.add(ByteString.copyFrom(args[i].getBytes(StandardCharsets.UTF_8)));
		}
		return result;
	}
}
