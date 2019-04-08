# Chaincode for Operators

## What is Chaincode?

Chaincode is a program, written in [Go](https://golang.org), [Node.js](https://nodejs.org),
or [Java](https://java.com/en/) that implements a prescribed interface.
Chaincode runs in a secured Docker container isolated from the endorsing peer
process. Chaincode initializes and manages ledger state through transactions
submitted by applications.

A chaincode typically handles business logic agreed to by members of the
network, so it may be considered as a "smart contract". Ledger updates created
by a chaincode are scoped exclusively to that chaincode and can't be accessed
directly by another chaincode. However, within the same network, given the
appropriate permission a chaincode may invoke another chaincode to access
its state.

In the following sections, we will explore chaincode through the eyes of a
blockchain network operator rather than an application developer. Chaincode
operators can use this tutorial to learn how to use the Fabric chainode
lifecycle to deploy and manage chaincode on their network.

## Chaincode lifecycle

The Fabric chaincode lifecycle is a process that allows multiple organizations
to agree on how a chaincode will be operated before it can be used on a channel.
The tutorial will discuss how a chaincode operator would use the Fabric
lifecycle to perform the following tasks:

- [Install and define a chaincode](#install-and-define-a-chaincode)
- [Upgrade a chaincode](#upgrade-a-chaincode)
- [Migrate to the new Fabric lifecycle](#migrate-to-the-new-fabric-lifecycle)

*Note: The new Fabric chaincode lifecycle in the v2.0 Alpha release is not yet
feature complete. Specifically, be aware of the following limitations in the
Alpha release:*

- *Service Discovery is not yet supported*
- *Chaincodes defined with the new lifecycle are not yet discoverable
  via service discovery*

*These limitations will be resolved after the Alpha release. To use the old
lifecycle model to install and instantiate a chaincode, visit the v1.4 version
of the [Chaincode for Operators tutorial](https://hyperledger-fabric.readthedocs.io/en/release-1.4/chaincode4noah.html).*

## Install and define a chaincode

Fabric chaincode lifecycle requires that organizations agree to the parameters
that define a chaincode, such as name, version, and the chaincode endorsement
policy. Channel members come to agreement using the following four steps. Not
every organization on a channel needs to complete each step.

1. **Package the chaincode:** This step can be completed by one organization or
  by each organization.
2. **Install the chaincode on your peers:** Every organization that will use the
  chaincode to endorse a transaction or query the ledger needs to complete this
  step.
3. **Approve a chaincode definition for your organization:** Every organization
  that will use the chaincode needs to complete this step. The chaincode
  definition needs to be approved by a sufficient number of organizations
  to satisfy the channel's LifecycleEndorsment policy (a majority, by default)
  before the chaincode can be started on the channel.
4. **Commit the chaincode definition to the channel:** The commit transaction
  needs to be submitted by one organization once the required number of
  organizations on the channel have approved. The submitter first collects
  endorsements from enough peers of the organizations that have approved, and
  then submits the transaction to commit the chaincode definition.

This tutorial provides a detailed overview of the operations of the Fabric
chaincode lifecycle rather than the specific commands. To learn more about how
to use the Fabric lifecycle using the Peer CLI, see [Install and define a chaincode](build_network.html#install-define-chaincode)
in the Building your First Network Tutorial or the [peer lifecycle command reference](commands/peerlifecycle.html).
To learn more about how to use the Fabric lifecycle using the Fabric SDK for
Node.js, visit [How to install and start your chaincode](https://fabric-sdk-node.github.io/master/tutorial-chaincode-lifecycle.html).

### Step One: Packaging the smart contract

Chaincode needs to be packaged in a tar file before it can be installed on your
peers. You can package a chaincode using the Fabric peer binaries, the Node
Fabric SDK, or a third party tool such as GNU tar. When you create a chaincode
package, you need to provide a chaincode package label to create a succinct and
human readable description of the package.

If you use a third party tool to package the chaincode, the resulting file needs
to be in the format below. The Fabric peer binaries and the Fabric SDKs will
automatically create a file in this format.
- The chaincode needs to be packaged in a tar file, ending with a `.tar.gz` file
  extension.
- The tar file needs to contain two files (no directory): a metadata file
  "Chaincode-Package-Metadata.json" and another tar containing the chaincode
  files.
- "Chaincode-Package-Metadata.json" contains JSON that specifies the
  chaincode language, code path, and package label.
  You can see an example of a metadata file below:
  ```
  {"Path":"github.com/chaincode/fabcar/go","Type":"golang","Label":"fabcarv1"}
  ```

### Step Two: Install the chaincode on your peers

You need to install the chaincode package on every peer that will execute and
endorse transactions. Whether using the CLI or an SDK, you need to complete this
step using your **Peer Administrator**, whose signing certificate is in the
_admincerts_ folder of your peer MSP. It is recommended that organizations only
package a chaincode once, and then install the same package on every peer
that belongs to their org. If a channel wants to ensure that each organization
is running the same chaincode, one organization can package a chaincode and send
it to other channel members out of band.

A successful install command will return a chaincode package identifier, which
is the package label combined with a hash of the package. This package
identifier is used to associate a chaincode package installed on your peers with
a chaincode definition approved by your organization. **Save the identifier**
for next step. You can also find the package identifier by querying the packages
installed on your peer using the Peer CLI.

### Step Three: Approve a chaincode definition for your organization

The chaincode is governed by a **chaincode definition**. When channel members
approve a chaincode definition, the approval acts as a vote by an organization
on the chaincode parameters it accepts. These approved organization definitions
allow channel members to agree on a chaincode before it can be used on a channel.
The chaincode definition includes the following parameters, which need to be
consistent across organizations:

- **Name:** The name that applications will use when invoking the chaincode.
- **Version:** A version number or value associated with a given chaincodes
  package. If you upgrade the chaincode binaries, you need to change your
  chaincode version as well.
- **Sequence:** The number of times the chaincode has been defined. This value
  is an integer, and is used to keep track of chaincode upgrades. For example,
  when you first install and approve a chaincode definition, the sequence number
  will be 1. When you next upgrade the chaincode, the sequence number will be
  incremented to 2.
- **Endorsement Policy:** Which organizations need to execute and validate the
  transaction output. The endorsement policy can be expressed as a string passed
  to the CLI or the SDK, or it can reference a policy in the channel config. By
  default, the endorsement policy is set to ``Channel/Application/Endorsement``,
  which defaults to require that a majority of organizations in the channel
  endorse a transaction.
- **Collection Configuration:** The path to a private data collection definition
  file associated with your chaincode. For more information about private data
  collections, see the [Private Data architecture reference](https://hyperledger-fabric.readthedocs.io/en/master/private-data-arch.html).
- **Initialization:** All chaincode need to contain an ``Init`` function that is
  used to initialize the chaincode. By default, this function is never executed.
  However, you can use the chaincode definition to request that the ``Init``
  function be callable. If execution of ``Init`` is requested, fabric will ensure
  that ``Init`` is invoked before any other function and is only invoked once.
- **ESCC/VSCC Plugins:** The name of a custom endorsement or validation
  plugin to be used by this chaincode.

The chaincode definition also includes the **Package Identifier**. This is a
required parameter for each organization that wants to use the chaincode. The
package ID does not need to be the same for all organizations. An organization
can approve a chaincode definition without installing a chaincode package or
including the identifier in the definition.

Each channel member that wants to use the chaincode needs to approve a chaincode
definition for their organization. This approval needs to be submitted to the
ordering service, after which it is distributed to all peers. This approval
needs to be submitted by your **Organization Administrator**, whose signing
certificate is listed as an admin cert in the MSP of your organization
definition. After the approval transaction has been successfully submitted,
the approved definition is stored in a collection that is available to all
the peers of your organization. As a result you only need to approve a
chaincode for your organization once, even if you have multiple peers.

### Step Four: Commit the chaincode definition to the channel

Once a sufficient number of channel members have approved a chaincode definition,
one organization can commit the definition to the channel. You can use the
``queryapprovalstatus`` command to find which channel members have approved a
definition before committing it to the channel using the peer CLI. The commit
transaction proposal is first sent to the peers of channel members, who query the
chaincode definition approved for their organizations and endorse the definition
if their organization has approved it. The transaction is then submitted to the
ordering service, which then commits the chaincode definition to the channel.
The commit definition transaction needs to be submitted as the **Organization**
**Administrator**, whose signing certificate is listed as an admin cert in the
MSP of your organization definition.

The number of organizations that need to approve a definition before it can be
successfully committed to the channel is governed by the
``Channel/Application/LifecycleEndorsement`` policy. By default, this policy
requires that a majority of organizations in the channel endorse the transaction.
The LifecycleEndorsement policy is separate from the chaincode endorsement
policy. For example, even if a chaincode endorsement policy only requires
signatures from one or two organizations, a majority of channel members still
need to approve the chaincode definition according to the default policy. When
committing a channel definition, you need to target enough peer organizations in
the channel to satisfy your LifecycleEndorsement policy.

An organization can approve a chaincode definition without installing the
chaincode package. If an organization does not need to use the chaincode, they
can approve a chaincode definition without a package identifier to ensure that
the Lifecycle Endorsement policy is satisfied.

After the chaincode definition has been committed to the channel, channel
members can start using the chaincode. The first invoke of the chaincode will
start the chaincode containers on all of the peers targeted by the transaction
proposal, as long as those peers have installed the chaincode package. You can use
the chaincode definition to require the invocation of the ``Init`` function to start
the chaincode. Otherwise, a channel member can start the chaincode container by
invoking any transaction in the chaincode. The first invoke, whether of an
``Init`` function or other transaction, is subject to the chaincode endorsement
policy. It may take a few minutes for the chaincode container to start.

## Upgrade a chaincode

You can upgrade a chaincode using the same Fabric lifecycle process as you used
to install and start the chainocode. You can upgrade the chaincode binaries, or
only update the chaincode policies. Follow these steps to upgrade a chaincode:

1. **Repackage the chaincode:** You only need to complete this step if you are
  upgrading the chaincode binaries.
2. **Install the new chaincode package on your peers:** Once again, you only
  need to complete this step if you are upgrading the chaincode binaries.
  Installing the new chaincode package will generate a package ID, which you will
  need to pass to the new chaincode definition. You also need to change the
  chaincode version.
3. **Approve a new chaincode definition:** If you are upgrading the chaincode
  binaries, you need to update the chaincode version and the package ID in the
  chaincode definition. You can also update your chaincode endorsement policy
  without having to repackage your chaincode binaries. Channel members simply
  need to approve a definition with the new policy. The new definition needs to
  increment the **sequence** variable in the definition by one.
4. **Commit the definition to the channel:** When a sufficient number of channel
  members have approved the new chaincode definition, one organization can
  commit the new definition to upgrade the chaincode definition to the channel.
  There is no separate upgrade command as part of the lifecycle process.
5. **Upgrade the chaincode container:** If you updated the chaincode definition
  without upgrading the chaincode package, you do not need to upgrade the
  chaincode container. If you did upgrade the chaincode binaries, a new invoke
  will upgrade the chaincode container. If you requested the execution of the
  ``Init`` function in the chaincode definition, you need to upgrade the
  chaincode container by invoking the ``Init`` function again after the new
  definition is successfully committed.

The Fabric chaincode lifecycle uses the **sequence** in the chaincode definition
to keep track of upgrades. All channel members need to increment the sequence
number by one and approve a new definition to upgrade the chaincode. The version
parameter is used to track the chaincode binaries, and needs to be changed only
when you upgrade the chaincode binaries.

## Migrate to the new Fabric lifecycle

You can use the Fabric chaincode lifecycle by creating a new channel and setting
the channel capabilities to `V2_0`. You will not be able to use the previous
lifecycle to install, instantiate, or update a chaincode on a channels with
`V2_0` capabilities enabled. There is no upgrade support to the v2.0 Alpha
release, and no intended upgrade support from the the Alpha release to future
versions of v2.x.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
