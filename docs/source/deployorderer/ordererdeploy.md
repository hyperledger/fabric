# Deploy the ordering service

Before deploying an ordering service, review the material in [Planning for an ordering service](./ordererplan.html) and [Checklist for a production ordering service](./ordererchecklist.html), which discusses all of the relevant decisions you need to make and parameters you need to configure before deploying an ordering service.

This tutorial is based on the Raft consensus protocol and can be used to build an ordering service, which is comprised of ordering nodes, or "orderers". It describes the process to create a three-node Raft ordering service where all of the ordering nodes belong to the same organization.

## Download the ordering service binary and configuration files

To get started, download the Fabric binaries from [GitHub](https://github.com/hyperledger/fabric/releases) to a folder on your local system, for example `fabric/`. In GitHub, scroll to the Fabric release you want to download, click the **Assets** twistie, and select the binary for your system type. After you extract the `.zip` file, you will find all of the Fabric binaries in the `/bin` folder and the associated configuration files in the `/config` folder.
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
  │   └── osnadmin
  └── config
    ├── configtx.yaml
    ├── orderer.yaml
    └── core.yaml
```

Along with the relevant binary file, the orderer configuration file, `orderer.yaml` is required to launch an orderer on the network. The other files are not required for the orderer deployment but are useful when you attempt to create or edit channels, among other tasks.

**Tip:** Add the location of the orderer binary to your `PATH` environment variable so that it can be picked up without fully qualifying the path to the binary executable, for example:

```
export PATH=<path to download location>/bin:$PATH
```

After you have mastered deploying and running an ordering service by using the orderer binary and `orderer.yaml` configuration file, it is likely that you will want to use an orderer container in a Kubernetes or Docker deployment. The Hyperledger Fabric project publishes an [orderer image](https://hub.docker.com/r/hyperledger/fabric-orderer) that can be used for development and test, and various vendors provide supported orderer images. For now though, the purpose of this topic is to teach you how to properly use the orderer binary so you can take that knowledge and apply it to the production environment of your choice.

## Prerequisites

Before you can launch an orderer in a production network, you need to make sure you've created and organized the necessary certificates, generate the genesis block, decided on storage, and configured `orderer.yaml`.

### Certificates

While **cryptogen** is a convenient utility that can be used to generate certificates for a test environment, it should **never** be used on a production network. The core requirement for certificates for Fabric nodes is that they are Elliptic Curve (EC) certificates. You can use any tool you prefer to issue these certificates (for example, OpenSSL). However, the Fabric CA streamlines the process because it generates the Membership Service Providers (MSPs) for you.

Before you can deploy the orderer, create the recommended folder structure for the orderer or orderer certificates that is described in the [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topic to store the generated certificates and MSPs.

This folder structure isn't mandatory, but these instructions presume you have created it:

```
├── organizations
  └── ordererOrganizations
      └── ordererOrg1.example.com
          ├── msp
            ├── cacerts
            └── tlscacerts
          ├── orderers
              └── orderer0.ordererOrg1.example.com
                  ├── msp
                  └── tls
```

You should have already used your certificate authority of choice to generate the orderer enrollment certificate, TLS certificate, private keys, and the MSPs that Fabric must consume. Refer to the [CA deployment guide](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/cadeploy.html) and [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html) topics for instructions on how to create a Fabric CA and how to generate these certificates. You need to generate the following sets of certificates:
  - **Orderer organization MSP**
  - **Orderer TLS CA certificates**
  - **Orderer local MSP (enrollment certificate and private key of the orderer)**

You will either need to use the Fabric CA client to generate the certificates directly into the recommended folder structure or you will need to copy the generated certificates to their recommended folders after they are generated. Whichever method you choose, most users are ultimately likely to script this process so it can be repeated as needed. A list of the certificates and their locations is provided here for your convenience.

If you are using a containerized solution for running your network (which for obvious reasons is a popular choice), **it is a best practice to mount volumes for the certificate directories external to the container where the node itself is running. This will allow the certificates to be used by an ordering node container, regardless whether the ordering node container goes down, becomes corrupted, or is restarted.**

#### TLS certificates

For the ordering node to launch successfully, the locations of the TLS certificates you specified in the [Checklist for a production ordering node](./ordererchecklist.html) must point to the correct certificates. To do this:

- Copy the **TLS CA Root certificate**, which by default is called `ca-cert.pem`, to the orderer organization MSP definition `organizations/ordererOrganizations/ordererOrg1.example.com/msp/tlscacerts/tls-cert.pem`.
- Copy the **CA Root certificate**, which by default is called `ca-cert.pem`, to the orderer organization MSP definition `organizations/ordererOrganizations/ordererOrg1.example.com/msp/cacerts/ca-cert.pem`.
- When you enroll the orderer identity with the TLS CA, the public key is generated in the `signcerts` folder, and the private key is located in the `keystore` directory. Rename the private key in the `keystore` folder to `orderer0-tls-key.pem` so that it can be easily recognized later as the TLS private key for this node.
- Copy the orderer TLS certificate and private key files to `organizations/ordererOrganizations/ordererOrg1.example.com/orderers/orderer0.ordererOrg1.example.com/tls`. The path and name of the certificate and private key files correspond to the values of the `General.TLS.Certificate` and `General.TLS.PrivateKey` parameters in the `orderer.yaml`.

**Note:** Don't forget to create the `config.yaml` file and add it to the organization MSP and local MSP folder for each ordering node. This file enables Node OU support for the MSP, an important feature that allows the MSP's admin to be identified based on an "admin" OU in an identity's certificate. Learn more in the [Fabric CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#nodeous) documentation.

If you are using a containerized solution for running your network (which for obvious reasons is a popular choice), **it is a best practice to mount volumes for the certificate directories external to the container where the node itself is running. This will allow the certificates to be used by an ordering node container, regardless whether the ordering node container goes down, becomes corrupted, or is restarted.**

#### Orderer local MSP (enrollment certificate and private key)

Similarly, you need to point to the [local MSP of your orderer](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#create-the-local-msp-of-a-node) by copying the MSP folder to `organizations/ordererOrganizations/ordererOrg1.example.com/orderers/orderer0.ordererOrg1.example.com/msp`. This path corresponds to the value of the `General.LocalMSPDir` parameter in the `orderer.yaml` file. Because of the Fabric concept of ["Node Organization Unit (OU)"](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#nodeous), you do not need to specify an admin of the orderer when bootstrapping. Rather, the role of "admin" is conferred onto an identity by setting an OU value of "admin" inside a certificate and enabled by the `config.yaml` file. When Node OUs are enabled, any admin identity from this organization will be able to administer the orderer.

Note that the local MSP contains the signed certificate (public key) and the private key for the orderer. The private key is used by the node to sign transactions, and is therefore not shared and must be secured. For maximum security, a Hardware Security Module (HSM) can be configured to generate and store this private key.

### Create the ordering service genesis block

The first channel that is created in a Fabric network is the "system" channel. The system channel defines the set of ordering nodes that form the ordering service and the set of organizations that serve as ordering service administrators. Peers transact on private "application" channels that are derived from the ordering service system channel, which also defines the "consortium" (the peer organizations known to the ordering service). Therefore, before you can deploy an ordering service, you need to generate the system channel configuration by creating the system channel "genesis block" using a tool called `configtxgen`. We'll then use the generated system channel genesis block to bootstrap each ordering node.

#### Set up the `configtxgen` tool

While it is possible to build the channel creation transaction file manually, it is easier to use the [configtxgen](../commands/configtxgen.html) tool, which works by reading a `configtx.yaml` file that defines the configuration of your channel and then writing the relevant information into a configuration block known as the "genesis block".

Notice that the `configtxgen` tool is located in the `bin` folder of downloaded Fabric binaries.

Before using `configtxgen`, confirm you have set the `FABRIC_CFG_PATH` environment variable to the path of the directory that contains your local copy of the `configtx.yaml` file. You can verify that are able to use the tool by printing the `configtxgen` help text:

```
configtxgen --help
```

#### The `configtx.yaml` file

The `configtx.yaml` file is used to specify the **channel configuration** of the system channel and application channels. The information that is required to build the channel configuration is specified in a readable and editable form in the `configtx.yaml` file. The `configtxgen` tool uses the channel profiles that are defined in `configtx.yaml` to create the channel configuration block in the [protobuf format](https://developers.google.com/protocol-buffers).

The `configtx.yaml` file is located in the `config` folder alongside the images that you downloaded and contains the following configuration sections that we need to create our new channel:

- **Organizations:** The organizations that can become members of your channel. Each organization has a reference to the cryptographic material that is used to build the [channel MSP](../membership/membership.html).
- **Orderer:** Which ordering nodes will form the Raft consenter set of the channel.
- **Policies** Different sections of the file work together to define the channel policies that will govern how organizations interact with the channel and which organizations need to approve channel updates. For the purposes of this tutorial, we will use the defaults that are used by Fabric. For more information about policies, check out [Policies](../policies/policies.html).
- **Profiles** Each channel profile references information from other sections of the `configtx.yaml` file to build a channel configuration. The profiles are used to create the genesis block of the channel.

The `configtxgen` tool uses `configtx.yaml` file to create the genesis block for the channel. A detailed version of the `configtx.yaml` file is available in the [Fabric sample configuration](https://github.com/hyperledger/fabric/blob/{BRANCH}/sampleconfig/configtx.yaml). Refer to the [Using configtx.yaml to build a channel configuration](../create_channel/create_channel_config.html) tutorial to learn more about the  settings in this file.

#### Generate the system channel genesis block

The first channel that is created in a Fabric network is the system channel. The system channel defines the set of ordering nodes that form the ordering service and the set of organizations that serve as ordering service administrators. The system channel also includes the organizations that are members of blockchain [consortium](../glossary.html#consortium). The consortium is a set of peer organizations that belong to the system channel, but are not administrators of the ordering service. Consortium members have the ability to create new channels and include other consortium organizations as channel members.

The genesis block of the system channel is required to deploy a new ordering service. A good example of a system channel profile can be found in the [test network configtx.yaml](https://github.com/hyperledger/fabric-samples/blob/master/test-network/configtx/configtx.yaml#L319) which includes the `TwoOrgsOrdererGenesis` profile as shown below:

```yaml
TwoOrgsOrdererGenesis:
        <<: *ChannelDefaults
        Orderer:
            <<: *OrdererDefaults
            Organizations:
                - *OrdererOrg
            Capabilities:
                <<: *OrdererCapabilities
        Consortiums:
            SampleConsortium:
                Organizations:
                    - *Org1
                    - *Org2
```

The `Orderer:` section of the profile defines the Raft ordering service, with the `OrdererOrg` as the ordering service administrator. The `Consortiums` section of the profile creates a consortium of peer organizations named `SampleConsortium:`.  For a production deployment, it is recommended that the peer and ordering nodes belong to separate organizations. This example uses peer organizations `Org1` and `Org2`. You will want to customize this section by providing your own consortium name and replacing `Org1` and `Org2` with the names of your peer organizations. If they are unknown at this time, you do not have to list any organizations under `Consortiums.SampleConsortium.Organizations`. Adding the peer organizations now saves the effort of a channel configuration update later. If you do add them, don't forget to define the peer organizations in the `Organizations:` section at the top of the `configtx.yaml` file as well. Notice this profile is missing an `Application:` section. You will need to create the application channels after you deploy the ordering service.

After you have completed editing the `configtx.yaml` to reflect the orderer and peer organizations that will participate in your network, run the following command to create the genesis block of the system channel:
```
configtxgen -profile TwoOrgsOrdererGenesis -channelID system-channel -outputBlock ./system-genesis-block/genesis.block
```

Where:
- `-profile` refers to the `TwoOrgsOrdererGenesis` profile in `configtx.yaml`.
- `-channelID` is the name of the system channel. In this tutorial, the system channel is named `system-channel`.
- `-outputBlock` refers to the location of the generated genesis block.

When the command is successful, you will see logs of `configtxgen` loading the `configtx.yaml` file and printing a channel creation transaction:
```
INFO 001 Loading configuration
INFO 002 Loaded configuration: /Usrs/fabric-samples/test-network/configtx/configtx.yaml
INFO 003 Generating new channel configtx
INFO 004 Generating genesis block
INFO 005 Creating system channel genesis block
INFO 006 Writing genesis block
```

Make note of the generated output block filename. This is your genesis block and will be referenced in the `orderer.yaml` file below.

### Storage

You must provision persistent storage for your ledgers. The default location for the ledger is located at `/var/hyperledger/production/orderer`. Ensure that your orderer has write access to the folder. If you choose to use a different location, provide that path in the `FileLedger:` parameter in the `orderer.yaml` file. If you decide to use Kubernetes or Docker, recall that in a containerized environment, local storage disappears when the container goes away, so you will need to provision or mount persistent storage for the ledger before you deploy an orderer.

### Configuration of `orderer.yaml`

Now you can use the [Checklist for a production orderer](./ordererchecklist.html) to modify the default settings in the `orderer.yaml` file. In the future, if you decide to deploy the orderer through Kubernetes or Docker, you can override the same default settings by using environment variables instead. Check out the [note](../deployment_guide_overview.html#step-five-deploy-orderers-and-ordering-nodes) in the deployment guide overview for instructions on how to construct the environment variable names for an override.

At a minimum, you need to configure the following parameters:
- `General.ListenAddress` - Hostname that the ordering node listens on.
- `General.ListenPort` - Port that the ordering node listens on.
- `General.TLS.Enabled: true` - Server-side TLS should be enabled in all production networks.
- `General.TLS.PrivateKey ` - Ordering node private key from TLS CA.
- `General.TLS.Certificate ` - Ordering node signed certificate (public key) from the TLS CA.
- `General.TLS.RootCAS` - This value should be unset.
- `General.BoostrapMethod:file` - Start ordering service with a system channel.
- `General.BootstrapFile` - Path to and name of the genesis block file for the ordering service system channel.
- `General.LocalMSPDir` - Path to the ordering node MSP folder.
- `General.LocalMSPID` - MSP ID of the ordering organization as specified in the channel configuration.
- `FileLedger.Location` - Location of the orderer ledger on the file system.

## Start the orderer

Make sure you have set the value of the `FABRIC_CFG_PATH` to be the location of the `orderer.yaml` file relative to where you are invoking the orderer binary. For example, if you run the orderer binary from the `fabric/bin` folder, it would point to the  `/config` folder:
    ```
    export FABRIC_CFG_PATH=../config
    ```

After `orderer.yaml` has been configured and your deployment backend is ready, you can simply start the orderer node with the following command:

```
cd bin
./orderer start
```

When the orderer starts successfully, you should see a message similar to:

```
INFO 019 Starting orderer:
INFO 01a Beginning to serve requests
```

You have successfully started one node, you now need to repeat these steps to configure and start the other two orderers. When a majority of orderers are started, a Raft leader is elected. You should see something similar to the following output:
```
INFO 01b Applied config change to add node 1, current nodes in channel: [1] channel=syschannel node=1
INFO 01c Applied config change to add node 2, current nodes in channel: [1 2] channel=syschannel node=1
INFO 01d Applied config change to add node 3, current nodes in channel: [1 2 3] channel=syschannel node=1
INFO 01e raft.node: 1 elected leader 2 at term 11 channel=syschannel node=1
INFO 01f Raft leader changed: 0 -> 2 channel=syschannel node=1
```

## Next steps

Your ordering service is started and ready to order transactions into blocks. Depending on your use case, you may need to add or remove orderers from the consenter set, or other organizations may want to contribute their own orderers to the cluster. See the topic on ordering service [reconfiguration](../raft_configuration.html#reconfiguration) for considerations and instructions.

Finally, your system channel includes a consortium of peer organizations as defined in the `Organization` section of the channel configuration. This list of peer organizations are allowed to create channels on your ordering service. You need to use the `configtxgen` command and the `configtx.yaml` file to create an application channel. Refer to the [Creating a channel](../create_channel/create_channel.html#creating-an-application-channel) tutorial for more details.

## Troubleshooting

### When you start the orderer, it fails with the following error:
```
ERRO 001 failed to parse config:  Error reading configuration: Unsupported Config Type ""
```

**Solution:**  

Your `FABRIC_CFG_PATH` is not set. Run the following command to set it to the location of your `orderer.yaml` file.

```
export FABRIC_CFG_PATH=<PATH_TO_ORDERER_YAML>
```

### When you start the orderer, it fails with the following error:
```
PANI 003 Failed to setup local msp with config: administrators must be declared when no admin ou classification is set
```

**Solution:**   

Your local MSP definition is missing the `config.yaml` file. Create the file and copy it into the local MSP `/msp` folder of orderer. See the [Fabric CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#nodeous) documentation for more instructions.


### When you start the orderer, it fails with the following error:
```
PANI 005 Failed validating bootstrap block: initializing channelconfig failed: could not create channel Orderer sub-group config: setting up the MSP manager failed: administrators must be declared when no admin ou classification is set
```

**Solution:**   

The system channel configuration is missing `config.yaml` file. If you are creating a new ordering service, the `MSPDir` referenced in `configtx.yaml` file is missing the `config.yaml` file. Follow instructions in the [Fabric CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#nodeous) documentation to generate this file and then rerun `configtxgen` to regenerate the genesis block for the system channel.
```
# MSPDir is the filesystem path which contains the MSP configuration.
        MSPDir: ../config/organizations/ordererOrganizations/ordererOrg1.example.com/msp
```
Before you restart the orderer, delete the existing channel ledger files that are stored in the `FileLedger.Location` setting of the `orderer.yaml` file.


### When you start the orderer, it fails with the following error:
```
PANI 004 Failed validating bootstrap block: the block isn't a system channel block because it lacks ConsortiumsConfig
```
**Solution:**  

Your channel configuration is missing the consortium definition. If you are starting a new ordering service, edit the `configtx.yaml` file `Profiles:` section and add the consortium definition:
```
Consortiums:
            SampleConsortium:
                Organizations:
```
The `Consortiums:` section is required but can be empty, as shown above, if the peer organizations are not yet known. Rerun `configtxgen` to regenerate the genesis block for the system channel and then before you start the orderer, delete the existing channel ledger files that are stored in the `FileLedger.Location` setting of the `orderer.yaml` file.

### When you start the orderer, it fails with the following error:
```
PANI 27c Failed creating a block puller: client certificate isn't in PEM format:  channel=mychannel node=3
```

**Solution:**  

Your `orderer.yaml` file is missing the `General.Cluster.ClientCertificate` and `General.Cluster.ClientPrivateKey` definitions. Provide the path to and filename of the public certificate (also known as a signed certificate) and private key generated by your TLS CA for the orderer in these two fields and restart the node.

### When you start the orderer, it fails with the following error:
```
ServerHandshake -> ERRO 025 TLS handshake failed with error remote error: tls: bad certificate server=Orderer remoteaddress=192.168.1.134:52413
```

**Solution:**  

This error can occur when the `tlscacerts` folder is missing from the orderer organization MSP folder specified in the channel configuration. Create the `tlscacerts` folder inside your MSP definition and insert the root certificate from your TLS CA (`ca-cert.pem`). Rerun `configtxgen` to regenerate the genesis block for the system channel so that the channel configuration includes this certificate. Before you start the orderer again, delete the existing channel ledger files that are stored in the `FileLedger.Location` setting of the `orderer.yaml` file.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/) -->
