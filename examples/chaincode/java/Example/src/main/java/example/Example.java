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

import org.hyperledger.java.shim.ChaincodeBase;
import org.hyperledger.java.shim.ChaincodeStub;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;


/**
 * <h1>"Hello world" Chaincode</h1>
 *<b> Features:</b>
 *
 *<li> - in addition to framework-propagated (put\del\query) functions 
 * also provides no-op function "hello" so it could be invoked without executing stub code  @see shim.ChaincodeStub
 * <li>- query will greet you regardless if argument found in the map, but if found it will be personal greeting
 * <li>- put and del work as you would expect for Map implementation
 * 
 * <h2>Meant as default java Chaincode implementation e.g. invoked by default and required no init\prep work to check finctionality</h2> 
 * steps to invoke chaincode functions in dev mode:
 * <li>1. run node in dev mode e.g.:
'export CORE_LOGGING_LEVEL=debug
./peer node start --peer-chaincodedev'
 *<li>2.  run chaincode e.g. via gradle:
	'gradle run'
 *<li>3. Now we can communicate to chaincode via peer:
 <br>
<code>
./peer chaincode deploy -n hello -c '{"Function":"init","Args":[]}'<br>
./peer chaincode query -n hello -c '{"Function":"put","Args":["Me"]}'<br>
- get you argument echo back if not found  in the map <br><br>

./peer chaincode invoke -n hello -c '{"Function":"hello","Args":[""]}'<br>
- no-op test. invoke chaincode, but  not ChaincodeStub @see shim.ChaincodeStub, Handler @see shim.Handler 
hence no channel call and only effect is line in stdout<br><br>

./peer chaincode invoke -n hello -c '{"Function":"put","Args":["hey","me"]}'<br>
- put your name on the map<br><br>

./peer chaincode query -n hello -c '{"Function":"put","Args":["hey"]}'<br>
- get you argument echo back if not found  in the map<br><br>

./peer chaincode query -n hello -c '{"Function":"put","Args":["hey"]}'<br>
- personal greeting for mapped name<br>
</code>
 * 
 * @author Sergey Pomytkin spomytkin@gmail.com
 *
 */
public class Example extends ChaincodeBase {
	 private static Log log = LogFactory.getLog(Example.class);

	@Override
	public String run(ChaincodeStub stub, String function, String[] args) {
		log.info("In run, function:"+function);
		switch (function) {
		case "put":
			for (int i = 0; i < args.length; i += 2)
				stub.putState(args[i], args[i + 1]);
			break;
		case "del":
			for (String arg : args)
				stub.delState(arg);
			break;
		case "hello":
			System.out.println("hello invoked");
			log.info("hello invoked");
			break;
		}
		log.error("No matching case for function:"+function);
		return null;
	}

	@Override
	public String query(ChaincodeStub stub, String function, String[] args) {
		log.info("query");
		System.out.println("Hello world! function:"+function);
		log.debug("query:"+args[0]+"="+stub.getState(args[0]));
		if (stub.getState(args[0])!=null&&!stub.getState(args[0]).isEmpty()){
			log.trace("returning: Hello world! from "+ stub.getState(args[0]));
			return "Hello world! from "+ stub.getState(args[0]);
		}else{
			log.debug("No value found for key '"+args[0]+"'");
			return "Hello "+args[0]+"!";
		}
	}

	@Override
	public String getChaincodeID() {
		return "hello";
	}

	public static void main(String[] args) throws Exception {
		System.out.println("Hello world! starting "+args);
		log.info("starting");
		new Example().start(args);
	}


}
