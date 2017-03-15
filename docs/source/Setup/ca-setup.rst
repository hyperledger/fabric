Fabric CA User's Guide
======================

Fabric CA is a Certificate Authority for Hyperledger Fabric.

| It provides features such as:
| 1) registration of identities, or connects to LDAP as the user
  registry;
| 2) issuance of Enrollment Certificates (ECerts);
| 3) issuance of Transaction Certificates (TCerts), providing both
  anonymity and unlinkability when transacting on a Hyperledger Fabric
  blockchain;
| 4) certificate renewal and revocation.

Fabric CA consists of both a server and a client component as described
later in this document.

For developers interested in contributing to Fabric CA, see the `Fabric
CA repository <https://github.com/hyperledger/fabric-ca>`__ for more
information.


.. _Back to Top:

Table of Contents
-----------------

1. `Overview`_
2. `Getting Started`_

   1. `Prerequisites`_
   2. `Install`_
   3. `Explore the Fabric CA CLI`_

3. `File Formats`_

   1. `Fabric CA server's configuration file format`_
   2. `Fabric CA client's configuration file format`_

4. `Configuration Settings Precedence`_

5. `Fabric CA Server`_

   1. `Initializing the server`_
   2. `Starting the server`_
   3. `Configuring the database`_
   4. `Configuring LDAP`_
   5. `Setting up a cluster`_

6. `Fabric CA Client`_

   1. `Enrolling the bootstrap user`_
   2. `Registering a new identity`_
   3. `Enrolling a peer identity`_
   4. `Reenrolling an identity`_
   5. `Revoking a certificate or identity`_
   6. `Enabling TLS`_

7. `Appendix`_

Overview
--------

The diagram below illustrates how the Fabric CA server fits into the
overall Hyperledger Fabric architecture.

.. image:: ../images/fabric-ca.png

There are two ways of interacting with a Fabric CA server:
via the Fabric CA client or through one of the Fabric SDKs.
All communication to the Fabric CA server is via REST APIs.
See `fabric-ca/swagger/swagger-fabric-ca.json` for the swagger documentation
for these REST APIs.

The Fabric CA client or SDK may connect to a server in a cluster of Fabric CA
servers.   This is illustrated in the top right section of the diagram.
The client routes to an HA Proxy endpoint which load balances traffic to one
of the fabric-ca-server cluster members.
All Fabric CA servers in a cluster share the same database for
keeping track of users and certificates.  If LDAP is configured, the user
information is kept in LDAP rather than the database.

Getting Started
---------------

Prerequisites
~~~~~~~~~~~~~~~

-  Go 1.7+ installation or later
-  **GOPATH** environment variable is set correctly
- libtool and libtdhl-dev packages are installed

The following installs the libtool dependencies.

::

   # sudo apt install libtool libltdl-dev

For more information on libtool, see https://www.gnu.org/software/libtool.

For more information on libtdhr-dev, see https://www.gnu.org/software/libtool/manual/html_node/Using-libltdl.html.

Install
~~~~~~~

The following installs both the `fabric-ca-server` and `fabric-ca-client` commands.

::

    # go get -u github.com/hyperledger/fabric-ca/cmd/...

Start Server Natively
~~~~~~~~~~~~~~~~~~~~~

The following starts the `fabric-ca-server` with default settings.

::

    # fabric-ca-server start -b admin:adminpw

The `-b` option provides the enrollment ID and secret for a bootstrap
administrator.  A default configuration file named `fabric-ca-server-config.yaml`
is created in the local directory which can be customized.

Start Server via Docker
~~~~~~~~~~~~~~~~~~~~~~~

The hyperledger/fabric-ca docker image is not currently being published, but
you can build and start the server via docker-compose as shown below.

::

    # cd $GOPATH/src/github.com/hyperledger/fabric-ca
    # make docker
    # cd docker/server
    # docker-compose up -d

The hyperledger/fabric-ca docker image contains both the fabric-ca-server and
the fabric-ca-client.

Explore the Fabric CA CLI
~~~~~~~~~~~~~~~~~~~~~~~~~~~

The following shows the Fabric CA server usage message:

