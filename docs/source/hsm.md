# Using a Hardware Security Module (HSM)

The cryptographic operations performed by Fabric nodes can be delegated to
a Hardware Security Module (HSM).  An HSM protects your private keys and
handles cryptographic operations, allowing your peers and orderer nodes to
sign and endorse transactions without exposing their private keys.  If you
require compliance with government standards such as FIPS 140-2, there are
multiple certified HSMs from which to choose.

Fabric currently leverages the PKCS11 standard to communicate with an HSM.


## Configuring an HSM

To use an HSM with your Fabric node, you need to update the `bccsp` (Crypto Service
Provider) section of the node configuration file such as core.yaml or
orderer.yaml. In the `bccsp` section, you need to select PKCS11 as the provider and
enter the path to the PKCS11 library that you would like to use. You also need
to provide the `Label` and `PIN` of the token that you created for your cryptographic
operations. You can use one token to generate and store multiple keys.

The prebuilt Hyperledger Fabric Docker images are not enabled to use PKCS11. If
you are deploying Fabric using docker, you need to build your own images and
enable PKCS11 using the following command:
```
make docker GO_TAGS=pkcs11
```
You also need to ensure that the PKCS11 library is available to be used by the
node by installing it or mounting it inside the container.

### Example

The following example demonstrates how to configure a Fabric node to use an HSM.

