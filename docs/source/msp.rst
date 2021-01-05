Membership Service Providers (MSP)
==================================

For a conceptual overview of the Membership Service Provider (MSP), check out
:doc:`membership/membership`.

This topic elaborates on the setup of the MSP implementation supported by
Hyperledger Fabric and discusses best practices concerning its use.

MSP configuration
-----------------

To setup an instance of the MSP, its configuration needs to be specified
locally at each peer and orderer (to enable peer and orderer signing),
and on the channels to enable peer, orderer, client identity validation, and
respective signature verification (authentication) by and for all channel
members.

Firstly, for each MSP a name needs to be specified in order to reference that MSP
in the network (e.g. ``msp1``, ``org2``, and ``org3.divA``). This is the name under
which membership rules of an MSP representing a consortium, organization or
organization division is to be referenced in a channel. This is also referred
to as the *MSP Identifier* or *MSP ID*. MSP Identifiers are required to be unique per MSP
instance.

In the case of the default MSP implementation, a set of parameters need to be
specified to allow for identity (certificate) validation and signature
verification. These parameters are deduced by
`RFC5280 <http://www.ietf.org/rfc/rfc5280.txt>`_, and include:

- A list of self-signed (X.509) CA certificates to constitute the *root of
  trust*
- A list of X.509 certificates to represent intermediate CAs this provider
  considers for certificate validation; these certificates ought to be
  certified by exactly one of the certificates in the root of trust;
  intermediate CAs are optional parameters
- A list of X.509 certificates representing the administrators of this MSP with a
  verifiable certificate path to exactly one of the CA certificates of the
  root of trust; owners of these certificates are authorized to request changes
  to this MSP configuration (e.g. root CAs, intermediate CAs)
- A list of Organizational Units that valid members of this MSP should
  include in their X.509 certificate; this is an optional configuration
  parameter, used when, e.g., multiple organizations leverage the same
  root of trust, and intermediate CAs, and have reserved an OU field for
  their members
- A list of certificate revocation lists (CRLs) each corresponding to
  exactly one of the listed (intermediate or root) MSP Certificate
  Authorities; this is an optional parameter
- A list of self-signed (X.509) certificates to constitute the *TLS root of
  trust* for TLS certificates.
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

`Openssl <https://www.openssl.org/>`_ can be used to generate X.509
certificates and keys. Please note that Hyperledger Fabric does not support
RSA key and certificates.

The Hyperledger Fabric CA can also be used to generate the keys and certificates
needed to configure an MSP. Check out
`Registering and enrolling identities with a CA <https://hyperledger-fabric-ca.readthedocs.io/en/latest/deployguide/use_CA.html>`_
for more information about how to generate MSPs for nodes and organizations.

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

Identity classification
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

Adding MSPs to channels
-----------------------

For information about how to add MSPs to a channel (including the decision of
whether to bootstrap ordering nodes with a system channel genesis block), check
out :doc:`create_channel/create_channel_overview`.

Best practices
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
  be defined to capture members of a specific ``role`` (should be one of: peer, admin,
  client, orderer, member), and these policies may constitute the read/write policies
  of a channel or endorsement policies of a chaincode. Specifying custom OUs in
  the profile section of ``configtx.yaml`` is currently not configured.
  A limitation of this approach is that gossip peers would
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

**5) Blocking an intermediate CA.**

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
and does not require blocking the no longer considered intermediate CA.

**6) CAs and TLS CAs**

MSP identities' root CAs and MSP TLS certificates' root CAs (and relative intermediate CAs)
need to be declared in different folders. This is to avoid confusion between
different classes of certificates. It is not forbidden to reuse the same
CAs for both MSP identities and TLS certificates but best practices suggest
to avoid this in production.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