::

    Hyperledger Fabric Certificate Authority Server

    Usage:
      fabric-ca-server [command]

    Available Commands:
      init        Initialize the fabric-ca server
      start       Start the fabric-ca server

    Flags:
          --address string                  Listening address of fabric-ca-server (default "0.0.0.0")
      -b, --boot string                     The user:pass for bootstrap admin which is required to build default config file
          --ca.certfile string              PEM-encoded CA certificate file (default "ca-cert.pem")
          --ca.keyfile string               PEM-encoded CA key file (default "ca-key.pem")
      -c, --config string                   Configuration file (default "fabric-ca-server-config.yaml")
          --csr.cn string                   The common name field of the certificate signing request to a parent fabric-ca-server
          --csr.serialnumber string         The serial number in a certificate signing request to a parent fabric-ca-server
          --db.datasource string            Data source which is database specific (default "fabric-ca-server.db")
          --db.tls.certfiles string         PEM-encoded comma separated list of trusted certificate files (e.g. root1.pem, root2.pem)
          --db.tls.client.certfile string   PEM-encoded certificate file when mutual authenticate is enabled
          --db.tls.client.keyfile string    PEM-encoded key file when mutual authentication is enabled
          --db.tls.enabled                  Enable TLS for client connection
          --db.type string                  Type of database; one of: sqlite3, postgres, mysql (default "sqlite3")
      -d, --debug                           Enable debug level logging
          --ldap.enabled                    Enable the LDAP client for authentication and attributes
          --ldap.groupfilter string         The LDAP group filter for a single affiliation group (default "(memberUid=%s)")
          --ldap.url string                 LDAP client URL of form ldap://adminDN:adminPassword@host[:port]/base
          --ldap.userfilter string          The LDAP user filter to use when searching for users (default "(uid=%s)")
      -p, --port int                        Listening port of fabric-ca-server (default 7054)
          --registry.maxenrollments int     Maximum number of enrollments; valid if LDAP not enabled
          --tls.certfile string             PEM-encoded TLS certificate file for server's listening port (default "ca-cert.pem")
          --tls.enabled                     Enable TLS on the listening port
          --tls.keyfile string              PEM-encoded TLS key for server's listening port (default "ca-key.pem")
      -u, --url string                      URL of the parent fabric-ca-server

    Use "fabric-ca-server [command] --help" for more information about a command.

The following shows the Fabric CA client usage message:

::

    # fabric-ca-client
    Hyperledger Fabric Certificate Authority Client

    Usage:
      fabric-ca-client [command]

    Available Commands:
      enroll      Enroll user
      reenroll    Reenroll user
      register    Register user
      revoke      Revoke user

    Flags:
      -c, --config string                Configuration file (default "/Users/saadkarim/.fabric-ca-client/fabric-ca-client-config.yaml")
          --csr.cn string                The common name field of the certificate signing request to a parent fabric-ca-server
          --csr.serialnumber string      The serial number in a certificate signing request to a parent fabric-ca-server
      -d, --debug                        Enable debug logging
          --enrollment.hosts string      Comma-separated host list
          --enrollment.label string      Label to use in HSM operations
          --enrollment.profile string    Name of the signing profile to use in issuing the certificate
          --id.affiliation string        Name associated with the identity
          --id.attr string               Attributes associated with this identity (e.g. hf.revoker=true)
          --id.maxenrollments int        MaxEnrollments is the maximum number of times the secret can be reused to enroll.
          --id.name string               Unique name of the identity
          --id.secret string             Secret is an optional password. If not specified, a random secret is generated.
          --id.type string               Type of identity being registered (e.g. 'peer, app, user')
      -m, --myhost string                Hostname to include in the certificate signing request during enrollment (default "saads-mbp.raleigh.ibm.com")
          --tls.certfiles string         PEM-encoded comma separated list of trusted certificate files (e.g. root1.pem, root2.pem)
          --tls.client.certfile string   PEM-encoded certificate file when mutual authenticate is enabled
          --tls.client.keyfile string    PEM-encoded key file when mutual authentication is enabled
          --tls.enabled                  Enable TLS for client connection
      -u, --url string                   URL of fabric-ca-server (default "http://localhost:7054")

    Use "fabric-ca-client [command] --help" for more information about a command.

`Back to Top`_

File Formats
------------

Fabric CA server's configuration file format
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If no configuration file is provided to the server or no file exists,
the server will generate a default configuration file like the one
below.  The location of the default configuration will depend on whether the
``-c`` or ``--config`` option was used or not. If the config option was used
and the file did not exist it will be created in the specified
location. However, if no config option was used, it will be create in
the server home directory (see `Fabric CA Server <#server>`__ section
more info).

