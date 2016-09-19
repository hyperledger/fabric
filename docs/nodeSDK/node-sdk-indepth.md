# Hyperledger fabric Client (HFC) SDK for Node.js

The Hyperledger fabric Client (HFC) SDK provides a powerful and easy to use API to interact with a Hyperledger fabric blockchain.

Below, you'll find the following sections:

- [Installing only the SDK](#installing-only-the-sdk)
- [Terminology](#terminology)
- [HFC Objects](#hfc-objects)
- [Pluggability](#pluggability)
- [Chaincode Deployment](#chaincode-deployment)
- [Enabling TLS](#enabling-tls)
- [Troubleshooting](#troubleshooting)

## Installing only the SDK

If you are an experienced node.js developer and already have a blockchain environment set up and running elsewhere, you can set up a client-only environment to run the node.js client by installing the HFC node module as shown below.  This assumes a minimum of npm version 2.11.3 and node.js version 0.12.7 are already installed.

* To install the latest HFC module of Hyperledger fabric

```
  npm install hfc
```

### Terminology

In order to transact on a Hyperledger fabric blockchain, you must first have an identity which has been both **registered** and **enrolled** with Membership Services.  For a topological overview of how the components interact, see [Application Developer's Overview](app-overview.md).

Think of **registration** as *issuing a user invitation* to join a blockchain. It consists of adding a new user name (also called an *enrollment ID*) to the membership service configuration. This can be done programatically with the `Member.register` method, or by adding the enrollment ID directly to the [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml) configuration file.

Think of **enrollment** as *accepting a user invitation* to join a blockchain. This is always done by the entity that will transact on the blockchain. This can be done programatically via the `Member.enroll` method.

## HFC Objects

HFC is written primarily in typescript. The source can be found in the `fabric/sdk/node/src` directory.  The reference documentation is generated automatically from this source code and can be found  in`fabric/sdk/node/doc` after building the project.

The following is a high-level description of the HFC objects (classes and interfaces) to help guide you through the object hierarchy.

* **Chain**

  This is the main top-level class which is the client's representation of a chain. HFC allows you to interact with multiple chains and to share a single `KeyValStore` and `MemberServices` object with multiple chains if needed. For each chain, you add one or more `Peer` objects which represents the endpoint(s) to which HFC connects to transact on the chain.  The second peer is used only if the first peer fails.  The third peer is used only if both the first and second peers fail, etc.

* **KeyValStore**

  This is a very simple interface which HFC uses to store and retrieve all persistent data. This data includes private keys, so it is very important to keep this storage secure. The default implementation is a simple file-based version found in the `FileKeyValStore` class.  If running in a clustered web application, you will need to either ensure that a shared file system is used, or you must implement your own `KeyValStore` that can be shared among all cluster members.

* **MemberServices**

   This is an interface representing Membership Services and is implemented by the `MemberServicesImpl` class.  It provides security and identity related features such as privacy, unlinkability, and confidentiality. This implementation issues *ECerts* (enrollment certificates) and *TCerts* (transaction certificates). ECerts are for enrollment identity and TCerts are for transactions.

* **Member** or **User**

  The Member class most often represents an end User who transacts on the chain, but it may also represent other types of members such as peers. From the Member class, you can *register* and *enroll* members or users. This interacts with the `MemberServices` object. You can also deploy, query, and invoke chaincode directly, which interacts with the `Peer`. The implementation for deploy, query and invoke simply creates a temporary `TransactionContext` object and delegates the work to it.

* **TransactionContext**

  This class implements the bulk of the deploy, invoke, and query logic. It interacts with Membership Services to get a TCert to perform these operations. Note that there is a one-to-one relationship between TCert and TransactionContext.  In other words, a single TransactionContext will always use the same TCert. If you want to issue multiple transactions with the same TCert, then you can get a `TransactionContext` object from a `Member` object directly and issue multiple deploy, invoke, or query operations on it. Note however that if you do this, these transactions are linkable, which means someone could tell that they came from the same user, though not know which user. For this reason, you will typically just call deploy, invoke, and query on the Member or User object.

## Pluggability

HFC was designed to support two pluggable components:

1. Pluggable `KeyValStore` key value store which is used to retrieve and store keys associated with a member. The key value store is used to store sensitive private keys, so care must be taken to properly protect access.

  **IMPORTANT NOTE**: The default KeyValStore is file-based.  If multiple instances of a web application run in a cluster, you must provide an implementation of the KeyValStore which is used by all members of the cluster.

2. Pluggable `MemberServices` which is used to register and enroll members. Member services enables hyperledger to be a permissioned blockchain, providing security services such as anonymity, unlinkability of transactions, and confidentiality

## Chaincode Deployment

### 'net' mode

To have the chaincode deployment succeed in network mode, you must properly set
up the chaincode project outside of your Hyperledger fabric source tree to include all the **golang** dependencies such that when tarred up and sent to the peer, the peer will be able to build the chain code and then deploy it. The
following instructions will demonstrate how to properly set up the directory
structure to deploy *chaincode_example02* in network mode.

The chaincode project must be placed under the `$GOPATH/src` directory. For example, the [chaincode_example02](https://github.com/hyperledger/fabric/blob/master/examples/chaincode/go/chaincode_example02/chaincode_example02.go)
project should be placed under `$GOPATH/src/` as shown below.

```
  mkdir -p $GOPATH/src/github.com/chaincode_example02/
  cd $GOPATH/src/github.com/chaincode_example02
  curl GET https://raw.githubusercontent.com/hyperledger/fabric/master/examples/chaincode/go/chaincode_example02/chaincode_example02.go > chaincode_example02.go
```

After you have placed your chaincode project under the `$GOPATH/src`, you will need to vendor the dependencies. From the directory containing your chaincode source, run the following commands:

```
  go get -u github.com/kardianos/govendor
  cd $GOPATH/src/github.com/chaincode_example02
  govendor init
  govendor fetch github.com/hyperledger/fabric
```

Now, execute `go build` to verify that all of the chaincode dependencies are
present.

```
  go build
```

### 'dev' mode
For deploying chaincode in development mode see [Writing, Building, and Running Chaincode in a Development Environment](../Setup/Chaincode-setup.md). 
The chaincode must be running and connected to the peer before issuing the `deploy()` from the Node.js application. The hfc `chain` object must be set to dev mode.

```javascript
chain.setDevMode(true);
```
The deploy request must include the `chaincodeName` that the chaincode registered with the peer. The built-in chaincode example checks an environment variable `CORE_CHAINCODE_ID_NAME=mycc` when it starts.

```json
   var deployRequest = {
       chaincodeName: 'mycc',
       fcn: "init",
       args: ["a", "100", "b", "200"]
   };
```

### Enabling TLS

If you wish to configure TLS with the Membership Services server, the following steps are required:

- Modify `$GOPATH/src/github.com/hyperledger/fabric/membersrvc/membersrvc.yaml` as follows:

```
  server:
      tls:
         cert:
            file: "/var/hyperledger/production/.membersrvc/tlsca.cert"
         key:
            file: "/var/hyperledger/production/.membersrvc/tlsca.priv"
```

To specify to the Membership Services (TLS) Certificate Authority (TLSCA) what X.509 v3 Certificate (with a corresponding Private Key) to use:

- Modify `$GOPATH/src/github.com/hyperledger/fabric/peer/core.yaml` as follows:

```
   peer:
      pki:
          tls:
             enabled: true
             rootcert:
                 file: "/var/hyperledger/production/.membersrvc/tlsca.cert"
```

To configure the peer to connect to the Membership Services server over TLS (otherwise, the connection will fail).

- Bootstrap your Membership Services and the peer. This is needed in order to have the file *tlsca.cert* generated by the member services.

- Copy `/var/hyperledger/production/.membersrvc/tlsca.cert` to `$GOPATH/src/github.com/hyperledger/fabric/sdk/node`.

*Note:* If you cleanup the folder `/var/hyperledger/production` then don't forget to copy again the *tlsca.cert* file as described above.

### Troubleshooting
If you see errors stating that the client has already been registered/enrolled, keep in mind that you can perform the enrollment process only once, as the enrollmentSecret is a one-time-use password. You will see these errors if you have performed a user registration/enrollment and subsequently deleted the cryptographic tokens stored on the client side. The next time you try to enroll, errors similar to the ones below will be seen.

```
   Error: identity or token do not match

```

```
   Error: user is already registered

```

To address this, remove any stored cryptographic material from the CA server by following the instructions [here](https://github.com/hyperledger/fabric/blob/master/docs/Setup/Chaincode-setup.md#removing-temporary-files-when-security-is-enabled). You will also need to remove any of the cryptographic tokens stored on the client side by deleting the KeyValStore directory. That directory is configurable and is set to `/tmp/keyValStore` within the unit tests.
