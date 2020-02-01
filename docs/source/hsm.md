# Using a Hardware Security Module (HSM)

You can use a Hardware Security Module (HSM) to generate and store the private
keys used by your Fabric nodes. An HSM protects your private keys and handles
cryptographic operations, which allows your peers and ordering nodes to sign and
endorse transactions without exposing their private keys. Currently, Fabric only
supports the PKCS11 standard to communicate with an HSM.

## Configuring an HSM

To use an HSM with your Fabric node, you need to update the BCCSP (Crypto Service
Provider) section of the node configuration file such as core.yaml or
orderer.yaml. In BCCSP section, you need to select PKCS11 as the provider and
provide the path to the PKCS11 library that you would like to use. You also need
to provide the label and pin of the token that you created for your cryptographic
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
crypto service provider. You can find an example BCCSP section below:

```
#############################################################################
# BCCSP (BlockChain Crypto Service Provider) section is used to select which
# crypto library implementation to use
#############################################################################
bccsp:
  default: PKCS11
  pkcs11:
    Library: /etc/hyperledger/fabric/libsofthsm2.so
    Pin: 71811222
    Label: fabric
    hash: SHA2
    security: 256
```

You can also use environment variables to override the relevant fields of the
configuration file. If you are connecting to an HSM using the Fabric CA server,
you need to set the following environment variables:

```
FABRIC_CA_SERVER_BCCSP_DEFAULT=PKCS11
FABRIC_CA_SERVER_BCCSP_PKCS11_LIBRARY=/etc/hyperledger/fabric/libsofthsm2.so
FABRIC_CA_SERVER_BCCSP_PKCS11_PIN=71811222
FABRIC_CA_SERVER_BCCSP_PKCS11_LABEL=fabric
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
generated inside the HSM rather than inside the `keystore` folder of the node's
local MSP folder. The `keystore` folder of the MSP will remain empty. Instead,
the Fabric node will use the subject key identifier of the signing certificate
in the `signcerts` folder to retrieve the private key from inside the HSM.
The process for creating the MSP folders will be different depending on if you
are using a Fabric Certificate Authority (CA) your own CA.

### Using a Fabric CA

You can set up a Fabric CA to use an HSM by making the same edits to the
configuration file as you would make to a peer or ordering node. Because you can
use Fabric CA to generate keys inside an HSM, the process of creating the local
MSP folders is straightforward. Use the following steps:

1. Create an HSM token and point to it in the Fabric CA server configuration
file. When the Fabric CA server starts, it will generate the CA signing
certificate inside your HSM. If you are not concerned about exposing your CA
signing certificate, you can skip this step.

2. Use the Fabric CA client to register the peer or ordering node identities
with your CA.

3. Edit the Fabric CA client config file or environment variables to use your
HSM as the crypto service provider. Then for each node, use the Fabric CA client
to generate the component MSP folder by enrolling against the node identity. The
enroll command will generate the private key inside your HSM.

3. Update the BCCSP section of the peer or orderer configuration file to use
PKCS11 and your token as the crypto service provider. Point to the MSP that was
generated using the Fabric CA client. Once it is deployed, the peer or orderer
node will be able sign and endorse transactions with the private key protected by
the HSM.

### Using an HSM with your own CA

If you are using your own Certificate Authority to deploy Fabric components, you
can use an HSM using the following steps:

1. Configure your CA to communicate with an HSM using PKCS11 and create a token.
Then use your CA to generate the private key and signing certificate for each
node, with the private key generated inside the HSM.

2. Use your CA to build the node MSP folder. Place the signing certificate that
you generated in step 1 inside the `signcerts` folder. You can leave the
`keystore` folder empty.

3. Update the peer or orderer configuration file to use PKCS11 and your token as
the crypto service provider. Point to the MSP folder that you created with the
signing certificate inside. Once it has deployed, the peer or ordering node will
be able to sign and endorse transactions using the HSM.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