::

    # Server's listening port (default: 7054)
    port: 7054

    # Enables debug logging (default: false)
    debug: false

    #############################################################################
    #  TLS section for the server's listening port
    #############################################################################
    tls:
      # Enable TLS (default: false)
      enabled: false
      # TLS for the server's listening port (default: false)
      certfile: ca-cert.pem
      keyfile: ca-key.pem

    #############################################################################
    #  The CA section contains the key and certificate files used when
    #  issuing enrollment certificates (ECerts) and transaction
    #  certificates (TCerts).
    #############################################################################
    ca:
      # Certificate file (default: ca-cert.pem)
      certfile: ca-cert.pem
      # Key file (default: ca-key.pem)
      keyfile: ca-key.pem

    #############################################################################
    #  The registry section controls how the fabric-ca-server does two things:
    #  1) authenticates enrollment requests which contain a username and password
    #     (also known as an enrollment ID and secret).
    #  2) once authenticated, retrieves the identity's attribute names and
    #     values which the fabric-ca-server optionally puts into TCerts
    #     which it issues for transacting on the Hyperledger Fabric blockchain.
    #     These attributes are useful for making access control decisions in
    #     chaincode.
    #  There are two main configuration options:
    #  1) The fabric-ca-server is the registry
    #  2) An LDAP server is the registry, in which case the fabric-ca-server
    #     calls the LDAP server to perform these tasks.
    #############################################################################
    registry:
      # Maximum number of times a password/secret can be reused for enrollment
      # (default: 0, which means there is no limit)
      maxEnrollments: 0

      # Contains user information which is used when LDAP is disabled
      identities:
         - name: <<<ADMIN>>>
           pass: <<<ADMINPW>>>
           type: client
           affiliation: ""
           attrs:
              hf.Registrar.Roles: "client,user,peer,validator,auditor,ca"
              hf.Registrar.DelegateRoles: "client,user,validator,auditor"
              hf.Revoker: true
              hf.IntermediateCA: true

    #############################################################################
    #  Database section
    #  Supported types are: "sqlite3", "postgres", and "mysql".
    #  The datasource value depends on the type.
    #  If the type is "sqlite3", the datasource value is a file name to use
    #  as the database store.  Since "sqlite3" is an embedded database, it
    #  may not be used if you want to run the fabric-ca-server in a cluster.
    #  To run the fabric-ca-server in a cluster, you must choose "postgres"
    #  or "mysql".
    #############################################################################
    database:
      type: sqlite3
      datasource: fabric-ca-server.db
      tls:
          enabled: false
          certfiles: db-server-cert.pem
          client:
            certfile: db-client-cert.pem
            keyfile: db-client-key.pem

    #############################################################################
    #  LDAP section
    #  If LDAP is enabled, the fabric-ca-server calls LDAP to:
    #  1) authenticate enrollment ID and secret (i.e. username and password)
    #     for enrollment requests;
    #  2) To retrieve identity attributes
    #############################################################################
    ldap:
       # Enables or disables the LDAP client (default: false)
       enabled: false
       # The URL of the LDAP server
       url: ldap://<adminDN>:<adminPassword>@<host>:<port>/<base>
       tls:
          certfiles: ldap-server-cert.pem
          client:
             certfile: ldap-client-cert.pem
             keyfile: ldap-client-key.pem

    #############################################################################
    #  Affiliation section
    #############################################################################
    affiliations:
       org1:
          - department1
          - department2
       org2:
          - department1

    #############################################################################
    #  Signing section
    #############################################################################
    signing:
        profiles:
          ca:
             usage:
               - cert sign
             expiry: 8000h
             caconstraint:
               isca: true
        default:
          usage:
            - cert sign
          expiry: 8000h

    ###########################################################################
    #  Certificate Signing Request section for generating the CA certificate
    ###########################################################################
    csr:
       cn: fabric-ca-server
       names:
          - C: US
            ST: "North Carolina"
            L:
            O: Hyperledger
            OU: Fabric
       hosts:
         - <<<MYHOST>>>
       ca:
          pathlen:
          pathlenzero:
          expiry:

    #############################################################################
    #  Crypto section configures the crypto primitives used for all
    #############################################################################
    crypto:
      software:
         hash_family: SHA2
         security_level: 256
         ephemeral: false
         key_store_dir: keys

Fabric CA client's configuration file format
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If no configuration file is provided to the client, it will generate a
default configuration file like the one below.  The location of the default
configuration file will depend on whether or not the ``-c`` or ``--config`` option
was used. If the config option was used and the file did not exist, it
will be created in the specified location. However, if no config option
was used, it will be created in the in the Fabric CA client home
directory (see `Fabric CA Client <#client>`__ section for more info)

::

    #############################################################################
    # Client Configuration
    #############################################################################

    # URL of the fabric-ca-server (default: http://localhost:7054)
    URL: http://localhost:7054

    #############################################################################
    #    TLS section for the client's listenting port
    #############################################################################
    tls:
      # Enable TLS (default: false)
      enabled: false

      # TLS for the client's listenting port (default: false)
      certfiles:   # Comma Separated (e.g. root.pem, root2.pem)
      client:
        certfile:
        keyfile:

    #############################################################################
    #  Certificate Signing Request section for generating the CSR for
    #  an enrollment certificate (ECert)
    #############################################################################
    csr:
      cn: <<<ENROLLMENT_ID>>>
      names:
        - C: US
          ST: "North Carolina"
          L:
          O: Hyperledger
          OU: Fabric
      hosts:
       - <<<MYHOST>>>
      ca:
        pathlen:
        pathlenzero:
        expiry:

    #############################################################################
    #  Registration section used to register a new user with fabric-ca server
    #############################################################################
    id:
      name:
      type:
      affiliation:
      attributes:
        - name:
          value:

    #############################################################################
    #  Enrollment section used to enroll a user with fabric-ca server
    #############################################################################
    enrollment:
      hosts:
      profile:
      label:

`Back to Top`_

