Membership Service Providers (MSP)
==================================

The document serves to provide details on the setup and best practices for MSPs.

Membership Service Provider (MSP) is a component that aims to offer an
abstraction of a membership operation architecture.

In particular, MSP abstracts away all cryptographic mechanisms and protocols
behind issuing and validating certificates, and user authentication. An
MSP may define their own notion of identity, and the rules by which those
identities are governed (identity validation) and authenticated (signature
generation and verification).

A Hyperledger Fabric blockchain network can be governed by one or more MSPs.
This provides modularity of membership operations, and interoperability
across different membership standards and architectures.

In the rest of this document we elaborate on the setup of the MSP
implementation supported by Hyperledger Fabric, and discuss best practices
concerning its use.

MSP Configuration
-----------------

To setup an instance of the MSP, its configuration needs to be specified
locally at each peer and orderer (to enable peer, and orderer signing),
and on the channels to enable peer, orderer, client identity validation, and
respective signature verification (authentication) by and for all channel
members.

Firstly, for each MSP a name needs to be specified in order to reference that MSP
in the network (e.g. ``msp1``, ``org2``, and ``org3.divA``). This is the name under
which membership rules of an MSP representing a consortium, organization or
organization division is to be referenced in a channel. This is also referred
to as the *MSP Identifier* or *MSP ID*. MSP Identifiers are required to be unique per MSP
instance. For example, shall two MSP instances with the same identifier be
detected at the system channel genesis, orderer setup will fail.

In the case of default implementation of MSP, a set of parameters need to be
specified to allow for identity (certificate) validation and signature
verification. These parameters are deduced by
`RFC5280 <http://www.ietf.org/rfc/rfc5280.txt>`_, and include:

- A list of self-signed (X.509) certificates to constitute the *root of
  trust*
- A list of X.509 certificates to represent intermediate CAs this provider
  considers for certificate validation; these certificates ought to be
  certified by exactly one of the certificates in the root of trust;
  intermediate CAs are optional parameters
- A list of X.509 certificates with a verifiable certificate path to
  exactly one of the certificates of the root of trust to represent the
  administrators of this MSP; owners of these certificates are authorized
  to request changes to this MSP configuration (e.g. root CAs, intermediate CAs)
- A list of Organizational Units that valid members of this MSP should
  include in their X.509 certificate; this is an optional configuration
  parameter, used when, e.g., multiple organizations leverage the same
  root of trust, and intermediate CAs, and have reserved an OU field for
  their members
- A list of certificate revocation lists (CRLs) each corresponding to
  exactly one of the listed (intermediate or root) MSP Certificate
  Authorities; this is an optional parameter
- A list of self-signed (X.509) certificates to constitute the *TLS root of
  trust* for TLS certificate.
- A list of X.509 certificates to represent intermediate TLS CAs this provider
  considers; these certificates ought to be
  certified by exactly one of the certificates in the TLS root of trust;
  intermediate CAs are optional parameters.

*Valid*  identities for this MSP instance are required to satisfy the following conditions:

- They are in the form of X.509 certificates with a verifiable certificate path to
  exactly one of the root of trust certificates;
- They are not included in any CRL;
- And they *list* one or more of the Organizational Units of the MSP configuration
  in the ``OU`` field of their X.509 certificate structure.

For more information on the validity of identities in the current MSP implementation,
we refer the reader to :doc:`msp-identity-validity-rules`.

In addition to verification related parameters, for the MSP to enable
the node on which it is instantiated to sign or authenticate, one needs to
specify:

- The signing key used for signing by the node (currently only ECDSA keys are
  supported), and
- The node's X.509 certificate, that is a valid identity under the
  verification parameters of this MSP.

It is important to note that MSP identities never expire; they can only be revoked
by adding them to the appropriate CRLs. Additionally, there is currently no
support for enforcing revocation of TLS certificates.

How to generate MSP certificates and their signing keys?
--------------------------------------------------------

To generate X.509 certificates to feed its MSP configuration, the application
can use `Openssl <https://www.openssl.org/>`_. We emphasize that in Hyperledger
Fabric there is no support for certificates including RSA keys.

Alternatively one can use ``cryptogen`` tool, whose operation is explained in
:doc:`getting_started`.

`Hyperledger Fabric CA <http://hyperledger-fabric-ca.readthedocs.io/en/latest/>`_
can also be used to generate the keys and certificates needed to configure an MSP.

