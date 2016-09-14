# Setting up the Full Hyperledger fabric Developer's Environment

1. See [Setting Up The Development Environment](../dev-setup/devenv.md) to set up your development environment.

2. Issue the following commands to build the Hyperledger fabric client (HFC) Node.js SDK including the API reference documentation  

   ```
   cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
   make all
   ```
   
3. Issue the following command where your Node.js application is located if you wish to use the `require("hfc")`, this will install the HFC locally.  

   ```
   npm install /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
   ```
   
   Or use point directly to the HFC directly by using the following require in your code:
   ```javascript
   require("/opt/gopath/src/github.com/hyperledger/fabric/sdk/node");
   ```
   
      
4. To see the API reference documentation which is built in step 2:
   ```
   cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node/doc
   ```

   The [Self Contained Node.js Environment](node-sdk-self-contained.md) will have the reference documentation already built and may be accessed by:
   ```
   docker exec -it nodesdk /bin/bash
   cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node/doc
   ```