Configuration Settings Precedence
---------------------------------

The Fabric CA provides 3 way to configure settings on the fabric-ca-server
and fabric-ca-client. The precedence order is defined below:

1. CLI flags
2. Environment variables
3. Configuration file

In the remainder of this document, we refer to making changes to
configuration files. However, configuration file changes can be
overridden through environment variables or CLI flags.

For example, if we have the following in the client configuration file:

::

    tls:
      # Enable TLS (default: false)
      enabled: false

      # TLS for the client's listenting port (default: false)
      certfiles:   # Comma Separated (e.g. root.pem, root2.pem)
      client:
        certfile: cert.pem
        keyfile:

The following environment variable may be used to override the ``cert.pem``
setting in the configuration file:

``export FABRIC_CA_CLIENT_TLS_CLIENT_CERTFILE=cert2.pem``

If we wanted to override both the environment variable and configuration
file, we can use a command line flag.

``fabric-ca-client enroll --tls.client.certfile cert3.pem``

The same approach applies to fabric-ca-server, except instead of using
``FABIRC_CA_CLIENT`` as the prefix to environment variables,
``FABRIC_CA_SERVER`` is used.

Fabric CA Server
----------------

This section describes the Fabric CA server.

You may initialize the Fabric CA server before starting it if you prefer.
This provides an opportunity for you to generate a default configuration
file but to review and customize its settings before starting it.

| The fabric-ca-server's home directory is determined as follows:
| - if the ``FABRIC_CA_SERVER_HOME`` environment variable is set, use
  its value;
| - otherwise, if ``FABRIC_CA_HOME`` environment variable is set, use
  its value;
| - otherwise, if the ``CA_CFG_PATH`` environment variable is set, use
  its value;
| - otherwise, use current working directory.

For the remainder of this server section, we assume that you have set
the ``FABRIC_CA_HOME`` environment variable to
``$HOME/fabric-ca/server``.

The instructions below assume that the server configuration file exists
in the server's home directory.

.. _initialize:

Initializing the server
~~~~~~~~~~~~~~~~~~~~~~~

Initialize the Fabric CA server as follows:

::

    # fabric-ca-server init -b admin:adminpw

The ``-b`` (bootstrap user) option is required for initialization. At
least one bootstrap user is required to start the fabric-ca-server. The
server configuration file contains a Certificate Signing Request (CSR)
section that can be configured. The following is a sample CSR.

If you are going to connect to the fabric-ca-server remotely over TLS,
replace "localhost" in the CSR section below with the hostname where you
will be running your fabric-ca-server.

::

    cn: localhost
    key:
        algo: ecdsa
        size: 256
    names:
      - C: US
        ST: "North Carolina"
        L:
        O: Hyperledger
        OU: Fabric

All of the fields above pertain to the X.509 signing key and certificate which
is generated by the ``fabric-ca-server init``.  This corresponds to the
``ca.certfile`` and ``ca.keyfile`` files in the server's configuration file.
The fields are as follows:

-  **cn** is the Common Name
-  **key** specifies the algorithm and key size as described below
-  **O** is the organization name
-  **OU** is the organizational unit
-  **L** is the location or city
-  **ST** is the state
-  **C** is the country

If custom values for the CSR are required, you may customize the configuration
file, delete the files specified by the ``ca.certfile`` and ``ca-keyfile``
configuration items, and then run the ``fabric-ca-server init -b admin:adminpw``
command again.

The ``fabric-ca-server init`` command generates a self-signed CA certificate
unless the ``-u <parent-fabric-ca-server-URL>`` option is specified.
If the ``-u`` is specified, the server's CA certificate is signed by the
parent fabric-ca-server.  The ``fabric-ca-server init`` command also
generates a default configuration file named **fabric-ca-server-config.yaml**
in the server's home directory.

Algorithms and key sizes

The CSR can be customized to generate X.509 certificates and keys that
support both RSA and Elliptic Curve (ECDSA). The following setting is an
example of the implementation of Elliptic Curve Digital Signature
Algorithm (ECDSA) with curve ``prime256v1`` and signature algorithm
``ecdsa-with-SHA256``:

::

    key:
       algo: ecdsa
       size: 256

The choice of algorithm and key size are based on security needs.

Elliptic Curve (ECDSA) offers the following key size options:

+--------+--------------+-----------------------+
| size   | ASN1 OID     | Signature Algorithm   |
+========+==============+=======================+
| 256    | prime256v1   | ecdsa-with-SHA256     |
+--------+--------------+-----------------------+
| 384    | secp384r1    | ecdsa-with-SHA384     |
+--------+--------------+-----------------------+
| 521    | secp521r1    | ecdsa-with-SHA512     |
+--------+--------------+-----------------------+

RSA offers the following key size options:

+--------+------------------+---------------------------+
| size   | Modulus (bits)   | Signature Algorithm       |
+========+==================+===========================+
| 2048   | 2048             | sha256WithRSAEncryption   |
+--------+------------------+---------------------------+
| 4096   | 4096             | sha512WithRSAEncryption   |
+--------+------------------+---------------------------+