First, you will need to install an implementation of the PKCS11 interface. This
example uses the [softhsm](https://github.com/opendnssec/SoftHSMv2) open source
implementation. After downloading and configuring softhsm, you will need to set
the SOFTHSM2_CONF environment variable to point to the softhsm2 configuration
file.

You can then use softhsm to create the token that will handle the cryptographic
operations of your Fabric node inside an HSM slot. In this example, we create a
token labelled "fabric" and set the pin to "71811222". After you have created
the token, update the configuration file to use PKCS11 and your token as the
crypto service provider. You can find an example `bccsp` section below:

```
#############################################################################
# BCCSP (BlockChain Crypto Service Provider) section is used to select which
# crypto library implementation to use
#############################################################################
bccsp:
  default: PKCS11
  pkcs11:
    Library: /etc/hyperledger/fabric/libsofthsm2.so
    Pin: "71811222"
    Label: fabric
    hash: SHA2
    security: 256
    Immutable: false
```

By default, when private keys are generated using the HSM, the private key is mutable, meaning PKCS11 private key  attributes can be changed after the key is generated. Setting `Immutable` to `true` means that the private key attributes cannot be altered after key generation. Before you configure immutability by setting `Immutable: true`, ensure that PKCS11 object copy is supported by the HSM.

You can also use environment variables to override the relevant fields of the configuration file. If you are connecting to softhsm2 using the Fabric CA server, you could set the following environment variables or directly set the corresponding values in the CA server config file:

```
FABRIC_CA_SERVER_BCCSP_DEFAULT=PKCS11
FABRIC_CA_SERVER_BCCSP_PKCS11_LIBRARY=/etc/hyperledger/fabric/libsofthsm2.so
FABRIC_CA_SERVER_BCCSP_PKCS11_PIN=71811222
FABRIC_CA_SERVER_BCCSP_PKCS11_LABEL=fabric
```

If you are connecting to softhsm2 using the Fabric peer, you could set the following environment variables or directly set the corresponding values in the peer config file:

```
CORE_PEER_BCCSP_DEFAULT=PKCS11
CORE_PEER_BCCSP_PKCS11_LIBRARY=/etc/hyperledger/fabric/libsofthsm2.so
CORE_PEER_BCCSP_PKCS11_PIN=71811222
CORE_PEER_BCCSP_PKCS11_LABEL=fabric
```

If you are connecting to softhsm2 using the Fabric orderer, you could set the following environment variables or directly set the corresponding values in the orderer config file:

```
ORDERER_GENERAL_BCCSP_DEFAULT=PKCS11
ORDERER_GENERAL_BCCSP_PKCS11_LIBRARY=/etc/hyperledger/fabric/libsofthsm2.so
ORDERER_GENERAL_BCCSP_PKCS11_PIN=71811222
ORDERER_GENERAL_BCCSP_PKCS11_LABEL=fabric
```

If you are deploying your nodes using docker compose, after building your own
images, you can update your docker compose files to mount the softhsm library
and configuration file inside the container using volumes. As an example, you
would add the following environment and volumes variables to your docker compose
file:
```
  environment:
     - SOFTHSM2_CONF=/etc/hyperledger/fabric/config.file
  volumes:
     - /home/softhsm/config.file:/etc/hyperledger/fabric/config.file
     - /usr/local/Cellar/softhsm/2.1.0/lib/softhsm/libsofthsm2.so:/etc/hyperledger/fabric/libsofthsm2.so
```

## Setting up a network using HSM

If you are deploying Fabric nodes using an HSM, your private keys need to be
generated and stored inside the HSM rather than inside the `keystore` folder of the node's
local MSP folder. The `keystore` folder of the MSP will remain empty. Instead,
the Fabric node will use the subject key identifier of the signing certificate
in the `signcerts` folder to retrieve the private key from inside the HSM.
The process for creating the node MSP folder differs depending on whether you
are using a Fabric Certificate Authority (CA) your own CA.

### Before you begin

Before configuring a Fabric node to use an HSM, you should have completed the following steps:

1. Created a partition on your HSM Server and recorded the `Label` and `PIN` of the partition.
2. Followed instructions in the documentation from your HSM provider to configure an HSM Client that communicates with your HSM server.

### Using an HSM with a Fabric CA

You can set up a Fabric CA to use an HSM by making the same edits to the CA server configuration file as you would make to a peer or ordering node. Because you can use the Fabric CA to generate keys inside an HSM, the process of creating the local MSP folders is straightforward. Use the following steps:

1. Modify the `bccsp` section of the Fabric CA server configuration file and point to the `Label` and `PIN` that you created for your HSM. When the Fabric CA server starts, the private key is generated and stored in the HSM. If you are not concerned about exposing your CA signing certificate, you can skip this step and only configure an HSM for your peer or ordering nodes, described in the next steps.

2. Use the Fabric CA client to register the peer or ordering node identities with your CA.

3. Before you deploy a peer or ordering node with HSM support, you need to enroll the node identity by storing its private key in the HSM. Edit the `bccsp` section of the Fabric CA client config file or use the associated environment variables to point to the HSM configuration for your peer or ordering node. In the Fabric CA Client configuration file, replace the default `SW` configuration with the `PKCS11` configuration and provide the values for your own HSM:

  ```
  bccsp:
    default: PKCS11
    pkcs11:
      Library: /etc/hyperledger/fabric/libsofthsm2.so
      Pin: "71811222"
      Label: fabric
      hash: SHA2
      security: 256
      Immutable: false
  ```

  Then for each node, use the Fabric CA client to generate the peer or ordering node's MSP folder by enrolling against the node identity that you registered in step 2. Instead of storing the private key in the `keystore` folder of the associated MSP, the enroll command uses the node's HSM to generate and store the private key for the peer or ordering node. The `keystore` folder remains empty.

4. To configure a peer or ordering node to use the HSM, similarly update the `bccsp` section of the peer or orderer configuration file to use PKCS11 and provide the `Label` and `PIN`. Also, edit the value of the `mspConfigPath` (for a peer node) or the `LocalMSPDir` (for an ordering node) to point to the MSP folder that was generated in the previous step using the Fabric CA client. Now that the peer or ordering node is configured to use HSM, when you start the node it will be able sign and endorse transactions with the private key protected by the HSM.

### Using an HSM with your own CA

If you are using your own Certificate Authority to deploy Fabric components, you
can use an HSM using the following steps:

1. Configure your CA to communicate with an HSM using PKCS11 and create a `Label` and `PIN`.
Then use your CA to generate the private key and signing certificate for each
node, with the private key generated inside the HSM.

2. Use your CA to build the peer or ordering node MSP folder. Place the signing certificate that you generated in step 1 inside the `signcerts` folder. You can leave the `keystore` folder empty.

3. To configure a peer or ordering node to use the HSM, similarly update the `bccsp` section of the peer or orderer configuration file to use PKCS11 andand provide the `Label` and `PIN`. Edit the value of the `mspConfigPath` (for a peer node) or the `LocalMSPDir` (for an ordering node) to point to the MSP folder that was generated in the previous step using the Fabric CA client. Now that the peer or ordering node is configured to use HSM, when you start the node it will be able sign and endorse transactions with the private key protected by the HSM.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
