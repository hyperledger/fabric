an# Membership

If you've read through the documentation on [identity](../identity/identity.html)
you've seen how a PKI can provide verifiable identities through a chain
of trust. Now let's see how these identities can be used to represent the
trusted members of a blockchain network.

This is where a **Membership Service** Provider (MSP) comes into play ---
**it identifies which Root CAs and Intermediate CAs are trusted to define
the members of a trust domain, e.g., an organization**, either by listing the
identities of their members, or by identifying which CAs are authorized to
issue valid identities for their members, or --- as will usually be the case ---
through a combination of both.

The power of an MSP goes beyond simply listing who is a network participant or
member of a channel. An MSP can identify specific **roles** an actor might play either
within the scope of the organization the MSP represents (e.g., admins, or as members
of a sub-organization group), and sets the basis for defining **access privileges**
in the context of a network and channel (e.g., channel admins, readers, writers).

The configuration of an MSP is advertised to all the channels where members of
the corresponding organization participate (in the form of a **channel MSP**). In
addition to the channel MSP, peers, orderers, and clients also maintain a **local MSP**
to authenticate member messages outside the context of a channel and to define the
permissions over a particular component (who has the ability to install chaincode on
a peer, for example).

In addition, an MSP can allow for the identification of a list of identities that
have been revoked --- as discussed in the [Identity](../identity/identity.html)
documentation --- but we will talk about how that process also extends to an MSP.

We'll talk more about local and channel MSPs in a moment. For now let's see what
MSPs do in general.

### Mapping MSPs to Organizations

An **organization** is a managed group of members. This can be something as big
as a multinational corporation or a small as a flower shop. What's most
important about organizations (or **orgs**) is that they manage their
members under a single MSP. Note that this is different from the organization
concept defined in an X.509 certificate, which we'll talk about later.

The exclusive relationship between an organization and its MSP makes it sensible to
name the MSP after the organization, a convention you'll find adopted in most policy
configurations. For example, organization `ORG1` would likely have an MSP called
something like `ORG1-MSP`. In some cases an organization may require multiple
membership groups --- for example, where channels are used to perform very different
business functions between organizations. In these cases it makes sense to have
multiple MSPs and name them accordingly, e.g., `ORG2-MSP-NATIONAL` and
`ORG2-MSP-GOVERNMENT`, reflecting the different membership roots of trust within
`ORG2` in the `NATIONAL` sales channel compared to the `GOVERNMENT` regulatory
channel.

![MSP1](./membership.diagram.3.png)

*Two different MSP configurations for an organization. The first configuration shows
the typical relationship between an MSP and an organization --- a single MSP defines
the list of members of an organization. In the second configuration, different MSPs
are used to represent different organizational groups with national, international,
and governmental affiliation.*

#### Organizational Units and MSPs

An organization is often divided up into multiple **organizational units** (OUs), each
of which has a certain set of responsibilities. For example, the `ORG1`
organization might have both `ORG1-MANUFACTURING` and `ORG1-DISTRIBUTION` OUs
to reflect these separate lines of business. When a CA issues X.509 certificates,
the `OU` field in the certificate specifies the line of business to which the
identity belongs.

We'll see later how OUs can be helpful to control the parts of an organization who
are considered to be the members of a blockchain network. For example, only
identities from the `ORG1-MANUFACTURING` OU might be able to access a channel,
whereas `ORG1-DISTRIBUTION` cannot.

Finally, though this is a slight misuse of OUs, they can sometimes be used by
different organizations in a consortium to distinguish each other. In such cases, the
different organizations use the same Root CAs and Intermediate CAs for their chain
of trust, but assign the `OU` field to identify members of each organization.
We'll also see how to configure MSPs to achieve this later.

### Local and Channel MSPs