Starting the server
~~~~~~~~~~~~~~~~~~~

Start the Fabric CA server as follows:

::

    # fabric-ca-server start -b <admin>:<adminpw>

If the server has not been previously initialized, it will initialize
itself as it starts for the first time.  During this initialization, the
server will generate the ca-cert.pem and ca-key.pem files if they don't
yet exist and will also create a default configuration file if it does
not exist.  See the `Initialize the Fabric CA server <#initialize>`__ section.

Unless the fabric-ca-server is configured to use LDAP, it must be
configured with at least one pre-registered bootstrap user to enable you
to register and enroll other identities. The ``-b`` option specifies the
name and password for a bootstrap user.

A different configuration file may be specified with the ``-c`` option
as shown below.

::

    # fabric-ca-server start -c <path-to-config-file> -b <admin>:<adminpw>

To cause the fabric-ca-server to listen on ``http`` rather than
``https``, set ``tls.enabled`` to ``true``.

To limit the number of times that the same secret (or password) can be
used for enrollment, set the ``registry.maxEnrollments`` in the configuration
file to the appropriate value. If you set the value to 1, the fabric-ca
server allows passwords to only be used once for a particular enrollment
ID. If you set the value to 0, the fabric-ca-server places no limit on
the number of times that a secret can be reused for enrollment. The
default value is 0.

The fabric-ca-server should now be listening on port 7054.

You may skip to the `Fabric CA Client <#fabric-ca-client>`__ section if
you do not want to configure the fabric-ca-server to run in a cluster or
to use LDAP.

Configuring the database
~~~~~~~~~~~~~~~~~~~~~~~~

This section describes how to configure the fabric-ca-server to connect
to Postgres or MySQL databases. The default database is SQLite and the
default database file is ``fabric-ca-server.db`` in the Fabric CA
server's home directory.

If you don't care about running the fabric-ca-server in a cluster, you
may skip this section; otherwise, you must configure either Postgres or
MySQL as described below.

Postgres
^^^^^^^^^^

The following sample may be added to the server's configuration file in
order to connect to a Postgres database. Be sure to customize the
various values appropriately.

::

    db:
      type: postgres
      datasource: host=localhost port=5432 user=Username password=Password dbname=fabric-ca-server sslmode=verify-full

Specifying *sslmode* configures the type of SSL authentication. Valid
values for sslmode are:

+----------------+----------------+
| Mode           | Description    |
+================+================+
| disable        | No SSL         |
+----------------+----------------+
| require        | Always SSL     |
|                | (skip          |
|                | verification)  |
+----------------+----------------+
| verify-ca      | Always SSL     |
|                | (verify that   |
|                | the            |
|                | certificate    |
|                | presented by   |
|                | the server was |
|                | signed by a    |
|                | trusted CA)    |
+----------------+----------------+
| verify-full    | Same as        |
|                | verify-ca AND  |
|                | verify that    |
|                | the            |
|                | certification  |
|                | presented by   |
|                | the server was |
|                | signed by a    |
|                | trusted CA and |
|                | the server     |
|                | host name      |
|                | matches the    |
|                | one in the     |
|                | certificate    |
+----------------+----------------+

If TLS would like to be used, we also need configure the TLS section in
the fabric-ca-server config file. If the database server requires client
authentication, then a client cert and key file needs to be provided.
The following should be present in the fabric-ca-server config:

::

    db:
      ...
      tls:
          enabled: false
          certfiles: db-server-cert.pem
          client:
                certfile: db-client-cert.pem
                keyfile: db-client-key.pem

| **certfiles** - PEM-encoded trusted root certificate files.
| **certfile** - PEM-encoded client certificate file.
| **keyfile** - PEM-encoded client key file containing private key associated with client certificate file.

MySQL
^^^^^^^

The following sample may be added to the fabric-ca-server config file in
order to connect to a MySQL database. Be sure to customize the various
values appropriately.

::

    db:
      type: mysql
      datasource: root:rootpw@tcp(localhost:3306)/fabric-ca?parseTime=true&tls=custom

If connecting over TLS to the MySQL server, the ``db.tls.client``
section is also required as described in the **Postgres** section above.

Configuring LDAP
~~~~~~~~~~~~~~~~

The fabric-ca-server can be configured to read from an LDAP server.

In particular, the fabric-ca-server may connect to an LDAP server to do
the following:

-  authenticate a user prior to enrollment, and
-  retrieve a user's attribute values which are used for authorization.

Modify the LDAP section of the server's configuration file to configure the
fabric-ca-server to connect to an LDAP server.

::

    ldap:
       # Enables or disables the LDAP client (default: false)
       enabled: false
       # The URL of the LDAP server
       url: scheme://<adminDN>:<adminPassword>@<host>:<port>/<base>
       userfilter: filter

