## Hyperledger Fabric Client (HFC) SDK for Node.js

The Hyperledger Fabric Client (HFC) SDK provides a powerful and easy to use API
to interact with a Hyperledger Fabric blockchain.

This document assumes that you already have set up a Node.js development
environment. If not, go [here](https://nodejs.org/en/download/package-manager/)
to download and install Node.js for your OS. You'll also want the latest version
of `npm` installed. For that, execute `sudo npm install npm -g` to get the
latest version.

### Installing the hfc module

We publish the `hfc` node module to `npm`. To install `hfc` from npm simply
execute the following command:

```
npm install -g hfc
```

See [Hyperledger fabric Node.js client SDK](../nodeSDK/node-sdk-guide.md) for more information.


## Hyperledger fabric network

First, you'll want to have a running peer node and member services. The
instructions for setting up a network are [here](Network-setup.md). You may also use the [Fabric-starter-kit](../starter/fabric-starter-kit.md) that provides the network.