MSPs appear in two places in a blockchain network: channel configuration
(**channel MSPs**), and locally on an actor's premise (**local MSP**). **Local MSPs are
defined for clients (users) and for nodes (peers and orderers)**. Node local MSPs define
the permissions for that node (who the peer admins are, for example). The local MSPs
of the users allow the user side to authenticate itself in its transactions as a member
of a channel (e.g. in chaincode transactions), or as the owner of a specific role
into the system (an org admin, for example, in configuration transactions).

**Every node and user must have a local MSP defined**, as it defines who has
administrative or participatory rights at that level (peer admins will not necessarily
be channel admins, and vice versa).

In contrast, **channel MSPs define administrative and participatory rights at the
channel level**. Every organization participating in a channel must have an MSP
defined for it. Peers and orderers on a channel will all share the same view of channel
MSPs, and will therefore be able to correctly authenticate the channel participants.
This means that if an organization wishes to join the channel, an MSP incorporating
the chain of trust for the organization's members would need to be included in the
channel configuration. Otherwise transactions originating from this organization's
identities will be rejected.

The key difference here between local and channel MSPs is not how they function
--- both turn identities into roles --- but their **scope**.

<a name="msp2img"></a>

![MSP2](./membership.diagram.4.png)

*Local and channel MSPs. The trust domain (e.g., the organization) of each
peer is defined by the peer's local MSP, e.g., ORG1 or ORG2. Representation
of an organization on a channel is achieved by adding the organization's MSP to
the channel configuration. For example, the channel of this figure is managed by
both ORG1 and ORG2. Similar principles apply for the network, orderers, and users,
but these are not shown here for simplicity.*