MSP setup on the peer & orderer side
------------------------------------

To set up a local MSP (for either a peer or an orderer), the administrator
should create a folder (e.g. ``$MY_PATH/mspconfig``) that contains six subfolders
and a file:

1. a folder ``admincerts`` to include PEM files each corresponding to an
   administrator certificate
2. a folder ``cacerts`` to include PEM files each corresponding to a root
   CA's certificate
3. (optional) a folder ``intermediatecerts`` to include PEM files each
   corresponding to an intermediate CA's certificate
4. (optional) a file ``config.yaml`` to configure the supported Organizational Units
   and identity classifications (see respective sections below).
5. (optional) a folder ``crls`` to include the considered CRLs
6. a folder ``keystore`` to include a PEM file with the node's signing key;
   we emphasise that currently RSA keys are not supported
7. a folder ``signcerts`` to include a PEM file with the node's X.509
   certificate
8. (optional) a folder ``tlscacerts`` to include PEM files each corresponding to a TLS root
   CA's certificate
9. (optional) a folder ``tlsintermediatecerts`` to include PEM files each
   corresponding to an intermediate TLS CA's certificate

In the configuration file of the node (core.yaml file for the peer, and
orderer.yaml for the orderer), one needs to specify the path to the
mspconfig folder, and the MSP Identifier of the node's MSP. The path to the
mspconfig folder is expected to be relative to FABRIC_CFG_PATH and is provided
as the value of parameter ``mspConfigPath`` for the peer, and ``LocalMSPDir``
for the orderer. The identifier of the node's MSP is provided as a value of
parameter ``localMspId`` for the peer and ``LocalMSPID`` for the orderer.
These variables can be overridden via the environment using the CORE prefix for
peer (e.g. CORE_PEER_LOCALMSPID) and the ORDERER prefix for the orderer (e.g.
ORDERER_GENERAL_LOCALMSPID). Notice that for the orderer setup, one needs to
generate, and provide to the orderer the genesis block of the system channel.
The MSP configuration needs of this block are detailed in the next section.

*Reconfiguration* of a "local" MSP is only possible manually, and requires that
the peer or orderer process is restarted. In subsequent releases we aim to
offer online/dynamic reconfiguration (i.e. without requiring to stop the node
by using a node managed system chaincode).

Organizational Units
--------------------

In order to configure the list of Organizational Units that valid members of this MSP should
include in their X.509 certificate, the ``config.yaml`` file
needs to specify the organizational unit (OU, for short) identifiers. You can find an example
below:

::

   OrganizationalUnitIdentifiers:
     - Certificate: "cacerts/cacert1.pem"
       OrganizationalUnitIdentifier: "commercial"
     - Certificate: "cacerts/cacert2.pem"
       OrganizationalUnitIdentifier: "administrators"

The above example declares two organizational unit identifiers: **commercial** and **administrators**.
An MSP identity is valid if it carries at least one of these organizational unit identifiers.
The ``Certificate`` field refers to the CA or intermediate CA certificate path
under which identities, having that specific OU, should be validated.
The path is relative to the MSP root folder and cannot be empty.

Identity Classification
-----------------------

The default MSP implementation allows organizations to further classify identities into clients,
admins, peers, and orderers based on the OUs of their x509 certificates.

* An identity should be classified as a **client** if it transacts on the network.
* An identity should be classified as an **admin** if it handles administrative tasks such as
  joining a peer to a channel or signing a channel configuration update transaction.
* An identity should be classified as a **peer** if it endorses or commits transactions.
* An identity should be classified as an **orderer** if belongs to an ordering node.

In order to define the clients, admins, peers, and orderers of a given MSP, the ``config.yaml`` file
needs to be set appropriately. You can find an example NodeOU section of the ``config.yaml`` file
below:

::

   NodeOUs:
     Enable: true
     # For each identity classification that you would like to utilize, specify
     # an OU identifier.
     # You can optionally configure that the OU identifier must be issued by a specific CA
     # or intermediate certificate from your organization. However, it is typical to NOT
     # configure a specific Certificate. By not configuring a specific Certificate, you will be
     # able to add other CA or intermediate certs later, without having to reissue all credentials.
     # For this reason, the sample below comments out the Certificate field.
     ClientOUIdentifier:
       # Certificate: "cacerts/cacert.pem"
       OrganizationalUnitIdentifier: "client"
     AdminOUIdentifier:
       # Certificate: "cacerts/cacert.pem"
       OrganizationalUnitIdentifier: "admin"
     PeerOUIdentifier:
       # Certificate: "cacerts/cacert.pem"
       OrganizationalUnitIdentifier: "peer"
     OrdererOUIdentifier:
       # Certificate: "cacerts/cacert.pem"
       OrganizationalUnitIdentifier: "orderer"

