## Fabric CA User's Guide

Fabric CA is a Certificate Authority for Hyperledger Fabric.

It provides features such as:  
  1) registration of identities, or connects to LDAP as the user registry;  
  2) issuance of Enrollment Certificates (ECerts);  
  3) issuance of Transaction Certificates (TCerts), providing both anonymity
    and unlinkability when transacting on a Hyperledger Fabric blockchain;  
  4) certificate renewal and revocation.

Fabric CA consists of both a server and a client component as described
later in this document.

For developers interested in contributing to Fabric CA, see the
[Fabric CA repository](https://github.com/hyperledger/fabric-ca) for more
information.

## Getting Started

### Prerequisites

* Go 1.7+ installation or later
* **GOPATH** environment variable is set correctly

### Install

To install the fabric-ca command:

```
# go get github.com/hyperledger/fabric-ca
```

### The Fabric CA CLI

The following shows the fabric-ca CLI usage:

```
# fabric-ca
fabric-ca client       - client related commands
fabric-ca server       - server related commands
fabric-ca cfssl        - all cfssl commands

For help, type "fabric-ca client", "fabric-ca server", or "fabric-ca cfssl".
```

The `fabric-ca server` and `fabric-ca client` commands are discussed below.

If you would like to enable debug-level logging (for server or client),
set the `FABRIC_CA_DEBUG` environment variable to `true`.

Since fabric-ca is built on top of [CFSSL](https://github.com/cloudflare/cfssl),
the `fabric-ca cfssl` commands are available but are not discussed in this document.
See [CFSSL](https://github.com/cloudflare/cfssl) for more information.

## Fabric CA Server

This section describes the fabric-ca server.

You must initialize the Fabric CA server before starting it.

The fabric-ca server's home directory is determined as follows:  
- if the `FABRIC_CA_HOME` environment variable is set, use its value;  
- otherwise, if the `HOME` environment variable is set, use `$HOME/fabric-ca`;  
- otherwise, use `/var/hyperledger/fabric/dev/fabric-ca'.

For the remainder of this server section, we assume that you have set the
`FABRIC_CA_HOME` environment variable to `$HOME/fabric-ca/server`.

#### Initializing the server

Initialize the Fabric CA server as follows:

```
# fabric-ca server init CSR-JSON-FILE
```

The following is a sample `CSR-JSON-FILE` which you can customize as desired.
The "CSR" stands for "Certificate Signing Request".

If you are going to connect to the fabric-ca server remotely over TLS,
replace "localhost" in the CSR-JSON-FILE below with the hostname where
you will be running your fabric-ca server.

```
{
  "CN": "localhost",
  "key": { "algo": "ecdsa", "size": 256 },
  "names": [
      {
        "O": "Hyperledger Fabric",
        "OU": "Fabric CA",
        "L": "Raleigh",
        "ST": "North Carolina",
        "C": "US"
      }
  ]
}
```

All of the fields above pertain to the X.509 certificate which is generated
by the `fabric server init` command as follows:  

<a name="csr-fields"/>
###### CSR fields

- **CN** is the Common Name  
- **keys** specifies the algorithm and key size as described below  
- **O** is the organization name   
- **OU** is the organization unit  
- **L** is the location or city  
- **ST** is the state  
- **C** is the country

The `fabric-ca server init` command generates a self-signed X.509 certificate.
It stores the certificate in the `server-cert.pem` file and the key in the
`server-key.pem` file in the Fabric CA server's home directory.

###### Algorithms and key sizes

The CSR-JSON-FILE can be customized to generate X.509 certificates and keys
that support both RSA and Elliptic Curve (ECDSA). The following setting is
an example of the implementation of Elliptic Curve Digital Signature
Algorithm (ECDSA) with curve `prime256v1` and signature algorithm
`ecdsa-with-SHA256`:
```
"key": {
   "algo": "ecdsa"
   "size": 256
}
```
The choice of algorithm and key size are based on security needs.

Elliptic Curve (ECDSA) offers the following key size options:

| size        | ASN1 OID           | Signature Algorithm  |
|-------------|:-------------:|:-----:|
| 256      | prime256v1 | ecdsa-with-SHA256 |
| 384      | secp384r1      |   ecdsa-with-SHA384 |
| 521 | secp521r1     | ecdsa-with-SHA512 |

RSA offers the following key size options:

| size        | Modulus (bits)| Signature Algorithm  |
|-------------|:-------------:|:-----:|
| 2048      | 2048 | sha256WithRSAEncryption |
| 4096      | 4096 | sha512WithRSAEncryption |

#### Starting the server

Create a file named `server-config.json` as shown below
in your fabric-ca server's home directory (e.g. *$HOME/fabric-ca/server*).

```
{
 "tls_disable": false,
 "ca_cert": "server-cert.pem",
 "ca_key": "server-key.pem",
 "driver":"sqlite3",
 "data_source":"fabric-ca.db",
 "user_registry": { "max_enrollments": 0 },
 "tls": {
     "tls_cert": "server-cert.pem",
     "tls_key": "server-key.pem"
 },
 "users": {
    "admin": {
      "pass": "adminpw",
      "type": "client",
      "group": "bank_a",
      "attrs": [
         {"name":"hf.Registrar.Roles","value":"client,peer,validator,auditor"},
         {"name":"hf.Registrar.DelegateRoles", "value": "client"}
      ]
    }
 },
 "groups": {
   "banks_and_institutions": {
     "banks": ["bank_a", "bank_b", "bank_c"],
     "institutions": ["institution_a"]
   }
 },
 "signing": {
    "default": {
       "usages": ["cert sign"],
       "expiry": "8000h",
       "ca_constraint": {"is_ca": true}
    }
 }
}
```

Now you may start the Fabric CA server as follows:

```
# cd $FABRIC_CA_HOME
# fabric-ca server start -address '0.0.0.0' -config server-config.json
```

To cause the fabric-ca server to listen on `http` rather than `https`,
set `tls_disable` to `true` in the `server-config.json` file.

To limit the number of times that the same secret (or password) can
be used for enrollment, set the `max_enrollments` in the `server-config.json`
file to the appropriate value.  If you set the value to 1, the fabric-ca
server allows passwords to only be used once for a particular enrollment ID.
If you set the value to 0, the fabric-ca server places no limit on the number
of times that a secret can be reused for enrollment.
The default value is 0.

The fabric-ca server should now be listening on port 7054.

You may skip to the [Fabric CA Client](#fabric-ca-client) section
if you do not want to configure the fabric-ca server to run in a
cluster or to use LDAP.

#### Server database configuration

This section describes how to configure the fabric-ca server to connect
to Postgres or MySQL databases.  The default database is SQLite and the
default database file is `fabric-ca.db` in the Fabric CA's home directory.

If you don't care about running the fabric-ca server in a cluster, you may
skip this section; otherwise, you must configure either Postgres or MySQL
as described below.

###### Postgres

The following sample may be added to the `server-config.json` file in order
to connect to a Postgres database.  Be sure to customize the various values
appropriately.

```
"driver":"postgres",
"data_source":"host=localhost port=5432 user=Username password=Password dbname=fabric-ca sslmode=verify-full",
```

Specifying *sslmode* enables SSL, and a value of *verify-full* means to verify
that the certificate presented by the postgres server was signed by a trusted CA
and that the postgres server's host name matches the one in the certificate.

We also need to set the TLS configuration in the fabric-ca server-config file.
If the database server requires client authentication, then a client cert and
key file needs to be provided. The following should be present in the fabric-ca
server config:

```
"tls":{
  ...
  "db_client":{
    "ca_certfiles":["CA.pem"],
    "client":[{"keyfile":"client-key.pem","certfile":"client-cert.pem"}]
  }
},
```

**ca_certfiles** - The names of the trusted root certificate files.

**certfile** - Client certificate file.

**keyfile** - Client key file.

###### MySQL

The following sample may be added to the `server-config.json` file in order
to connect to a MySQL database.  Be sure to customize the various values
appropriately.

```
...
"driver":"mysql",
"data_source":"root:rootpw@tcp(localhost:3306)/fabric-ca?parseTime=true&tls=custom",
...
```

If connecting over TLS to the MySQL server, the `tls.db_client` section is
also required as described in the **Postgres** section above.

#### LDAP

The fabric-ca server can be configured to read from an LDAP server.

In particular, the fabric-ca server may connect to an LDAP server to do the following:

   * authenticate a user prior to enrollment, and  
   * retrieve a user's attribute values which are used for authorization.

In order to configure the fabric-ca server to connect to an LDAP server, add a section
of the following form to your fabric-ca server's configuration file:

```
{
   "ldap": {
       "url": "scheme://adminDN:pass@host[:port][/base]"
       "userfilter": "filter"
   }
```

where:
   * `scheme` is one of *ldap* or *ldaps*;
   * `adminDN` is the distinquished name of the admin user;
   * `pass` is the password of the admin user;  
   * `host` is the hostname or IP address of the LDAP server;
   * `port` is the optional port number, where default 389 for *ldap* and 636 for *ldaps*;
   * `base` is the optional root of the LDAP tree to use for searches;
   * `filter` is a filter to use when searching to convert a login user name to
   a distinquished name.  For example, a value of `(uid=%s)` searches for LDAP
   entries with the value of a `uid` attribute whose value is the login user name.
   Similarly, `(email=%s)` may be used to login with an email address.

The following is a sample configuration section for the default settings for the
 OpenLDAP server whose docker image is at `https://github.com/osixia/docker-openldap`.

```
 "ldap": {
    "url": "ldap://cn=admin,dc=example,dc=org:admin@localhost:10389/dc=example,dc=org",
    "userfilter": "(uid=%s)"
 },
```

See `FABRIC_CA/testdata/testconfig-ldap.json` for the complete configuration file with this section.
Also see `FABRIC_CA/scripts/run-ldap-tests` for a script which starts an OpenLDAP docker image, configures it,
runs the LDAP tests in FABRIC_CA/cli/server/ldap/ldap_test.go, and stops the OpenLDAP server.

###### When LDAP is configured, enrollment works as follows:

  * A fabric-ca client or client SDK sends an enrollment request with a basic authorization header.
  * The fabric-ca server receives the enrollment request, decodes the user/pass in the authorization header, looks up the DN (Distinquished Name) associated with the user using the "userfilter" from the configuration file, and then attempts an LDAP bind with the user's password. If successful, the enrollment processing is authorized and can proceed.

###### When LDAP is configured, attribute retrieval works as follows:

   * A client SDK sends a request for a batch of tcerts **with one or more attributes** to the fabric-ca server.
   * The fabric-ca server receives the tcert request and does as follows:
       * extracts the enrollment ID from the token in the authorization header
       (after validating the token);
       * does an LDAP search/query to the LDAP server, requesting all of the
       attribute names received in the tcert request;
       * the attribute values are placed in the tcert as normal

#### Setting up a cluster of fabric-ca servers

You may use any IP sprayer to load balance to a cluster of fabric-ca
servers.  This section provides an example of how to set up Haproxy
to route to a fabric-ca server cluster. Be sure to change
hostname and port to reflect the settings of your fabric-ca servers.

haproxy.conf

```
global
      maxconn 4096
      daemon

defaults
      mode http
      maxconn 2000
      timeout connect 5000
      timeout client 50000
      timeout server 50000

listen http-in
      bind *:7054
      balance roundrobin
      server server1 hostname1:port
      server server2 hostname2:port
      server server3 hostname3:port
```

<a name="fabric-ca-client"/>
## Fabric CA Client

This section describes how to use the fabric-ca client.

The default fabric-ca client's home directory is `$HOME/fabric-ca`, but
this can be changed by setting the `FABRIC_CA_HOME` environment variable.

You must create a file named **client-config.json** in the fabric-ca
client's home directory.
The following is a sample client-config.json file:

```
{
  "ca_certfiles":["server-cert.pem"],
  "signing": {
    "default": {
       "usages": ["cert sign"],
       "expiry": "8000h"
    }
 }
}
```

You must also copy the server's certificate into the client's home directory.
In the examples in this document, the server's certificate is at
`$HOME/fabric-ca/server/server-cert.pem`.  The file name must
match the name in the *client-config.json* file.

<a name="EnrollBootstrap"/>
#### Enroll the bootstrap user

Unless the fabric-ca server is configured to use LDAP, it must
be configured with at least one pre-registered bootstrap user.
In the previous server-config.json in this document, that user
has an enrollment ID of *admin* with an enrollment secret of *adminpw*.

<a name="csr-admin"/>
First, create a CSR (Certificate Signing Request) JSON file similar to
the following.  Customize it as desired.

```
{
  "key": { "algo": "ecdsa", "size": 256 },
  "names": [
      {
        "O": "Hyperledger Fabric",
        "OU": "Fabric CA",
        "L": "Raleigh",
        "ST": "North Carolina",
        "C": "US"
      }
  ]
}
```

See [CSR fields](#csr-fields) for a description of the fields in this file.
When enrolling, the CN (Common Name) field is automatically set to the enrollment ID
which is *admin* in this example, so it can be omitted from the csr.json file.

The following command enrolls the admin user and stores an enrollment certificate (ECert)
in the fabric-ca client's home directory.

```
# export FABRIC_CA_HOME=$HOME/fabric-ca/clients/admin
# fabric-ca client enroll -config client-config.json admin adminpw http://localhost:7054 csr.json
```

You should see a message similar to `[INFO] enrollment information was successfully stored in`
which indicates where the certificate and key files were stored.

The enrollment certificate is stored at `$FABRIC_CA_ENROLLMENT_DIR/cert.pem` by default, but a different path can be specified by setting the `FABRIC_CA_CERT_FILE` environment variable.

The enrollment key is stored at `$FABRIC_CA_ENROLLMENT_DIR/key.pem` by default, but a different
path can be specified by setting the `FABRIC_CA_KEY_FILE` environment variable.

If `FABRIC_CA_ENROLLMENT_DIR` is not set, the value of the `FABRIC_CA_HOME`
environment variable is used in its place.

#### Register a new identity

The user performing the register request must be currently enrolled, and
must also have the proper authority to register the type of user being
registered.

In particular, the invoker's identity must have been registered with the attribute
"hf.Registrar.Roles". This attribute specifies the types of identities
that the registrar is allowed to register.

For example, the attributes for a registrar might be as follows, indicating
that this registrar identity can register peer, application, and user identities.

```
"attrs": [{"name":"hf.Registrar.Roles", "value":"peer,app,user"}]
```

To register a new identity, you must first create a JSON file similar to the one below
which defines information for the identity being registered.
This is a sample of registration information for a peer.

```
{
  "id": "peer1",
  "type": "peer",
  "group": "bank_a",
  "attrs": [{"name":"SomeAttrName","value":"SomeAttrValue"}]
}
```

The **id** field is the enrollment ID of the identity.

The **type** field is the type of the identity: orderer, peer, app, or user.

The **group** field must be a valid group name as found in the *server-config.json*
file.

The **attrs** field is optional and is not required for a peer, but is shown
here as example of how you associate attributes with any identity.

Assuming you store the information above in a file named **register.json**,
the following command uses the **admin** user's credentials to
register the **peer1** identity.

```
# export FABRIC_CA_HOME=$HOME/fabric-ca/clients/admin
# cd $FABRIC_CA_HOME
# fabric-ca client register -config client-config.json register.json http://localhost:7054
```

The output of a successful *fabric-ca client register* command is a
password similar to `One time password: gHIexUckKpHz`.  Make a note of
your password to use in the following section to enroll a peer.

#### Enrolling a peer identity

Now that you have successfully registered a peer identity,
you may now enroll the peer given the enrollment ID and secret
(i.e. the *password* from the previous section).

First, create a CSR (Certificate Signing Request) JSON file similar to
the one described in the [Enrolling the bootstrap user](#EnrollBootstrap) section.
Name the file *csr.json* for the following example.

This is similar to enrolling the bootstrap user except that
we also demonstrate how to use environment variables to place
the key and certificate files in a specific location.
The following example shows how to place them into a Hyperledger Fabric
MSP (Membership Service Provider) directory structure.
The *MSP_DIR* environment variable refers to the root
directory of MSP in Hyperledger Fabric and the $MSP_DIR/signcerts
and $MSP_DIR/keystore directories must exist.

Also note that you must replace *\<secret>* with the secret which was
returned from the registration in the previous section.

```
# export FABRIC_CA_CERT_FILE=$MSP_DIR/signcerts/peer.pem
# export FABRIC_CA_KEY_FILE=$MSP_DIR/keystore/key.pem
# fabric-ca client enroll -config client-config.json peer1 <secret> https://localhost:7054 csr.json
```

The peer.pem and key.pem files should now exist at the locations specified
by the environment variables.

#### Revoke a certificate or user

In order to revoke a certificate or user, the calling identity must have the
`hf.Revoker` attribute.

You may revoke a specific certificate by specifying its
AKI (Authority Key Identifier) and its serial number, as shown below.

```
fabric-ca client revoke -config client-config.json -aki xxx -serial yyy -reason "you're bad" https://localhost:7054
```

The following command disables a user's identity and also
revokes all of the certificates associated with the identity.  All
future requests received by the fabric-ca server from this identity
will be rejected.

```
fabric-ca client revoke -config client-config.json https://localhost:7054 ENROLLMENT-ID -reason "you're really bad"
```

#### Enabling TLS for a fabric-ca client

This section describes in more detail how to configure TLS for a fabric-ca client.

The following sections may be configured in the `client-config.json`.

```
{
"ca_certfiles":["CA_root_cert.pem"],
"client":[{"keyfile":"client-key.pem","certfile":"client-cert.pem"}]
}
```

The **ca_certfiles** option is the set of root certificates trusted by the client.
This will typically just be the root fabric-ca server's certificate found in
the server's home directory in the **server-cert.pem** file.

The **client** option is required only if mutual TLS is configured on the server.

## Appendix

### Postgres SSL Configuration

**Basic instructions for configuring SSL on Postgres server:**
1. In postgresql.conf, uncomment SSL and set to "on" (SSL=on)
2. Place Certificate and Key files Postgress data directory.

Instructions for generating self-signed certificates for:
https://www.postgresql.org/docs/9.1/static/ssl-tcp.html

Note: Self-signed certificates are for testing purposes and should not be used
in a production environment

**Postgres Server - Require Client Certificates**
1. Place certificates of the certificate authorities (CAs) you trust in the file
 root.crt in the Postgres data directory
2. In postgresql.conf, set "ssl_ca_file" to point to the root cert of client (CA cert)
3. Set the clientcert parameter to 1 on the appropriate hostssl line(s) in pg_hba.conf.

For more details on configuring SSL on the Postgres server, please refer to the
following Postgres documentation: https://www.postgresql.org/docs/9.4/static/libpq-ssl.html


### MySQL SSL Configuration
**Basic instructions for configuring SSL on MySQL server:**

1. Open or create my.cnf file for the server. Add or un-comment the lines below
in [mysqld] section. These should point to the key and certificates for the
server, and the root CA cert.

  Instruction on creating server and client side certs:
http://dev.mysql.com/doc/refman/5.7/en/creating-ssl-files-using-openssl.html

  [mysqld]
  ssl-ca=ca-cert.pem
  ssl-cert=server-cert.pem
  ssl-key=server-key.pem

  Can run the following query to confirm SSL has been enabled.

  mysql> SHOW GLOBAL VARIABLES LIKE 'have_%ssl';

  Should see:

  ```
  +---------------+-------+
  | Variable_name | Value |
  +---------------+-------+
  | have_openssl  | YES   |
  | have_ssl      | YES   |
  +---------------+-------+
  ```

2. After the server-side SSL configuration is finished, the next step is to
create a user who has a privilege to access the MySQL server over SSL. For that,
log in to the MySQL server, and type:

  mysql> GRANT ALL PRIVILEGES ON *.* TO 'ssluser'@'%' IDENTIFIED BY 'password' REQUIRE SSL;
  mysql> FLUSH PRIVILEGES;

  If you want to give a specific ip address from which the user will access the
  server change the '%' to the specific ip address.

  **MySQL Server - Require Client Certificates**
  Options for secure connections are similar to those used on the server side.

  - ssl-ca identifies the Certificate Authority (CA) certificate. This option,
   if used, must specify the same certificate used by the server.
  - ssl-cert identifies the client public key certificate.
  - ssl-key identifies the client private key.

  Suppose that you want to connect using an account that has no special encryption
  requirements or was created using a GRANT statement that includes the REQUIRE SSL
  option. As a recommended set of secure-connection options, start the MySQL
  server with at least --ssl-cert and --ssl-key, and invoke the fabric-ca server with
  **ca_certfiles** option set in the fabric-ca server file.

  To require that a client certificate also be specified, create the account using
  the REQUIRE X509 option. Then the client must also specify the proper client key
  and certificate files or the MySQL server will reject the connection. CA cert,
  client cert, and client key are all required for the fabric-ca server.