| where:
| \* ``scheme`` is one of *ldap* or *ldaps*;
| \* ``adminDN`` is the distinquished name of the admin user;
| \* ``pass`` is the password of the admin user;
| \* ``host`` is the hostname or IP address of the LDAP server;
| \* ``port`` is the optional port number, where default 389 for *ldap*
  and 636 for *ldaps*;
| \* ``base`` is the optional root of the LDAP tree to use for searches;
| \* ``filter`` is a filter to use when searching to convert a login
  user name to a distinquished name. For example, a value of
  ``(uid=%s)`` searches for LDAP entries with the value of a ``uid``
  attribute whose value is the login user name. Similarly,
  ``(email=%s)`` may be used to login with an email address.

The following is a sample configuration section for the default settings
for the OpenLDAP server whose docker image is at
``https://github.com/osixia/docker-openldap``.

::

    ldap:
       enabled: true
       url: ldap://cn=admin,dc=example,dc=org:admin@localhost:10389/dc=example,dc=org
       userfilter: (uid=%s)

See ``FABRIC_CA/scripts/run-ldap-tests`` for a script which starts an
OpenLDAP docker image, configures it, runs the LDAP tests in
``FABRIC_CA/cli/server/ldap/ldap_test.go``, and stops the OpenLDAP
server.

When LDAP is configured, enrollment works as follows:


-  The fabric-ca-client or client SDK sends an enrollment request with a
   basic authorization header.
-  The fabric-ca-server receives the enrollment request, decodes the
   user name and password in the authorization header, looks up the DN (Distinquished
   Name) associated with the user name using the "userfilter" from the
   configuration file, and then attempts an LDAP bind with the user's
   password. If the LDAP bind is successful, the enrollment processing is
   authorized and can proceed.

When LDAP is configured, attribute retrieval works as follows:


-  A client SDK sends a request for a batch of tcerts **with one or more
   attributes** to the fabric-ca-server.
-  The fabric-ca-server receives the tcert request and does as follows:

   -  extracts the enrollment ID from the token in the authorization
      header (after validating the token);
   -  does an LDAP search/query to the LDAP server, requesting all of
      the attribute names received in the tcert request;
   -  the attribute values are placed in the tcert as normal.

Setting up a cluster
~~~~~~~~~~~~~~~~~~~~

You may use any IP sprayer to load balance to a cluster of fabric-ca
servers. This section provides an example of how to set up Haproxy to
route to a fabric-ca-server cluster. Be sure to change hostname and port
to reflect the settings of your fabric-ca servers.

haproxy.conf

::

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

`Back to Top`_

Fabric CA Client
----------------

This section describes how to use the fabric-ca-client command.

| The fabric-ca-client's home directory is determined as follows:
| - if the ``FABRIC_CA_CLIENT_HOME`` environment variable is set, use
  its value;
| - otherwise, if the ``FABRIC_CA_HOME`` environment variable is set,
  use its value;
| - otherwise, if the ``CA_CFG_PATH`` environment variable is set, use
  its value;
| - otherwise, use ``$HOME/.fabric-ca-client``.

The default fabric-ca-client's home directory is
``$HOME/.fabric-ca-client``, but this can be changed by setting the
``FABRIC_CA_HOME`` or ``FABRIC_CA_CLIENT_HOME`` environment variable.

The instructions below assume that the client configuration file exists
in the client's home directory.

Enrolling the bootstrap user
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

First, if desired, customize the CSR (Certificate Signing Request) in the client
configuration file.  If custom values for the CSR are required, you
must create the client config file before triggering the ``enroll``
command and place it in the client's home directory. If no client
configuration file is provided, default values will be used for CSR.

::

    csr:
      key:
        algo: ecdsa
        size: 256
      names:
        - C: US
          ST: North Carolina
          L: Raleigh
          O: Hyperledger Fabric
          OU: Fabric CA
      hosts:
       - hostname
      ca:
        pathlen:
        pathlenzero:
        expiry:

See `CSR fields <#csr-fields>`__ for a description of the fields in this
file. When enrolling, the CN (Common Name) field is automatically set to
the enrollment ID which is *admin* in this example.

The following command enrolls the admin user and stores an enrollment
certificate (ECert) in the fabric-ca-client's home directory.

::

    # export FABRIC_CA_CLIENT_HOME=$HOME/fabric-ca/clients/admin
    # fabric-ca-client enroll -u http://admin:adminpw@localhost:7054

You should see a message similar to
``[INFO] enrollment information was successfully stored in`` which
indicates where the certificate and key files were stored.

The enrollment certificate is stored at
``$FABRIC_CA_ENROLLMENT_DIR/cert.pem`` by default, but a different path
can be specified by setting the ``FABRIC_CA_CERT_FILE`` environment
variable.

The enrollment key is stored at ``$FABRIC_CA_ENROLLMENT_DIR/key.pem`` by
default, but a different path can be specified by setting the
``FABRIC_CA_KEY_FILE`` environment variable.

