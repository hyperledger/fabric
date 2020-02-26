# CA Deployment steps

## Download the binaries

The Fabric CA server and CA client binaries are available for download from [github](https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.4). Scroll down to **Assets** and select the latest binary for your machine. The .zip file contains both the CA server and the CA client binaries. Copy the CA server file to a directory where you will run the server. Likewise, copy the CA client binary to its own directory. Having the CA client in its own folder facilitate certificate management especially when you need to interact with multiple CAs.  When you issue a command from the CA client against a CA server, you can target a specific CA by modifying the CA server URL on the request. Therefore, only a single Fabric CA client binary is required and can be used to transact with multiple CAs. More on using the Fabric CA client below.

## Fabric CA client

While you can use the Fabric SDKs to interact with your CA, it is recommended that you **use the Fabric CA client to administer the CA** and register and enroll identities.   The instructions provided in this topic assume a single Fabric CA client is being used.

Because you will use the CA client to submit transactions to the CA for enrolling identities, certificate management is critically important when using the CA client. The private and public key are first generated locally by the Fabric CA client, and then the public key is sent to the CA which returns an encoded "signing certificate". A best practice therefore is for each TLS CA, organization CA, or intermediate CA that you create, to create the following folders. You don't need to create them yet, the instructions later in this topic will guide you through the setup:
- You need a folder for each CA, such as `/tls-ca` or `/org1-ca` or `/int-ca`. This folder can be under the Fabric CA client or elsewhere where the CA client can access the path.
- Under each CA folder, you need a folder to store the TLS certificate, such as `org1-ca/crypto`, as well as a folder to place the generated certificates on an enroll command. If this is an admin identity, it would be  `org1-ca/admin`. This TLS certificate secures the communications between the CA client and the CA server.

You can also use environment variables to specify certificate locations.
- `FABRIC_CA_CLIENT_TLS_CERTFILES` - Specify the location and name of the TLS certificate. If the path of the environment variable FABRIC_CA_CLIENT_TLS_CERTFILES is not an absolute path, it will be parsed as relative to the client’s home directory.
- `FABRIC_CA_CLIENT_HOME` - Specify the path to where the generated certificates should be stored. This path is relative to the Fabric CA client binary.
- `FABRIC_CA_CLIENT_MSPDIR` - Specify the name of the folder under `FABRIC_CA_CLIENT_HOME` where the generated certificates are stored. If not specified, it defaults to `$FABRIC_CA_CLIENT_HOME/msp`. **Important** when multiple members are enrolled with the CA, you will need to store their certificates in distinct folders.

The first time you issue an `enroll` command from the CA client, the `fabric-ca-client-config.yaml` is generated in the `$FABRIC_CA_CLIENT_HOME` directory, if one is not already there. When you customize the values in this file, they can be used automatically by the CA client and do not have to be passed on the command line on a subsequent `enroll` command.


## Submitting transactions from the CLI

Two sets of CLI commands are included with the CA server and CA client binary files:

- Use the [Fabric CA **server** CLI commands](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/servercli.html) to deploy and update the CA server.

- Use the [Fabric CA **client** CLI commands ](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/clientcli.html) to submit requests to your CA server after it is set up, such as registering, enrolling or revoking identities.

We will use these CLI commands throughout this topic.

## Deploy the TLS CA

Regardless of whether you are setting up a TLS CA, an organization CA or an intermediate CA, the process follows the same overall steps. The differences will be in the modifications you make to the CA server configuration .yaml file. The following instructions describe the process beginning with the TLS CA.

