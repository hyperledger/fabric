# Hyperledger Fabric Client SDK for Node.js

## Overview

The Hyperledger Fabric Client SDK (HFC) provides a powerful and easy to use API to interact with a [Hyperledger Fabric](https://www.hyperledger.org/) blockchain from a Node.js application. It provides a set of APIs to register and enroll new network clients, to deploy new chaincodes to the network, and to interact with existing chaincodes through chaincode function invocations and queries.

To view the complete Node.js SDK documentation, additional usage samples, and getting started materials please visit the Hyperledger Fabric [SDK documentation page](http://hyperledger-fabric.readthedocs.io/en/latest/nodeSDK/node-sdk-guide/).

## Installation

If you are interfacing with a network running the Hyperledger Fabric [`v0.5-developer-preview`](https://github.com/hyperledger-archives/fabric/tree/v0.5-developer-preview) branch, you must use the hfc version 0.5.x release and install it with the following command:

    npm install hfc@0.5.x

For networks running newer versions of the Hyperledger Fabric, please install the version of hfc that corresponds to the same version of the Fabric code base. For example, for the Fabric v0.6 branch install hfc with the following command:

    npm install hfc@0.6.x

## Usage

### Terminology

In order to transact on a Hyperledger blockchain, you must first have an identity which is both **registered** and **enrolled**. **Registration** means issuing a user invitation to join a blockchain network. **Enrollment** means accepting a user invitation to join a blockchain network.

In the current Hyperleger Fabric implementation, the administative user admin is already registered on the network by the virtue of the fact that their user ID and password pair are already added to the membership services server confiruation file, [membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml). Therefore, the admin user must only enroll to be able to transact on the network. After enrollment, the administrative user is then able to register and enroll new users, in the role of a registrar. New users are added to the network programmatically, through the HFC `chain.registerAndEnroll` API.

### Example

An example presented in the [SDK documentation](http://hyperledger-fabric.readthedocs.io/en/latest/nodeSDK/sample-standalone-app/) demonstrated a typical application using the hfc.

The application sets up a target for the membership service server and enrolls the `admin` user. Subsequently, the admin user is set as the registrar user and registers and enrolls a new member, `JohnDoe`. The new member, `JohnDoe`, is then able to transact on the blockchain by deploying a chaincode, invoking a chaincode function, and querying chaincode state.

### Chaincode Deployment

Chaincode may be deployed in **development** mode or **network** mode. **Development** mode refers to running the chaincode as a stand alone process outside the network peer. This approach is useful while doing chaincode development and testing. The approach is described [here](http://hyperledger-fabric.readthedocs.io/en/latest/Setup/Chaincode-setup/). **Network** mode refers to packaging the chaincode and its dependencies and subsequently deploying the package to an existing network as a Docker container. To have the chaincode deployment with hfc succeed in network mode, you must properly set up the chaincode project and its dependencies. The instructions for doing so can be found in the [Chaincode Deployment](http://hyperledger-fabric.readthedocs.io/en/latest/nodeSDK/node-sdk-indepth/#chaincode-deployment) section of the Node.js SDK documentation page.

### Enabling TLS

If you wish to enable TLS connection between the member services, the peers, and the chaincode container, please see the [Enabling TLS](http://hyperledger-fabric.readthedocs.io/en/latest/nodeSDK/node-sdk-indepth/#enabling-tls) section within the Node.js SDK documentation.

## Objects, Classes, And Interfaces

HFC is written primarily in Typescript and is object-oriented. The source can be found in the [fabric/sdk/node](https://github.com/hyperledger/fabric/tree/master/sdk/node) directory of the Hyperledger Fabric project. To better understand the object hierarchy, please read the high-level descriptions of the HFC objects (classes and interfaces) in the [HFC Objects](http://hyperledger-fabric.readthedocs.io/en/latest/nodeSDK/node-sdk-indepth/#hfc-objects) section within the Node.js SDK documentation. You can also build the complete reference documentation locally by following the instructions [here](http://hyperledger-fabric.readthedocs.io/en/latest/nodeSDK/app-developer-env-setup/).

## Unit Tests

HFC includes a set of unit tests implemented with the [tape framework](https://github.com/substack/tape). Currently, the unit tests are designed to run inside the Hyperledger Fabric Vagrant development environment. To run the unit tests, first set up the Hyperledger Fabric Vagrant development environment per the instructions [here](http://hyperledger-fabric.readthedocs.io/en/latest/dev-setup/devenv/).

Launch the Vagrant VM and log into the VM by executing the following command:

    vagrant ssh

Run the unit tests within the Vagrant environment with the following commands:

    cd $GOPATH/src/github.com/hyperledger/fabric
    make node-sdk-unit-tests

The following are brief descriptions of each of the unit tests that are being run.

#### registrar

The [registrar.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/registrar.js) test case exercises registering users with the membership services server. It also tests registering a designated registrar user which can then register additional users.

#### chain-tests

The [chain-tests.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/chain-tests.js) test case exercises the [chaincode_example02.go](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go/chaincode_example02) chaincode when it has been deployed in both development mode and network mode.

#### asset-mgmt

The [asset-mgmt.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/asset-mgmt.js) test case exercises the [asset_management.go](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go/asset_management) chaincode when it has been deployed in both development mode and network mode.

#### asset-mgmt-with-roles

The [asset-mgmt-with-roles.js](https://github.com/hyperledger/fabric/blob/master/sdk/node/test/unit/asset-mgmt-with-roles.js) test case exercises the [asset_management_with_roles.go](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go/asset_management_with_roles) chaincode when it has been deployed in both development mode and network mode.

## Basic Troubleshooting

Keep in mind that you can perform the enrollment process with the membership services server only once, as the enrollmentSecret is a one-time-use password. If you have performed a user registration/enrollment with the membership services and subsequently deleted the crypto tokens stored on the client side, the next time you try to enroll, errors similar to the ones below will be seen.

   ```
   Error: identity or token do not match
   ```
   ```
   Error: user is already registered
   ```

To address this, remove any stored crypto material from the CA server by following the instructions [here](https://github.com/hyperledger/fabric/blob/master/docs/Setup/Chaincode-setup.md#removing-temporary-files-when-security-is-enabled). You will also need to remove any of the crypto tokens stored on the client side by deleting the KeyValStore directory. That directory is configurable and is set to `/tmp/keyValStore` within the unit tests.

## Support

We use [Hyperledger Slack](https://hyperledgerproject.slack.com/) for communication regarding the SDK and any potential issues. Please join SDK related channels on Slack such as #fabric-sdk and #nodesdk. If you would like to file a bug report or submit a request for a feature please do so on Jira [here](https://jira.hyperledger.org).

## Contributing

We welcome any contributions to the Hyperledger Fabric Node.js SDK project. If you would like to contribute please read the contribution guidelines [here](http://hyperledger-fabric.readthedocs.io/en/latest/CONTRIBUTING/) for more information.

## License

Contributed to the Hyperledger Project under the [Apache Software License 2.0](https://github.com/hyperledger/fabric/blob/master/LICENSE).