If ``FABRIC_CA_ENROLLMENT_DIR`` is not set, the value of the fabric
client home directory is used in its place.

Registering a new identity
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The user performing the register request must be currently enrolled, and
must also have the proper authority to register the type of user being
registered.

In particular, two authorization checks are made by the fabric-ca-server
during registration as follows.

 1. The invoker's identity must have the "hf.Registrar.Roles" attribute with a
    comma-separated list of values where one of the value equals the type of
    identity being registered; for example, if the invoker's identity has the
    "hf.Registrar.Roles" attribute with a value of "peer,app,user", the invoker
    can register identities of type peer, app, and user, but not orderer.

 2. The affiliation of the invoker's identity must be equal to or a prefix of
    the affiliation of the identity being registered.  For example, an invoker
    with an affiliation of "a.b" may register an identity with an affiliation
    of "a.b.c" but may not register an identity with an affiliation of "a.c".

To register a new identity, you must first edit the ``id`` section in
the client configuration file similar to the one below.  This information
describes the identity being registered.

::

    id:
      name: MyPeer1
      type: peer
      affiliation: org1.department1
      attributes:
        - name: SomeAttrName
          value: SomeAttrValue
        - name: foo
          value: bar

The **id** field is the enrollment ID of the identity.

The **type** field is the type of the identity: orderer, peer, app, or
user.

The **affiliation** field must be a valid group name as found in the
server configuration file.

The **attributes** field is optional and is not required for a peer, but
is shown here as example of how you associate attributes with any
identity.  Note that attribute names beginning with "hf." are reserved
for Hyperledger Fabric usage (e.g. "hf.Revoker")

The following command uses the **admin** user's credentials to register
the **peer1** identity.

::

    # export FABRIC_CA_CLIENT_HOME=$HOME/fabric-ca/clients/admin
    # fabric-ca-client register

The output of a successful *fabric-ca-client register* command is a
password similar to ``Password: gHIexUckKpHz``. Make a note of your
password to use in the following section to enroll a peer.

Suppose further than you wanted to register another peer and also want to
provide your own password (or secret).  You may do so as follows:

::

    # export FABRIC_CA_CLIENT_HOME=$HOME/fabric-ca/clients/admin
    # fabric-ca-client register --id.name MyPeer2 --id.secret mypassword

Enrolling a Peer Identity
~~~~~~~~~~~~~~~~~~~~~~~~~

Now that you have successfully registered a peer identity, you may now
enroll the peer given the enrollment ID and secret (i.e. the *password*
from the previous section).

First, create a CSR (Certificate Signing Request) request file similar
to the one described in the `Enrolling the bootstrap
user <#EnrollBootstrap>`__ section.

This is similar to enrolling the bootstrap user except that we also
demonstrate how to use environment variables to place the key and
certificate files in a specific location. The following example shows
how to place them into a Hyperledger Fabric MSP (Membership Service
Provider) directory structure. The *MSP\_DIR* environment variable
refers to the root directory of MSP in Hyperledger Fabric and the
``$MSP_DIR/signcerts`` and ``$MSP_DIR/keystore`` directories must exist.

::

    # export FABRIC_CA_CERT_FILE=$MSP_DIR/signcerts/peer.pem
    # export FABRIC_CA_KEY_FILE=$MSP_DIR/keystore/key.pem
    # fabric-ca client enroll -u http://peer1:<password>@localhost:7054

The cert.pem and key.pem files should now exist at the locations
specified by the environment variables.

Reenrolling an Identity
~~~~~~~~~~~~~~~~~~~~~~~

Suppose your enrollment certificate is about to expire. You can issue
the reenroll command to renew your enrollment certificate as follows.
Note that this is identical to the enroll command except that no username or
password is required. Instead, your previously stored private key is
used to authenticate to the Fabric CA server.

::

    # export FABRIC_CA_CLIENT_HOME=$HOME/fabric-ca/clients/admin
    # cd $FABRIC_CA_HOME
    # fabric-ca-client reenroll

The enrollment certificate and enrollment key are stored in the same
location as described in the previous section for the ``enroll``
command.

Revoking a certificate or identity
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

In order to revoke a certificate or user, the calling identity must have
the ``hf.Revoker`` attribute. The revoking identity can only revoke a
certificate or user that has an affiliation that is equal to or prefixed
by the revoking identity's affiliation.

For example, a revoker with affiliation bank.bank\_1 can revoke user
with bank.bank1.dep1 but can't revoke bank.bank2.

You may revoke a specific certificate by specifying its AKI (Authority
Key Identifier) and its serial number as follows:

::

    fabric-ca-client revoke -a xxx -s yyy -r <reason>

The following command disables a user's identity and also revokes all of
the certificates associated with the identity. All future requests
received by the fabric-ca-server from this identity will be rejected.

::

    fabric-ca-client revoke -e <enrollment_id> -r <reason>

