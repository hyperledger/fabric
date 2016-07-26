## Hyperledger Fabric Client SDK for Node.js

The Hyperledger Fabric Client SDK (HFC) provides a powerful and easy to use API to interact with a Hyperledger Fabric blockchain.

To learn about how to install and use the Node.js SDK, please visit the [fabric documentation](http://hyperledger-fabric.readthedocs.io/en/latest/Setup/NodeSDK-setup).

The [Going Deeper](#going-deeper) section discusses HFC's pluggability and extensibility design. It also describes the main object hierarchy to help you get started in navigating the reference documentation. The top-level class is `Chain`.

The [Future Work](#future-work) section describes some upcoming work to be done.

   **WARNING**: To view the reference documentation correctly, you first need to build the SDK and confirm functionality by [running the unit tests](#running-unit-tests). Subsequently open the following URLs directly in your browser. Be sure to replace YOUR-FABRIC-DIR with the path to your fabric directory.

   `file:///YOUR-FABRIC-DIR/sdk/node/doc/modules/_hfc_.html`

   `file:///YOUR-FABRIC-DIR/sdk/node/doc/classes/_hfc_.chain.html`

### Going Deeper

#### Pluggability
HFC was designed to support two pluggable components:

1. Pluggable key value store which is used to retrieve and store keys associated with a member. The key value store is used to store sensitive private keys, so care must be taken to properly protect access.

2. Pluggable member service which is used to register and enroll members. Member services enables hyperledger to be a permissioned blockchain, providing security services such as anonymity, unlinkability of transactions, and confidentiality

#### HFC objects and reference documentation
HFC is written primarily in typescript and is object-oriented. The source can be found in the `fabric/sdk/node/src` directory.

To go deeper, you can view the reference documentation in your browser by opening the [reference documentation](doc/modules/_hfc_.html) and clicking on **"hfc"** on the right-hand side under **"Globals"**. This will work after you have built the SDK per the instruction [here](#building-the-client-sdk).

The following is a high-level description of the HFC objects (classes and interfaces) to help guide you through the object hierarchy.

* The main top-level class is [Chain](doc/classes/_hfc_.chain.html). It is the client's representation of a chain. HFC allows you to interact with multiple chains and to share a single [KeyValStore](doc/interfaces/_hfc_.keyvalstore.html) and [MemberServices](doc/interfaces/_hfc_.memberservices.html) object with multiple Chain objects as needed. For each chain, you add one or more [Peer](doc/classes/_hfc_.peer.html) objects which represents the endpoint(s) to which HFC connects to transact on the chain.

* The [KeyValStore](doc/interfaces/_hfc_.keyvalstore.html) is a very simple interface which HFC uses to store and retrieve all persistent data. This data includes private keys, so it is very important to keep this storage secure. The default implementation is a simple file-based version found in the [FileKeyValStore](doc/classes/_hfc_.filekeyvalstore.html) class.

* The [MemberServices](doc/interfaces/_hfc_.memberservices.html) interface is implemented by the [MemberServicesImpl](doc/classes/_hfc_.memberservicesimpl.html) class and provides security and identity related features such as privacy, unlinkability, and confidentiality. This implementation issues *ECerts* (enrollment certificates) and *TCerts* (transaction certificates). ECerts are for enrollment identity and TCerts are for transactions.

* The [Member](doc/classes/_hfc_.member.html) class most often represents an end user who transacts on the chain, but it may also represent other types of members such as peers. From the Member class, you can *register* and *enroll* members or users. This interacts with the [MemberServices](doc/interfaces/_hfc_.memberservices.html) object. You can also deploy, query, and invoke chaincode directly, which interacts with the [Peer](doc/classes/_hfc_.peer.html). The implementation for deploy, query and invoke simply creates a temporary [TransactionContext](doc/classes/_hfc_.transactioncontext.html) object and delegates the work to it.

* The [TransactionContext](doc/classes/_hfc_.transactioncontext.html) class implements the bulk of the deploy, invoke, and query logic. It interacts with MemberServices to get a TCert to perform these operations. Note that there is a one-to-one relationship between TCert and TransactionContext; in other words, a single TransactionContext will always use the same TCert. If you want to issue multiple transactions with the same TCert, then you can get a [TransactionContext](doc/classes/_hfc_.transactioncontext.html) object from a [Member](doc/classes/_hfc_.member.html) object directly and issue multiple deploy, invoke, or query operations on it. Note however that if you do this, these transactions are linkable, which means someone could tell that they came from the same user, though not know which user. For this reason, you will typically just call deploy, invoke, and query on the User or Member object.

## Future Work
The following is a list of known remaining work to be done.

* The reference documentation needs to be made simpler to follow as there are currently some internal classes and interfaces that are being exposed.

* We are investigating how to make deployment of a chaincode simpler by not requiring you to set up a specific directory structure with dependencies.

* Implement events appropriately, both custom and non-custom. The 'complete' event for `deploy` and `invoke` is currently implemented by simply waiting a set number of seconds (5 for invoke, 20 for deploy). It needs to receive a complete event from the server with the result of the transaction and make this available to the caller. This has not yet been implemented.

* Support SHA2. HFC currently supports SHA3.
