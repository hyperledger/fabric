# Deploy the peer

Before deploying a peer, make sure to digest the material in [Planning for a peer](./peerplan.html) and [Checklist for a production peer](./peerchecklist.html) which discusses all of the relevant decisions you need to make and parameters you need to configure before deploying a peer.

Note: in order for a peer to be a joined to a channel, the organization the peer belongs to must be joined to the channel. This means that you must have [created the MSP of your organization](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#create-the-org-msp-needed-to-add-an-org-to-a-channel). The MSP ID of this organization must be the same as the ID specified at `peer.localMspId` in `core.yaml`.

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

**Tip:** Add the location of the peer binary to your `PATH` environment variable so that it can be picked up without fully qualifying the path to the binary executable, for example:

```
export PATH=<path to download location>/bin:$PATH
```

After you have mastered deploying and running a peer by using the peer binary and core.yaml configuration file, it is likely that you will want to use a peer container in a Kubernetes or Docker deployment. The Hyperledger Fabric project publishes a [peer image](https://hub.docker.com/r/hyperledger/fabric-peer) that can be used for development and test, and various vendors provide supported peer images. For now though, the purpose of this topic is to teach you how to properly use the peer binary so you can take that knowledge and apply it to the production environment of your choice.

## Prerequisites

Before you can launch a peer node in a production network, you need to make sure you've created and organized the necessary certificates, decided on storage, and configured `core.yaml`.

### Certificates

Note: while **cryptogen** is a convenient utility that can be used to generate certificates for a test network, it should **never** be used on a production network. The core requirement for certificates for Fabric nodes is that they are Elliptic Curve (EC) certificates. You can use any tool you prefer to issue these certificates (for example, OpenSSL). However, the Fabric CA streamlines the process because it generates the Membership Service Providers (MSPs) for you.

Before you can deploy the peer, create the recommended folder structure for the peer certificates that is described in the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic to store the generated certificates and MSPs.

This folder structure isn't mandatory, but these instructions presume you have created it:
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

You should have already used your certificate authority of choice to generate the peer enrollment certificate, TLS certificate, private keys, and the MSPs that Fabric must consume. Refer to the [CA deployment](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/cadeploy.html) and [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topics for instructions on how to create a Fabric CA and how to generate these certificates. You need to generate the following sets of certificates:
  - **Peer TLS CA certificates**
  - **Peer local MSP (enrollment certificate and private key of the peer)**

You will either need to use the Fabric CA client to generate the certificates directly into the recommended folder structure or you will need to copy the generated certificates to their recommended folders after they are generated. Whichever method you choose, most users are ultimately likely to script this process so it can be repeatable as needed. A list of the certificates and their locations is provided here for your convenience.

#### TLS certificates

In order for the peer to launch successfully, make sure that the locations of the TLS certificates you specified in the [Checklist for a production peer](./peerchecklist.html) point to the correct certificates. To do this:

- Copy the TLS certificate that contains the public key that is associated with the signing (private) key certificate, which by default is called `ca-cert.pem`, to `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/tls-cert.pem`. The path and name of the certificate corresponds to the `peer.tls.rootcert.file` parameter in `core.yaml`.
- After you have generated the peer TLS certificate, the certificate will have been generated in the `signcerts` directory, and the private key will have been generated in the `keystore` directory. Rename the generated private key in the `keystore` folder to `peer0-key.pem` so that it can more easily be recognized later on as being a private key.
- Copy the peer TLS certificate and private key to `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls`. The path and name of the certificate and private key files correspond to the `peer.tls.cert.file` and `peer.tls.key.file` parameters in the `core.yaml`.
- If using mutual authentication (`clientAuthRequired` set to true), you need to indicate to the peer which TLS CA root certificates to use to authorize clients. Copy the organization's TLS CA root certificate `ca-cert.pem` to `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca-cert.pem` so that the organization's clients will be authorized. The path and name of the certificate corresponds to `peer.tls.clientRootCAs.files` parameter in `core.yaml`. Note that multiple files can be configured, one for each client organization that will communicate with the peer (for example if other organizations will use this peer for endorsements). If `clientAuthRequired` is set to false, you can skip this step.

#### Peer local MSP (enrollment certificate and private key)

Similarly, you need to point to the [local MSP of your node](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#create-the-local-msp-of-a-node) by copying it to `organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/msp`. This path corresponds to the value of the `peer.mspConfigPath` parameter in the `core.yaml` file. Because of the Fabric concept of ["Node Organization Unit (OU)"](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#nodeous), you do not need to specify an admin of the peer when bootstrapping. Rather, the role of "admin" is conferred onto an identity by setting an OU value of "admin" inside a certificate and enabled by the `config.yaml` file. When Node OUs are enabled, any organization admin will be able to administer the peer.

Note that the local MSP contains the signed certificate (public key) and the private key for the peer. The private key is used by the node to sign transactions, and is therefore not shared and must be secured. For maximum security, a Hardware Security Module (HSM) can be configured to generate and store this private key.

### Storage

You must provision persistent storage for your ledger files. The following properties in `core.yaml` dictates where ledger files and snapshots are written:
* `peer.fileSystemPath` - defaults to `/var/hyperledger/production`
* `ledger.snapshots.rootDir` - defaults to `/var/hyperledger/production/snapshots`

Ensure that your peer has write access to these directories.

If you decide to use Kubernetes or Docker, recall that in a containerized environment local storage disappears when the container goes away, so you will need to provision or mount persistent storage for the ledger before you deploy a peer.

### Configuration of `core.yaml`

Now you can use the [Checklist for a production peer](./peerchecklist.html) to modify the default settings in the `core.yaml` file. In the future, if you decide to deploy the peer through Kubernetes or Docker, you can override the same default settings by using environment variables instead. Check out the [note](../deployment_guide_overview.html#step-five-deploy-peers-and-ordering-nodes) in the deployment guide overview for instructions on how to construct the environment variable names for an override.

Make sure to set the value of the `FABRIC_CFG_PATH` to be the location of the `core.yaml` file. When you run the peer binary from the `fabric/bin` folder, it would point to the  `/config` folder:
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

In order to be able to transact on a network, the peer must be joined to a channel. The organization the peer belongs to must be a member of a channel before one of its peers can be joined to it. Note that if an organization wants to create a channel, it must be a member of the consortium hosted by the ordering service. If your organization has not specified at least one [anchor peer](../glossary.html#anchor-peer) on the channel, you should do so, as it will enabled communication between organizations. See the [Create a channel tutorial](../create_channel/create_channel_overview.html) to learn more. Once a peer is joined to a channel, the Fabric chaincode lifecycle process can be used to install chaincode packages on the peer. Only peer admin identities can be used to install a package on a peer.

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
