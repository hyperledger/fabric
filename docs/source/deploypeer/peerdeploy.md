# Deploy the peer

Before deploying a peer, make sure to digest the material in [Planning for a peer](./peerplan.html) and [Checklist for a production peer](./peerchecklist.html) which discusses all of the relevant decisions you need to make and parameters you need to configure before deploying a peer.

## Download the peer binary and configuration files

The Fabric peer binary and configuration files can be downloaded from [GitHub](https://github.com/hyperledger/fabric/releases) to a folder on your local system for example `fabric/`. Scroll to the Fabric release you want to download, click the **Assets** twistie, and select the binary for your system type. Extract the ZIP file and you will find all of the Fabric binaries in the `/bin` folder and the associated configuration files in the `/config` folder.
The resulting folder structure is similar to:

```
├── fabric
  ├── bin
  │   ├── configtxgen
  │   ├── configtxlator
  │   ├── cryptogen
  │   ├── discover
  │   ├── idemixgen
  │   ├── orderer
  │   └── peer
  └── config
    ├── configtx.yaml
    ├── core.yaml
    └── orderer.yaml
```

Along with the relevant binaries, you will receive both the peer binary executable and the peer configuration file, `core.yaml`, that is required to launch a peer on the network. The other files are not required for the peer deployment but will be useful when you attempt to create or edit channels, among other tasks.

After you have mastered deploying and running a peer by using these two files, it is likely you will want to use the peer image that Fabric publishes instead, for example in a Kubernetes or Docker deployment. For now though, the purpose of this topic is to teach you how to properly use the peer binary.

**Tip:** You may want to add the location of the peer binary to your PATH environment variable so that it can be picked up without fully qualifying the path to the binary executable, for example:

```
export PATH=<path to download location>/bin:$PATH
```

## Prerequisites

Before you can launch a peer node in a production network, you need to configure it. Complete the following steps:

1. While **cryptogen** is a convenient utility to generate certificates for a test network, it should never be used on a production network. The core requirement for certificates for Fabric nodes is that they are Elliptic Curve (EC) certificates. You can use any tool you prefer as the certificate authority for issuing these certificates. For example, OpenSSL can be used, but the Fabric CA streamlines the process when it generates the Membership Service Providers (MSPs) for you. Regardless of which certificate provider you choose, you should have already used it to generate the peer enrollment, TLS certificate, private key, and the MSPs that Fabric must consume. Refer to the [CA deployment](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/cadeploy.html) and [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topics for instructions on how to generate these certificates. You need to generate the following sets of certificates:
    - **Peer TLS certificates**
    - **Peer organization admin certificates (organization MSP)**
    - **Peer enrollment certificates (peer local MSP)**  
2. Create the recommended folder structure described in the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic to store the generated certificates and MSPs:
    ```
    ├── organizations
      └── peerOrganizations
          └── org1.example.com
              ├── msp
              └── peers
                  └── peer0.org1.example.com
                      ├── msp
                      └── tls
    ```
3. Copy the generated certificates to their designated folders.
      - **Peer TLS certificate and private key**:
        - Copy the TLS CA root `ca-cert.pem` to `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/tls-ca-cert.pem`. The path and name of the certificate corresponds to `tls.rootcert.file` parameter in `core.yaml`.
        - After you have generated the peer TLS certificate, rename the generated private key in the `keystore` folder to `peer0-key.pem` so that it can more easily be recognized later on as being a private key.
        - Copy the peer TLS certificate and private key to `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls`. The path and name of these certificates correspond to the `tls.cert.file` and `tls.key.file` parameters in the `core.yaml`.
      - **Peer organization admin MSP**:
        - After you have registered the peer organization admin user and enrolled the identity, copy the generated MSP folder to `peerOrganizations/org1.example.com/msp`. This is the peer organization MSP definition and is used by every peer node in the organization. This path becomes the value of the `mspConfigPath` parameter in the `core.yaml` file.
        - Fabric introduced the concept of "Node Organization Unit (OU)" support that allows a role to be conferred onto an identity by setting an OU value inside a certificate. [Node OU](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#specifying-nodeous) support is enabled by creating a `config.yaml` file inside the `/msp` folder in the `organizations/peerOrganizations/org1.example.com` path in which a role is tied to the root of trust for the issued certificate. **Important:** After you generate the file, remember to edit it and set the path and file name of the root CA certificate for each role. The name of the root CA certificate file is included in the generated MSP under `/msp/cacerts`).
      - **Peer enrollment certificate and private key**:
        - After you have registered the peer user for this node and enrolled its identity with the peer organization CA, copy the generated `msp` folder and replace the **local MSP** with it under `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/msp`.
        - This MSP contains the signed certificate (public key) and the private key for the peer. The private key is used by the node to sign transactions, and is therefore not shared and must be secured. For maximum security, a Hardware Security Module (HSM) can be configured to generate and store this private key.
4. Provision storage for your ledger. If you are not using an external chaincode builder and launcher, you should factor in storage for that as well. The default location is `/var/hyperledger/production`. Ensure that your peer has write access to the folder. If you choose to use a different location, you need to specify that path in the `fileSystemPath` parameter in the `core.yaml` file.
5. Now use the [Checklist for a production peer](./peerchecklist.html) to modify the default settings in the `core.yaml` file. In the future, if you decide to deploy the peer through Kubernetes or Docker, you can override the same default settings by using environment variables instead.
6. Set the value of the FABRIC_CFG_PATH to be the location of the `core.yaml` file. When you run the peer binary from the `fabric/bin` folder, it would point to the  `/config` folder:
    ```
    export FABRIC_CFG_PATH=../config
    ```

## Start the peer

After `core.yaml` has been configured and your deployment backend is ready, you can simply start the peer node with the following command:

```
cd bin
./peer node start
```

When the peer starts successfully, you should see a message similar to:

```
[nodeCmd] serve -> INFO 017 Started peer with ID=[peer0.org1.example.com], network ID=[prod], address=[peer0.org1.example.com:7060]
```

## Next steps

In order to be able to transact on a network, the peer must be joined to a channel. The organization the peer belongs to must be a member of a channel before one of its peers can be joined to it. Note that if an organization wants to create a channel, it must be a member of the consortium hosted by the ordering service. Once a peer is joined to a channel, the Fabric chaincode lifecycle process can be used to install chaincode packages on the peer. Only peer admin identities can be used to install a package on a peer.

For high availability, you should consider deploying at least one other peer in the organization so that this peer can safely go offline for maintenance while transaction requests can continue to be addressed by the other peer. This redundant peer should be on a separate system or virtual machine in case the location where both peers are deployed goes down.

## Troubleshooting peer deployment

### Peer fails to start with ERRO 001 Fatal error when initializing core config

**Problem:** When launching the peer, it fails with:

```
InitCmd -> ERRO 001 Fatal error when initializing core config : Could not find config file. Please make sure that FABRIC_CFG_PATH is set to a path which contains core.yaml
```

**Solution:**

This error occurs when the `FABRIC_CFG_PATH` is not set or is set incorrectly. Ensure that you have set the `FABRIC_CFG_PATH` environment variable to point to the location of the peer `core.yaml` file. Navigate to the folder where the `peer.exe` binary file resides and run the following command:

```
export FABRIC_CFG_PATH=../config
```
