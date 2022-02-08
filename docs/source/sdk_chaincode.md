# Fabric Contract APIs and Application APIs

## Fabric Contract APIs

Hyperledger Fabric offers a number of APIs to support developing smart contracts (chaincode) in various programming languages.
Smart contract APIs are available for Go, Node.js, and Java.

* [Go contract API](https://github.com/hyperledger/fabric-contract-api-go) and [documentation](https://pkg.go.dev/github.com/hyperledger/fabric-contract-api-go).
* [Node.js contract API](https://github.com/hyperledger/fabric-chaincode-node) and [documentation](https://hyperledger.github.io/fabric-chaincode-node/).
* [Java contract API](https://github.com/hyperledger/fabric-chaincode-java) and [documentation](https://hyperledger.github.io/fabric-chaincode-java/).

## Fabric Application APIs

Hyperledger Fabric offers a Fabric Gateway client API to support developing applications in Go, Node.js, and Java. This API uses the Gateway peer capability introduced in Fabric v2.4 to interact with the Fabric network, and is an evolution of the new application programming model introduced in Fabric v1.4. The Fabric Gateway client API is the preferred API for developing applications for Fabric v2.4 onwards.

* [Fabric Gateway client API](https://github.com/hyperledger/fabric-gateway) and [documentation](https://hyperledger.github.io/fabric-gateway/).

Legacy application SDKs also exist for various programming languages, and can be used with Fabric v2.4. These application SDKs support versions of Fabric prior to v2.4, and do not require the Gateway peer capability. They also include some functionality, such as administrative actions for managing enrollment of identities with a Certificate Authority (CA), that are not offered by the Fabric Gateway API. Application SDKs are available for Go, Node.js and Java.

* [Node.js SDK](https://github.com/hyperledger/fabric-sdk-node) and [documentation](https://hyperledger.github.io/fabric-sdk-node/).
* [Java SDK](https://github.com/hyperledger/fabric-gateway-java) and [documentation](https://hyperledger.github.io/fabric-gateway-java/).
* [Go SDK](https://github.com/hyperledger/fabric-sdk-go) and [documentation](https://pkg.go.dev/github.com/hyperledger/fabric-sdk-go/).

Prerequisites for developing with the SDKs can be found in the
Node.js SDK [README](https://github.com/hyperledger/fabric-sdk-node#build-and-test),
Java SDK [README](https://github.com/hyperledger/fabric-gateway-java/blob/main/README.md), and
Go SDK [README](https://github.com/hyperledger/fabric-sdk-go/blob/main/README.md).

In addition, there is one other application SDK that has not yet been
officially released for Python, but is still available for downloading and testing:

* [Python SDK](https://github.com/hyperledger/fabric-sdk-py).