You may find it helpful to see how local and channel MSPs are used by seeing
what happens when a blockchain administrator installs and instantiates a smart
contract, as shown in the [diagram above](#msp2img).

An administrator `B` connects to the peer with an identity issued by `RCA1`
and stored in their local MSP. When `B` tries to install a smart contract on
the peer, the peer checks its local MSP, `ORG1-MSP`, to verify that the identity
of `B` is indeed a member of `ORG1`. A successful verification will allow the
install command to complete successfully. Subsequently, `B` wishes
to instantiate the smart contract on the channel. Because this is a channel
operation, all organizations on the channel must agree to it. Therefore, the
peer must check the MSPs of the channel before it can successfully commit this
command. (Other things must happen too, but concentrate on the above for now.)

**Local MSPs are only defined on the file system of the node or user** to which
they apply. Therefore, physically and logically there is only one local MSP per
node or user. However, as channel MSPs are available to all nodes in the
channel, they are logically defined once in the channel configuration. However,
**a channel MSP is also instantiated on the file system of every node in the
channel and kept synchronized via consensus**. So while there is a copy of each
channel MSP on the local file system of every node, logically a channel MSP
resides on and is maintained by the channel or the network.

### MSP Levels

The split between channel and local MSPs reflects the needs of organizations
to administer their local resources, such as a peer or orderer nodes, and their
channel resources, such as ledgers, smart contracts, and consortia, which
operate at the channel or network level. It's helpful to think of these MSPs
as being at different **levels**, with **MSPs at a higher level relating to
network administration concerns** while **MSPs at a lower level handle
identity for the administration of private resources**. MSPs are mandatory
at every level of administration --- they must be defined for the network,
channel, peer, orderer, and users.

![MSP3](./membership.diagram.2.png)

*MSP Levels. The MSPs for the peer and orderer are local, whereas the MSPs for a
channel (including the network configuration channel) are shared across all
participants of that channel. In this figure, the network configuration channel
is administered by ORG1, but another application channel can be managed by ORG1
and ORG2. The peer is a member of and managed by ORG2, whereas ORG1 manages the
orderer of the figure. ORG1 trusts identities from RCA1, whereas ORG2 trusts
identities from RCA2. Note that these are administration identities, reflecting
who can administer these components. So while ORG1 administers the network,
ORG2.MSP does exist in the network definition.*

 * **Network MSP:** The configuration of a network defines who are the
 members in the network --- by defining the MSPs of the participant organizations
 --- as well as which of these members are authorized to perform
 administrative tasks (e.g., creating a channel).

 * **Channel MSP:** It is important for a channel to maintain the MSPs of its members
 separately. A channel provides private communications between a particular set of
 organizations which in turn have administrative control over it. Channel policies
 interpreted in the context of that channel's MSPs define who has ability to
 participate in certain action on the channel, e.g., adding organizations, or
 instantiating chaincodes. Note that there is no necessary relationship between
 the permission to administrate a channel and the ability to administrate the
 network configuration channel (or any other channel). Administrative rights
 exist within the scope of what is being administrated (unless the rules have
 been written otherwise --- see the discussion of the `ROLE` attribute below).

 * **Peer MSP:** This local MSP is defined on the file system of each peer and there is a
 single MSP instance for each peer. Conceptually, it performs exactly the same function
 as channel MSPs with the restriction that it only applies to the peer where it is defined.
 An example of an action whose authorization is evaluated using the peer's local MSP is
 the installation of a chaincode on the peer.

 * **Orderer MSP:** Like a peer MSP, an orderer local MSP is also defined on the file system
 of the node and only applies to that node. Like peer nodes, orderers are also owned by a single
 organization and therefore have a single MSP to list the actors or nodes it trusts.

### MSP Structure

So far, you've seen that the most important element of an MSP are the specification
of the root or intermediate CAs that are used to establish an actor's or node's
membership in the respective organization. There are, however, more elements that are
used in conjunction with these two to assist with membership functions.

![MSP4](./membership.diagram.5.png)

*The figure above shows how a local MSP is stored on a local filesystem. Even though
channel MSPs are not physically structured in exactly this way, it's still a helpful
way to think about them.*

As you can see, there are nine elements to an MSP. It's easiest to think of these elements
in a directory structure, where the MSP name is the root folder name with each
subfolder representing different elements of an MSP configuration.

Let's describe these folders in a little more detail and see why they are important.

* **Root CAs:** This folder contains a list of self-signed X.509 certificates of
  the Root CAs trusted by the organization represented by this MSP.
  There must be at least one Root CA X.509 certificate in this MSP folder.

  This is the most important folder because it identifies the CAs from
  which all other certificates must be derived to be considered members of the
  corresponding organization.


* **Intermediate CAs:** This folder contains a list of X.509 certificates of the
  Intermediate CAs trusted by this organization. Each certificate must be signed by
  one of the Root CAs in the MSP or by an Intermediate CA whose issuing CA chain ultimately
  leads back to a trusted Root CA.

  An intermediate CA may represent a different subdivision of the organization
  (like `ORG1-MANUFACTURING` and `ORG1-DISTRIBUTION` do for `ORG1`), or the
  organization itself (as may be the case if a commercial CA is leveraged for
  the organization's identity management). In the latter case intermediate CAs
  can be used to represent organization subdivisions. [Here](../msp.html) you
  may find more information on best practices for MSP configuration. Notice, that
  it is possible to have a functioning network that does not have an Intermediate
  CA, in which case this folder would be empty.

  Like the Root CA folder, this folder defines the CAs from which certificates must be
  issued to be considered members of the organization.


* **Organizational Units (OUs):** These are listed in the `$FABRIC_CFG_PATH/msp/config.yaml`
  file and contain a list of organizational units, whose members are considered
  to be part of the organization represented by this MSP. This is particularly
  useful when you want to restrict the members of an organization to the ones
  holding an identity (signed by one of MSP designated CAs) with a specific OU
  in it.

  Specifying OUs is optional. If no OUs are listed, all the identities that are part of
  an MSP --- as identified by the Root CA and Intermediate CA folders --- will be considered
  members of the organization.


* **Administrators:** This folder contains a list of identities that define the
  actors who have the role of administrators for this organization. For the standard MSP type,
  there should be one or more X.509 certificates in this list.

  It's worth noting that just because an actor has the role of an administrator it doesn't
  mean that they can administer particular resources! The actual power a given identity has
  with respect to administering the system is determined by the policies that manage system
  resources. For example, a channel policy might specify that `ORG1-MANUFACTURING`
  administrators have the rights to add new organizations to the channel, whereas the
  `ORG1-DISTRIBUTION` administrators have no such rights.

  Even though an X.509 certificate has a `ROLE` attribute (specifying, for example, that
  an actor is an `admin`), this refers to an actor's role within its organization
  rather than on the blockchain network. This is similar to the purpose of
  the `OU` attribute, which --- if it has been defined --- refers to an actor's place in
  the organization.

  The `ROLE` attribute **can** be used to confer administrative rights at the channel level
  if the policy for that channel has been written to allow any administrator from an organization
  (or certain organizations) permission to perform certain channel functions (such as
  instantiating chaincode). In this way, an organizational role can confer a network role.


* **Revoked Certificates:** If the identity of an actor has been revoked,
  identifying information about the identity --- not the identity itself --- is held
  in this folder. For X.509-based identities, these identifiers are pairs of strings known as
  Subject Key Identifier (SKI) and Authority Access Identifier (AKI), and are checked
  whenever the X.509 certificate is being used to make sure the certificate has not
  been revoked.

  This list is conceptually the same as a CA's Certificate Revocation List (CRL),
  but it also relates to revocation of membership from the organization. As a result,
  the administrator of an MSP, local or channel, can quickly revoke an actor or node
  from an organization by advertising the updated CRL of the CA the revoked certificate
  as issued by. This "list of lists" is optional. It will only become populated
  as certificates are revoked.


* **Node Identity:** This folder contains the identity of the node, i.e.,
  cryptographic material that --- in combination to the content of `KeyStore` --- would
  allow the node to authenticate itself in the messages that is sends to other
  participants of its channels and network. For X.509 based identities, this
  folder contains an **X.509 certificate**. This is the certificate a peer places
  in a transaction proposal response, for example, to indicate that the peer has
  endorsed it --- which can subsequently be checked against the resulting
  transaction's endorsement policy at validation time.

  This folder is mandatory for local MSPs, and there must be exactly one X.509 certificate
  for the node. It is not used for channel MSPs.


* **`KeyStore` for Private Key:** This folder is defined for the local MSP of a peer or
  orderer node (or in an client's local MSP), and contains the node's **signing key**.
  This key matches cryptographically the node's identity included in **Node Identity**
  folder and is used to sign data --- for example to sign a transaction proposal response,
  as part of the endorsement phase.

  This folder is mandatory for local MSPs, and must contain exactly one private key.
  Obviously, access to this folder must be limited only to the identities of users who have
  administrative responsibility on the peer.

  Configuration of a **channel MSPs** does not include this folder, as channel MSPs
  solely aim to offer identity validation functionalities and not signing abilities.


* **TLS Root CA:** This folder contains a list of self-signed X.509 certificates of the
  Root CAs trusted by this organization **for TLS communications**. An example of a TLS
  communication would be when a peer needs to connect to an orderer so that it can receive
  ledger updates.

  MSP TLS information relates to the nodes inside the network --- the peers and the
  orderers, in other words, rather than the applications and administrations that
  consume the network.

  There must be at least one TLS Root CA X.509 certificate in this folder.


* **TLS Intermediate CA:** This folder contains a list intermediate CA certificates
  CAs trusted by the organization represented by this MSP **for TLS communications**.
  This folder is specifically useful when commercial CAs are used for TLS certificates of an
  organization. Similar to membership intermediate CAs, specifying intermediate TLS CAs is
  optional.

  For more information about TLS, click [here](../enable_tls.html).

If you've read this doc as well as our doc on [Identity](../identity/identity.html)), you
should have a pretty good grasp of how identities and membership work in Hyperledger Fabric.
You've seen how a PKI and MSPs are used to identify the actors collaborating in a blockchain
network. You've learned how certificates, public/private keys, and roots of trust work,
in addition to how MSPs are physically and logically structured.

<!---
Licensed under Creative Commons Attribution 4.0 International License https://creativecommons.org/licenses/by/4.0/
-->