The following are the supported reasons for revoking that can be
specified using ``-r`` flag.

| **Reasons:**
| - unspecified
| - keycompromise
| - cacompromise
| - affiliationchange
| - superseded
| - cessationofoperation
| - certificatehold
| - removefromcrl
| - privilegewithdrawn
| - aacompromise

Enabling TLS
~~~~~~~~~~~~

This section describes in more detail how to configure TLS for a
fabric-ca-client.

The following sections may be configured in the ``fabric-ca-client-config.yaml``.

::

    tls:
      # Enable TLS (default: false)
      enabled: true

      # TLS for the client's listenting port (default: false)
      certfiles: root.pem   # Comma Separated (e.g. root.pem,root2.pem)
      client:
        certfile: tls_client-cert.pem
        keyfile: tls_client-key.pem

The **certfiles** option is the set of root certificates trusted by the
client. This will typically just be the root fabric-ca-server's
certificate found in the server's home directory in the **ca-cert.pem**
file.

The **client** option is required only if mutual TLS is configured on
the server.

`Back to Top`_

Appendix
--------

Postgres SSL Configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~

**Basic instructions for configuring SSL on Postgres server:** 1. In
postgresql.conf, uncomment SSL and set to "on" (SSL=on) 2. Place
Certificate and Key files Postgress data directory.

Instructions for generating self-signed certificates for:
https://www.postgresql.org/docs/9.1/static/ssl-tcp.html

Note: Self-signed certificates are for testing purposes and should not
be used in a production environment

**Postgres Server - Require Client Certificates** 1. Place certificates
of the certificate authorities (CAs) you trust in the file root.crt in
the Postgres data directory 2. In postgresql.conf, set "ssl\_ca\_file"
to point to the root cert of client (CA cert) 3. Set the clientcert
parameter to 1 on the appropriate hostssl line(s) in pg\_hba.conf.

For more details on configuring SSL on the Postgres server, please refer
to the following Postgres documentation:
https://www.postgresql.org/docs/9.4/static/libpq-ssl.html

MySQL SSL Configuration
~~~~~~~~~~~~~~~~~~~~~~~

On MySQL 5.7, strict mode affects whether the server permits '0000-00-00' as a valid date:
If strict mode is not enabled, '0000-00-00' is permitted and inserts
produce no warning. If strict mode is enabled, '0000-00-00' is not permitted
and inserts produce an error.

**Disabling STRICT_TRANS_TABLES mode**

However to allow the format 0000-00-00 00:00:00, you have to disable
STRICT_TRANS_TABLES mode in mysql config file or by command

**Command:** SET sql_mode = '';

**File:** Go to /etc/mysql/my.cnf and comment out STRICT_TRANS_TABLES

**Basic instructions for configuring SSL on MySQL server:**

1. Open or create my.cnf file for the server. Add or un-comment the
   lines below in [mysqld] section. These should point to the key and
   certificates for the server, and the root CA cert.

   Instruction on creating server and client side certs:
   http://dev.mysql.com/doc/refman/5.7/en/creating-ssl-files-using-openssl.html

   [mysqld] ssl-ca=ca-cert.pem ssl-cert=server-cert.pem ssl-key=server-key.pem

   Can run the following query to confirm SSL has been enabled.

   mysql> SHOW GLOBAL VARIABLES LIKE 'have\_%ssl';

   Should see:

   +----------------+----------------+
   | Variable_name  | Value          |
   +================+================+
   | have_openssl   | YES            |
   +----------------+----------------+
   | have_ssl       | YES            |
   +----------------+----------------+

2. After the server-side SSL configuration is finished, the next step is
   to create a user who has a privilege to access the MySQL server over
   SSL. For that, log in to the MySQL server, and type:

   mysql> GRANT ALL PRIVILEGES ON *.* TO 'ssluser'@'%' IDENTIFIED BY
   'password' REQUIRE SSL; mysql> FLUSH PRIVILEGES;

   If you want to give a specific ip address from which the user will
   access the server change the '%' to the specific ip address.

**MySQL Server - Require Client Certificates** Options for secure
connections are similar to those used on the server side.

-  ssl-ca identifies the Certificate Authority (CA) certificate. This
   option, if used, must specify the same certificate used by the
   server.
-  ssl-cert identifies the client public key certificate.
-  ssl-key identifies the client private key.

Suppose that you want to connect using an account that has no special
encryption requirements or was created using a GRANT statement that
includes the REQUIRE SSL option. As a recommended set of
secure-connection options, start the MySQL server with at least
--ssl-cert and --ssl-key, and invoke the fabric-ca-server with
**ca\_certfiles** option set in the fabric-ca-server file.

To require that a client certificate also be specified, create the
account using the REQUIRE X509 option. Then the client must also specify
the proper client key and certificate files or the MySQL server will
reject the connection. CA cert, client cert, and client key are all
required for the fabric-ca-server.

`Back to Top`_
