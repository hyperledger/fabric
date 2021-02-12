# Deploy the ordering service

Before deploying an ordering service, review the material in [Planning for an ordering service](./ordererplan.html) and [Checklist for a production ordering service](./ordererchecklist.html), which discusses all of the relevant decisions you need to make and parameters you need to configure before deploying an ordering service.

This tutorial is based on the Raft consensus protocol and can be used to build an ordering service, which is comprised of ordering nodes, or "orderers". It describes the process to create a three-node Raft ordering service where all of the ordering nodes belong to the same organization. This tutorial assumes that a system channel genesis block will not be used when bootstrapping the orderer. Instead, these nodes (or a subset of them), will be joined to a channel using the process to [Create a channel](../create_channel/create_channel_participation.html).

For information on how to create an orderer that will be bootstrapped with a system channel genesis block, check out [Deploy the ordering service](https://hyperledger-fabric.readthedocs.io/en/release-2.2/deployorderer/ordererdeploy.html) from the Fabric v2.2 documentation.

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
- `General.BoostrapMethod:none` - This allows the orderer to start without needing a system channel configuration block.
- `General.LocalMSPDir` - Path to the ordering node MSP folder.
- `General.LocalMSPID` - MSP ID of the ordering organization as specified in the channel configuration.
- `FileLedger.Location` - Location on the file system to the ledgers of the channels this orderer will be servicing.
- `ChannelParticipation.Enabled` - Set to `true`. This allows the orderer to be joined to an application channel without joining a system channel first.

Because this tutorial assumes that a system channel genesis block will not be used when bootstrapping the orderer, the following additional parameters are required if you want to create an application channel with the `osnadmin` command.

- `Admin.ListenAddress` - The orderer admin server address (host and port) that can be used by the `osnadmin` command to configure channels on the ordering service. This value should be a unique `host:port` combination to avoid conflicts.
- `Admin.TLS.Enabled:` - Technically this can be set to `false`, but this is not recommended. In general, you should always set this value to `true`.
- `Admin.TLS.PrivateKey:` - The path to and file name of the orderer private key issued by the TLS CA.
- `Admin.TLS.Certificate:` - The path to and file name of the orderer signed certificate issued by the TLS CA.
- `Admin.TLS.ClientAuthRequired:` This value must be set to `true`. Note that while mutual TLS is required for all operations on the orderer Admin endpoint, the entire network is not required to use Mutual TLS.
- `Admin.TLS.ClientRootCAs:` - The path to and file name of the admin client TLS CA Root certificate. In the folder structure above, this is `admin-client/client-tls-ca-cert.pem`.

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
INFO 01d Registrar initializing without a system channel, number of application channels: 0, with 0 consensus.Chain(s) and 0 follower.Chain(s)
INFO 01f Starting orderer:
```

You have successfully started one node, you now need to repeat these steps to configure and start the other two orderers.

## Next steps

Your ordering service is started and ready to order transactions into blocks. Depending on your use case, you may need to add or remove orderers from the consenter set, or other organizations may want to contribute their own orderers to the cluster. See the topic on ordering service [reconfiguration](../raft_configuration.html#reconfiguration) for considerations and instructions.

Once your nodes have been created, you are ready to join them to a channel. Check out [Create a channel](../create_channel/create_channel_participation.html) for more information.

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
PANI 27c Failed creating a block puller: client certificate isn't in PEM format:  channel=mychannel node=3
```

**Solution:**  

Your `orderer.yaml` file is missing the `General.Cluster.ClientCertificate` and `General.Cluster.ClientPrivateKey` definitions. Provide the path to and filename of the public certificate (also known as a signed certificate) and private key generated by your TLS CA for the orderer in these two fields and restart the node.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/) -->