Identity classification is enabled when ``NodeOUs.Enable`` is set to ``true``. Then the client
(admin, peer, orderer) organizational unit identifier is defined by setting the properties of
the ``NodeOUs.ClientOUIdentifier`` (``NodeOUs.AdminOUIdentifier``, ``NodeOUs.PeerOUIdentifier``,
``NodeOUs.OrdererOUIdentifier``) key:

a. ``OrganizationalUnitIdentifier``: Is the OU value that the x509 certificate needs to contain
   to be considered a client (admin, peer, orderer respectively). If this field is empty, then the classification
   is not applied.
b. ``Certificate``: (Optional) Set this to the path of the CA or intermediate CA certificate
   under which client (peer, admin or orderer) identities should be validated.
   The field is relative to the MSP root folder. Only a single Certificate can be specified.
   If you do not set this field, then the identities are validated under any CA defined in
   the organization's MSP configuration, which could be desirable in the future if you need
   to add other CA or intermediate certificates.

Notice that if the ``NodeOUs.ClientOUIdentifier`` section (``NodeOUs.AdminOUIdentifier``,
``NodeOUs.PeerOUIdentifier``, ``NodeOUs.OrdererOUIdentifier``) is missing, then the classification
is not applied. If ``NodeOUs.Enable`` is set to ``true`` and no classification keys are defined,
then identity classification is assumed to be disabled.

Identities can use organizational units to be classified as either a client, an admin, a peer, or an
orderer. The four classifications are mutually exclusive.
The 1.1 channel capability needs to be enabled before identities can be classified as clients
or peers. The 1.4.3 channel capability needs to be enabled for identities to be classified as an
admin or orderer.

Classification allows identities to be classified as admins (and conduct administrator actions)
without the certificate being stored in the ``admincerts`` folder of the MSP. Instead, the
``admincerts`` folder can remain empty and administrators can be created by enrolling identities
with the admin OU. Certificates in the ``admincerts`` folder will still grant the role of
administrator to their bearer, provided that they possess the client or admin OU.

Channel MSP setup
-----------------

At the genesis of the system, verification parameters of all the MSPs that
appear in the network need to be specified, and included in the system
channel's genesis block. Recall that MSP verification parameters consist of
the MSP identifier, the root of trust certificates, intermediate CA and admin
certificates, as well as OU specifications and CRLs.
The system genesis block is provided to the orderers at their setup phase,
and allows them to authenticate channel creation requests. Orderers would
reject the system genesis block, if the latter includes two MSPs with the same
identifier, and consequently the bootstrapping of the network would fail.

For application channels, the verification components of only the MSPs that
govern a channel need to reside in the channel's genesis block. We emphasize
that it is **the responsibility of the application** to ensure that correct
MSP configuration information is included in the genesis blocks (or the
most recent configuration block) of a channel prior to instructing one or
more of their peers to join the channel.

When bootstrapping a channel with the help of the configtxgen tool, one can
configure the channel MSPs by including the verification parameters of MSP
in the mspconfig folder, and setting that path in the relevant section in
``configtx.yaml``.

*Reconfiguration* of an MSP on the channel, including announcements of the
certificate revocation lists associated to the CAs of that MSP is achieved
through the creation of a ``config_update`` object by the owner of one of the
administrator certificates of the MSP. The client application managed by the
admin would then announce this update to the channels in which this MSP appears.

Best Practices
--------------

In this section we elaborate on best practices for MSP
configuration in commonly met scenarios.

**1) Mapping between organizations/corporations and MSPs**

We recommend that there is a one-to-one mapping between organizations and MSPs.
If a different type of mapping is chosen, the following needs to be to
considered:

- **One organization employing various MSPs.** This corresponds to the
  case of an organization including a variety of divisions each represented
  by its MSP, either for management independence reasons, or for privacy reasons.
  In this case a peer can only be owned by a single MSP, and will not recognize
  peers with identities from other MSPs as peers of the same organization. The
  implication of this is that peers may share through gossip organization-scoped
  data with a set of peers that are members of the same subdivision, and NOT with
  the full set of providers constituting the actual organization.