- [Step one: Initialize the CA server](#initialize-the-tls-ca-server)
- [Step two: Modify the CA server configuration](#modify-the-tls-ca-server-configuration)
- [Step three: Delete the CA server certificates](#delete-the-tls-ca-server-certificates)
- [Step four: Start the CA server](#start-the-tls-ca-server)
- [Step five: Enroll node admins with TLS CA](#enroll-node-admins-with-tls-ca)

### Before you begin

You should have already downloaded and copied the Fabric CA server binary `fabric-ca-server` to a clean directory on your machine.

### Initialize the TLS CA server

The first step to deploy a CA server is to "initialize" it. Run the following CA server CLI command to initialize the server by specifying the admin user id and password for the CA:

```
fabric-ca-server init -b <ADMIN_USER>:<ADMIN_PWD>
```

For example:
```
fabric-ca-server init -b tls-admin:tls-adminpw
```

**This example is for illustration purposes only. Obviously, in a production environment you would never use `tls-admin` and `tls-adminpw` as the bootstrap username and password.**  Be sure that you record the admin id and password that you specify. They are required later when you issue enroll commands against the CA. It can help to use a meaningful id to differentiate which server you are transacting with and follow secure password practices.

#### What does the CA server `init` command do?

The `init` command does not actually start the server but generates the required metadata if it does not already exist for the server:
- By default the CA Home directory (referred to as `FABRIC_CA_HOME` in these instructions) is set to where the `fabric-ca-server init` command is run.
- The default default configuration file `fabric-ca-server-config.yaml` is generated which can be used as a template for your server the `FABRIC_CA_HOME` directory. We refer to this file throughout these instructions as the "configuration .yaml" file.
- The command looks for the root certificate file `ca-cert.pem`. If it does not exist in the CA Home directory, it creates it. This is the self-signed root certificate, meaning it is generated and signed by the CA itself and does not come from another source.
- The private key is generated and stored in the `FABRIC_CA_HOME` directory under `/msp/keystore`.
- A default sqlLite database is initialized for the server although you can modify the database setting in the configuration .yaml file to use the supported database of your choice. Every time the server is started, it loads the data from this database.
- The `-b` flag bootstraps the CA server administrator onto the server using the values specified in `<ADMIN_USER>` and `<ADMIN_PWD>`. The admin user is registered with all of the default admin attributes present in the configuration .yaml file `registry` section. If this CA will be used to register other users with any of those attributes, then the CA admin user needs to possess those attributes. In other words, the registrar must have the `hf.Registrar.Roles` attributes before it can register another identity with any of those attributes. Therefore, if this CA admin will be used to register the admin identity for an Intermediate CA, then this CA admin must have the `hf.IntermediateCA` set to `true` even though this may not be an intermediate CA server.

**Important**: When you modify settings in the .yaml file and restart the server, the **previously issued certificates are not replaced**. If you want the certificates to be regenerated when the server is started, you need to delete them and run the `fabric-ca-server start` command. For example, if you modify the `csr` values or `hf.` attributes after you start the server, you need to delete the previously generated certificates, and then run the `fabric-ca-server start` command. Be aware though, that when you restart the CA server using the new signing certificate and private key, all previously issued certificates will no longer be able to authenticate with the CA.

### Modify the TLS CA server configuration

Now that you have initialized your server, you can edit the generated `fabric-ca-server-config.yaml` file to modify the default configuration settings for your use case according to the [Checklist for a production CA server](ca-config.html).

At a minimum you should do the following:
- `tls.enabled` - Recall that TLS is disabled in the default configuration file. Since this is a production server, enable it by setting this value to `true`. Setting this value to `true` causes the TLS signing certificate `tls-cert.pem` file  to be generated when the server is started in the next step.
- `ca.name` - Give the CA a name by editing the  parameter.
- `csr.hosts` - Update this parameter to include this hostname and ip address where this server is running, if it is different than what is already in this file.
- `port` - Enter the port that you want to use for this server.
- `operations.listenAddress:` -  In the unlikely case that there is another node running on this host and port, then you need to update this parameter to use a different port.

### Delete the TLS CA server certificates

Before starting the server, if you modified the `csr` values or `hf.` attributes in the configuration .yaml file, you need to delete the `ca-cert.pem` file and the entire `/msp` folder. These certificates will be re-generated when you start the CA server in the next step.

### Start the TLS CA server

Run the following command to start the CA server:
```
fabric-ca-server start
```
When the server starts successfully you will see something similar to:
```
[INFO] Listening on https://0.0.0.0:7052
```
Because you have enabled TLS communications, notice that the TLS signing certificate `tls-cert.pem` file is generated under the `FABRIC_CA_HOME` location.

**Tip:** Note that the CA `ADMIN_USER` and `ADMIN_PWD` that were set on the `init` command cannot be overridden with the `-b` flag on this `start` command. When you need to modify the CA admin password, use the Fabric CA client [identity](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/clientcli.html#identity-command) command.

**Optional flags**:
- `-d` - If you want to run the server in DEBUG mode which facilitates problem diagnosis, you can include the `-d` flag on the start command. However, in general it is not recommended to run a server with debug enabled as this will cause the server to perform slower.
- `-p` - If you want the server to run on a port different than what is specified in the configuration .yaml file, you can override the existing port.

### Enroll node admins with TLS CA

Now that your TLS CA is configured and before you can deploy any other nodes for your organization, you need to first enroll the admin user of that node with the TLS CA to generate the TLS certificates. Since the CA server is up and running we now use the **Fabric CA client CLI commands** to submit an enrollment request. Before you deploy another new CA, you need to provide the CA admin TLS certificate and private key to the configuration which allows the CA to transact security with the other nodes in the network. The following process is described for a CA node, but can be easily extended for peer or ordering nodes.

The enrollment process is used to generate the certificate and private key pair which forms the node TLS identity. It is performed by using the Fabric CA client. The process begins by setting up two folders which the Fabric CA client can use to:
 - Know where the TLS certificate resides that allows the Fabric CA client to communicate with the TLS CA server.
 - Store the certificates that are issued when the Fabric CA client enroll command is run against the TLS CA server.

1. Create a folder that will be used when transacting with the **TLS CA**. This directory can be where your Fabric CA client is running or anywhere it can be accessed by the Fabric CA client.
   ```
   mkdir tls-ca
   ```
2. Under that folder, create an `/admin` subdirectory for the generated admin certificates and a `/crypto` folder to store the TLS root cert from the server.
   ```
   cd tls-ca
   mkdir admin
   mkdir crypto
   ```
3. Copy the TLS CA root certificate file `ca-cert.pem`, that was generated when the TLS CA server was initialized, to the folder `/tls-ca/crypto/ca-cert.pem`. **Important:** This TLS CA signing certificate will need to be available on each host that will run commands against the TLS CA.

4. The Fabric CA Client also needs to know where to put the certificates that are generated by the `enroll` command. The destination is controlled by the value of the `FABRIC_CA_CLIENT_HOME` environment variable.  Modify the `FABRIC_CA_CLIENT_HOME` to point to the `/admin` folder that was created under the `tls-ca` folder above:
   ```
   export FABRIC_CA_CLIENT_HOME=<RELATIVE-PATH-TO-tls-ca-FOLDER>/tls-ca/admin
   ```
   For example, if the `tls-ca` folder resides inside the Fabric CA Client directory it would simply be:
   ```
   export FABRIC_CA_CLIENT_HOME=tls-ca/admin
   ```
5. You are ready to use the Fabric CA client CLI to generate the TLS CA admin certificate and private key. You need this certificate and private key to be able to transact securely with other nodes in the organization. Run the command:
   ```
   fabric-ca-client enroll -d -u https://<ADMIN>:<ADMIN-PWD>@<CSR-HOST>:<PORT> --tls.certfiles <RELATIVE-PATH-TO-TLS-CERT>
   ```
   Replace:
   - `<ADMIN>` - with the TLS CA admin specified on the `init` command.
   - `<ADMIN-PWD>` - with the TLS CA admin specified on the `init` command.
   - `<CSR-HOST>` - with the hostname specified in the `csr` section of the TLS CA configuration .yaml file.
   - `<PORT>` - with the port that the TLS CA is listening on.
   - `<RELATIVE-PATH-TO-TLS-CERT>` - with the path to the tls-cert.pem file that you copied from your TLS CA. This is the path relative to `FABRIC_CA_CLIENT_HOME`.

   In this case, the `-d` parameter runs the client in DEBUG mode which is useful for debugging command failures.

   For example:
   ```
   fabric-ca-client enroll -d -u https://tls-admin:tls-adminpw@my-machine.example.com:7054 --tls.certfiles ../crypto/ca-cert.pem
   ```

When this command completes, an `/msp` folder is generated under `/tls-ca/admin` and contains the TLS signing cert and private key for the node.

**Important:** Make note of the `/msp/signcerts/cert.pem` and `/msp/keystore/` files. These are your TLS signcert and private key that you will need to reference in the CA configuration .yaml file in the following steps.

The following diagram is a conceptual summary of the steps you performed to create a TLS CA server:

![ca-tls-flow](./ca-tls-flow.png)

### (Optional) Register the Intermediate CA admin with the TLS CA

If you are planning to have an intermediate CA in addition to a root CA, you can also  register and enroll that intermediate CA admin user now as well. The following command registers the intermediate ca admin id `icaadmin` and `icaadminpw` with the TLS CA.

```
fabric-ca-client register -d --id.name icaadmin --id.secret icaadminpw --id.type client -u https://my-machine.example.com:7054  --tls.certfiles ../crypto/ca-cert.pem
```

We can also generate the intermediate CA TLS certificates now as well. First, we use the `FABRIC_CA_CLIENT_MSPDIR` environment variable to indicate where the generated intermediate CA TLS certificates should be stored. Then, run the enroll command for the `icaadmin` user.


```
export FABRIC_CA_CLIENT_MSPDIR=msp-ica
fabric-ca-client enroll -d -u https://icaadmin:icaadminpw@my-machine.example.com:7054 --tls.certfiles ../crypto/ca-cert.pem
```

The intermediate CA TLS signing certificate is generated under `FABRIC_CA_CLIENT_HOME=tls-ca/admin/msp-ica/signcert` and the private key is available under `FABRIC_CA_CLIENT_HOME=tls-ca/admin/msp-ica/keystore`.

When you deploy the intermediate CA you will need to refer to these two file in the intermediate CA configuration .yaml file.

## Deploy an organization CA

The deployment process overview describes the need for both an enrollment CA and a TLS CA for every organization. The TLS CA issues the TLS certificates that allow for secure transactions within the organization. The enrollment CA, also referred to as the "organization CA" or the "eCert CA" is used to issue identities for the organization. You deployed the TLS CA in the previous set of steps, now we are ready to deploy the organization CA. Later in this topic we will also create an intermediate CA; therefore, this CA serves as the "root CA" in that chain of trust.

Because you've already enrolled your organization CA admin in the previous step, you are ready to deploy the CA following the same pattern of steps that were used when you deployed the TLS CA.

### Before you start

- Copy the Fabric CA server binary `fabric-ca-server` to a new directory on your machine.
- Make sure you have copied the CA admin TLS certificate and key pair that you generated in the previous step to a location that can be accessed by this CA server. These are the `/msp/signcerts/cert.pem` and `/msp/keystore/` files that were generated by the enroll command in the /tls-ca folder.

### Initialize the CA server

Run the command to initialize the server specifying a new admin user id and password for the CA:

```
fabric-ca-server init -b <ADMIN_USER>:<ADMIN_PWD>
```

For example:
```
fabric-ca-server init -b rcaadmin:rcaadminpw
```

### Modify the CA server configuration

As we did with the TLS CA, we will now edit the generated `fabric-ca-server-config.yaml` file for the organization CA to modify the default configuration settings for your use case according to the [Checklist for a production CA server](ca-config.html).

At a minimum, you should edit the following fields:
- `tls.enabled` - Enable TLS by setting this value to `true`.
- `tls.certfile` and `tls.keystore`- Enter the path and filenames for the TLS CA signing certificate and private key that were generated when the CA admin for this node was enrolled with the TLS CA. The signing certificate, `cert.pem`, was generated using the Fabric CA client and can be found under `FABRIC_CA_CLIENT_HOME/msp/signcerts`. The private key is located under `FABRIC_CA_CLIENT_HOME/msp/keystore`.
- `ca.name` - Give the CA a name by specifying a value in this parameter, for example `rca`, for "root ca".
- `csr.hosts` - Update this parameter to include this hostname and ip address where this server is running if it is different than what is already in the file.
- `port` - Enter the port that you want to use for this server.
- `operations.listenAddress:` - If there is another CA running on this host, then you need to update this parameter to use a different port.
- `signing.maxpathlength` - If this will be a parent server for another intermediate CA, you need to set value to greater than 0 in the  configuration .yaml file of the parent or root CA. See the instructions for the [intermediate CA](ca-config.html#intermediate-ca).

### Delete the CA server certificates

Before starting the server, if you modified the `csr` values or `hf.` attributes in the configuration .yaml file, you need to delete the `ca-cert.pem` file and the entire `/msp` folder. These certificates will be re-generated based on the new settings in the configuration .yaml file when you start the CA server in the next step.

### Start the CA server

Run the following command to start the CA server:
```
fabric-ca-server start
```

### Enroll the CA admin

The final step for deploying the CA is to enroll the CA admin identity which generates the node signing certificate and private key. The key-pair is required for this admin identity to be able to enroll other identities. Again we will use the Fabric CA client CLI to enroll the admin and the process begins by setting up two new folders which the Fabric CA client can use to:
 - Know where the TLS certificate resides that allows the Fabric CA client to communicate with the TLS CA server.
 - Store the certificates that are issued when the Fabric CA client enroll command is run against the TLS CA server.

1. Create a subdirectory that will be used when the Fabric CA client transacts with the **organization CA**. This directory can be where your Fabric CA client is running or anywhere it can be accessed by the Fabric CA client.
   ```
   mkdir org1-ca
   ```
2. Under that directory, create an `/admin` folder for the generated admin certificates and a `/crypto` folder to store the TLS root cert from the server.
   ```
   cd org1-ca
   mkdir admin
   mkdir crypto
   ```
3. Copy the TLS CA root certificate file `ca-cert.pem`, that was generated when the TLS CA server was initialized, to the folder `/org1-ca/crypto/ca-cert.pem`.
4. The Fabric CA Client also needs to know where to put the certificates that are generated by the `enroll` command. The destination is controlled by the value of the `FABRIC_CA_CLIENT_HOME` environment variable.  Modify the `FABRIC_CA_CLIENT_HOME` to point to the `/admin` folder that was created under the `org1-ca` folder above:
   ```
   export FABRIC_CA_CLIENT_HOME=<RELATIVE-PATH-TO-org1-ca-FOLDER>/org1-ca/admin
   ```
   For example, if the `org1-ca` folder resides inside the Fabric CA Client directory it would simply be:
   ```
   export FABRIC_CA_CLIENT_HOME=org1-ca/admin
   ```
   If you registered and enrolled the intermediate CA admin with the TLS server, the generated certificates folder was changed. For consistency, you should set it back to the `/msp` folder. Otherwise, you do not need to run this command.
   ```
   export FABRIC_CA_CLIENT_MSPDIR=msp
   ```
5. Now you can use the Fabric CA client to generate the CA admin certificate and private key. You need this certificate and private key to be able to issue identities using this CA. Run the command:
   ```
   fabric-ca-client enroll -d -u https://<ADMIN>:<ADMIN-PWD>@<CSR-HOST>:<PORT> --tls.certfiles <RELATIVE-PATH-TO-TLS-CERT>
   ```
   Replace:
   - `<ADMIN>` - with the organization CA admin specified on the `init` command.
   - `<ADMIN-PWD>` - with the organization CA admin specified on the `init` command.
   - `<CSR-HOST>` - with the hostname specified in the `csr` section of the organization CA configuration .yaml file.
   - `<PORT>` - with the port that the organization CA is listening on.
   - `<RELATIVE-PATH-TO-TLS-CERT>` - with the path to the ca-cert.pem file that you copied from your TLS CA. This is the path relative to `FABRIC_CA_CLIENT_HOME`.

   In this case, the `-d` parameter runs the client in DEBUG mode which is useful for debugging command failures.

   For example:
   ```
   fabric-ca-client enroll -d -u https://rcaadmin:rcaadminpw@my-machine.example.com:7055 --tls.certfiles ../crypto/ca-cert.pem
   ```
When this command completes, an `/msp` folder is generated under `/org1-ca/admin/msp` and contains the signing cert and private key for the CA and looks similar to:

```
└── msp
   ├── IssuerPublicKey
   ├── IssuerRevocationPublicKey
   ├── cacerts
   │   └── my-machine-example-com-7055.pem
   ├── keystore
   │   └── 60b6a16b8b5ba3fc3113c522cce86a724d7eb92d6c3961cfd9afbd27bf11c37f_sk
   ├── signcerts
   │   └── cert.pem
   └── user
```

Where:
- `my-machine-example-com-7055.pem` is the **Organization CA root certificate**.
- `60b6a16b8b5ba3fc3113c522cce86a724d7eb92d6c3961cfd9afbd27bf11c37f_sk` is the **private key** for the organization CA admin identity. This key needs to be protected and should not be shared with anyone. It is required to be able to register and enroll other identities with this CA.
- `cert.pem` is the CA admin identity **signing certificate**.

## (Optional) Deploy an intermediate CA

Intermediate CAs form a chain a trust with the root CA and can be used to direct enrollment requests for a specific organization to a single CA as well as protect the root of trust by shutting down the root CA. When intermediate CAs are addressing all the enrollment requests, the root CA can be turned off.

**Note:** This section assumes that you have already [registered and enrolled](optional-register-the-intermediate-ca-admin-with-the-tls-ca) the `icaadmin` identity with the TLS CA.

After deploying the root CA, you can perform these steps to deploy an intermediate CA:

1. You already registered the `icaadmin` identity with the TLS CA. You need to also register the same identity with the (root) organization CA and  because this will be an admin of an intermediate CA, you need to include the `hf.IntermediateCA=true` attribute.  (Run this command from the same terminal window where you enrolled the organization CA admin in the previous step.)
   ```
     fabric-ca-client register -u https://my-machine.example.com:7055  --id.name icaadmin --id.secret icaadminpw --id.attrs '"hf.Registrar.Roles=user,peer","hf.Revoker=true","hf.IntermediateCA=true"' --tls.certfiles ../crypto/ca-cert.pem
   ```   
2. From the intermediate CA home directory, initialize the CA by running the `init` command and bootstrapping the `icaadmin` id that you already registered with the TLS CA and the root CA. For example:
   ```
   fabric-ca-server init -b icaadmin:icaadminpw
   ```
3. Modify the `fabric-ca-server-config.yaml` file.
   * `port`: Specify a unique port for this server.
   * `tls.enabled`: Must be set to `true`.
   * `tls.certfile` and `tls.keystore`: Enter the path and filename for the TLS CA signing certificate and private key. These are the same certs that were used for the root CA.
   * `ca`: Specify a name for the ca. For example `ica`.
   * `signing.profiles.ca.maxpathlength`: Set this value to 0.
   * `csr.name`: Must be blank for intermediate CAs.
   * `csr.ca.pathlength`: Set this value to 0.
   * `intermediate.parentserver.url`: Enter the value of the parent server URL, in the form `https://<ROOT-CA-ADMIN>:<ROOT-CA-ADMIN-PW>@<CSR-HOST>:<PORT>`
   * `intermediate.parentserver.caname`: Enter the value of the parentserver `caname` from the root CA server configuration .yaml file.
   * `intermediate.enrollment.hosts`: Enter the host name that the intermediate CA server is listening on.
   * `intermediate.enrollment.profile`: Enter the name of the signing profile from `signing.profile` section to use when issuing certificates. Normally this value is `ca`.
   * `intermediate.tls.certfiles`: Enter the path and file name of the copied TLS `ca-cert.pem` file.
   * `operations.listenAddress`: If another CA is running on the same host, you need to specify a unique port.
4. **Important:** You must delete the intermediate CA `ca-cert.pem` and `/msp` folders in order for them to be regenerated with the intermediate CA settings.
5. Start the intermediate CA server.
   ```
   fabric-ca-server start
   ```

   Because this is an intermediate CA server, notice that a `ca-chain.pem` file is generated. This file contains the certificate chain, and includes the intermediate ca-cert.pem as well as the root ca-cert.pem.

### Enroll the Intermediate CA admin

The final step for deploying the intermediate CA is to enroll the intermediate CA admin to generate the node signing certificate and private key which is required for the identity to be able to enroll other identities. Again we will use the Fabric CA client to enroll the admin and the process begins by setting up two folders which the Fabric CA client can use to:
- Know where the TLS certificate resides that allows the Fabric CA client to communicate with the TLS CA server.
- Store the certificates that are issued when the Fabric CA client enroll command is run against the TLS CA server.

1. Create a subdirectory where the Fabric CA client can issue the **intermediate CA** signing certificate and private key. This directory can be under your existing `/ica` folder.
   ```
   cd ica
   mkdir icaadmin
   ```
2. You can reuse the TLS CA root certificate file `ca-cert.pem`, that you previously copied to the folder `/ica/crypto/ca-cert.pem`.
3. Set the value of the `FABRIC_CA_CLIENT_HOME` environment variable to be the location of where the put the certificates that are generated by the `enroll` command.
   ```
   export FABRIC_CA_CLIENT_HOME=<RELATIVE-PATH-TO-tls-ca-FOLDER>/ica/admin
   ```
   For example, if the `org1-ca` folder resides inside the Fabric CA Client directory it would simply be:
   ```
   export FABRIC_CA_CLIENT_HOME=ica/admin
   ```
   Again, when you registered and enrolled the intermediate CA admin with the TLS server, the generated certificates folder was changed. For consistency, you should set it back to the `/msp` folder. Otherwise, you do not need to run this command.
   ```
   export FABRIC_CA_CLIENT_MSPDIR=msp
   ```
4. Now you can use the Fabric CA client to generate the CA admin certificate and private key. You need this certificate and private key to be able to issue identities using this CA. Run the command:
  ```
  fabric-ca-client enroll -d -u https://<ADMIN>:<ADMIN-PWD>@<CSR-HOST>:<PORT> --tls.certfiles <RELATIVE-PATH-TO-TLS-CERT>
  ```
  Replace:
  - `<ADMIN>` - with the organization CA admin specified on the `init` command.
  - `<ADMIN-PWD>` - with the organization CA admin specified on the `init` command.
  - `<CSR-HOST>` - with the hostname specified in the `csr` section of the intermediate CA configuration .yaml file.
  - `<PORT>` - with the port that the organization CA is listening on.
  - `<RELATIVE-PATH-TO-TLS-CERT>` - with the path to the ca-cert.pem file that you copied from your TLS CA. This is the path relative to `FABRIC_CA_CLIENT_HOME`.

  For example:
  ```
  fabric-ca-client enroll -d -u https://icaadmin:icaadminpw@my-machine.example.com:7056 --tls.certfiles ../crypto/ca-cert.pem
  ```
When this command completes, an `/msp` folder is generated under `/ica/admin` and contains the signcert and private key for the intermediate CA admin identity. Notice the `/intermediatecerts` folder is also created and populated with the intermediate CA certificate which connects this CA to the root CA. 

## Troubleshooting CA Deployment

### Fabric CA client `enroll` command fails

**Problem:** When running an enroll command with the Fabric CA client CLI, it fails with:
```
Error: Failed to read config file at '/Users/mwp/.fabric-ca-client/fabric-ca-client-config.yaml': While parsing config: yaml: line 42: mapping values are not allowed in this context
```

**Solution:**

This error occurs when the `FABRIC_CA_CLIENT_HOME` is not set. Ensure that you have set the `FABRIC_CA_CLIENT_HOME` environment variable to point to the location where the generated certificates will be stored by running the command:

```
export FABRIC_CA_CLIENT_HOME=<RELATIVE-PATH-TO-tls-ca-FOLDER>/ica/icaadmin
```

### Intermediate CA server fails to start

**Problem:**: The intermediate CA server fails to start with the error:
```
Error: Response from server: Error Code: 0 - Certificate signing failure: {"code":5300,"message":"Policy violation request"}
```
You may also see the associated error on the root CA:
```
[ERROR] local signer certificate disallows CA MaxPathLen extending
[INFO] 9.27.117.220:49864 POST /enroll 500 0 "Certificate signing failure: {"code":5300,"message":"Policy violation request"}"

```
**Solution:**
The values of the `signing.profiles.ca.maxpathlength` and the `csr.ca.pathlength` fields in the intermediate CA configuration .yaml file need to be set to 0.

### Enrolling the intermediate CA admin user fails

**Problem:** When you enroll the intermediate CA admin it fails with the error:
```
Error: Response from server: Error Code: 0 - Chain file does not exist at /Users/mbk/ca-ica/ca-chain.pem
```

**Solution:**

You need to delete the intermediate CA, `ca-cert.pem` file and the `/msp` folder before you start the intermediate CA server.