- **Multiple organizations using a single MSP.** This corresponds to a
  case of a consortium of organizations that are governed by similar
  membership architecture. One needs to know here that peers would propagate
  organization-scoped messages to the peers that have an identity under the
  same MSP regardless of whether they belong to the same actual organization.
  This is a limitation of the granularity of MSP definition, and/or of the peer’s
  configuration.

**2) One organization has different divisions (say organizational units), to**
**which it wants to grant access to different channels.**

Two ways to handle this:

- **Define one MSP to accommodate membership for all organization’s members**.
  Configuration of that MSP would consist of a list of root CAs,
  intermediate CAs and admin certificates; and membership identities would
  include the organizational unit (``OU``) a member belongs to. Policies can then
  be defined to capture members of a specific ``OU``, and these policies may
  constitute the read/write policies of a channel or endorsement policies of
  a chaincode. A limitation of this approach is that gossip peers would
  consider peers with membership identities under their local MSP as
  members of the same organization, and would consequently gossip
  with them organization-scoped data (e.g. their status).
- **Defining one MSP to represent each division**.  This would involve specifying for each
  division, a set of certificates for root CAs, intermediate CAs, and admin
  Certs, such that there is no overlapping certification path across MSPs.
  This would mean that, for example, a different intermediate CA per subdivision
  is employed. Here the disadvantage is the management of more than one
  MSPs instead of one, but this circumvents the issue present in the previous
  approach.  One could also define one MSP for each division by leveraging an OU
  extension of the MSP configuration.

**3) Separating clients from peers of the same organization.**

In many cases it is required that the “type” of an identity is retrievable
from the identity itself (e.g. it may be needed that endorsements are
guaranteed to have derived by peers, and not clients or nodes acting solely
as orderers).

There is limited support for such requirements.

One way to allow for this separation is to create a separate intermediate
CA for each node type - one for clients and one for peers/orderers; and
configure two different MSPs - one for clients and one for peers/orderers.
Channels this organization should be accessing would need to include
both MSPs, while endorsement policies will leverage only the MSP that
refers to the peers. This would ultimately result in the organization
being mapped to two MSP instances, and would have certain consequences
on the way peers and clients interact.

Gossip would not be drastically impacted as all peers of the same organization
would still belong to one MSP. Peers can restrict the execution of certain
system chaincodes to local MSP based policies. For
example, peers would only execute “joinChannel” request if the request is
signed by the admin of their local MSP who can only be a client (end-user
should be sitting at the origin of that request). We can go around this
inconsistency if we accept that the only clients to be members of a
peer/orderer MSP would be the administrators of that MSP.

Another point to be considered with this approach is that peers
authorize event registration requests based on membership of request
originator within their local MSP. Clearly, since the originator of the
request is a client, the request originator is always deemed to belong
to a different MSP than the requested peer and the peer would reject the
request.

**4) Admin and CA certificates.**

It is important to set MSP admin certificates to be different than any of the
certificates considered by the MSP for ``root of trust``, or intermediate CAs.
This is a common (security) practice to separate the duties of management of
membership components from the issuing of new certificates, and/or validation of existing ones.

**5) Blacklisting an intermediate CA.**

As mentioned in previous sections, reconfiguration of an MSP is achieved by
reconfiguration mechanisms (manual reconfiguration for the local MSP instances,
and via properly constructed ``config_update`` messages for MSP instances of a channel).
Clearly, there are two ways to ensure an intermediate CA considered in an MSP is no longer
considered for that MSP's identity validation:

1. Reconfigure the MSP to no longer include the certificate of that
   intermediate CA in the list of trusted intermediate CA certs. For the
   locally configured MSP, this would mean that the certificate of this CA is
   removed from the ``intermediatecerts`` folder.
2. Reconfigure the MSP to include a CRL produced by the root of trust
   which denounces the mentioned intermediate CA's certificate.

In the current MSP implementation we only support method (1) as it is simpler
and does not require blacklisting the no longer considered intermediate CA.

**6) CAs and TLS CAs**

MSP identities' root CAs and MSP TLS certificates' root CAs (and relative intermediate CAs)
need to be declared in different folders. This is to avoid confusion between
different classes of certificates. It is not forbidden to reuse the same
CAs for both MSP identities and TLS certificates but best practices suggest
to avoid this in production.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
