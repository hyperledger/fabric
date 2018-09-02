# Access Control Lists (ACL)

## What is an Access Control List?

*Note: This topic deals with access control and policies on a channel
administration level. To learn about access control within a chaincode, check out
our [chaincode for developers tutorial](./chaincode4ade.html#Chaincode_API).*

Fabric uses access control lists (ACLs) to manage access to resources by associating
a **policy** --- which specifies a rule that evaluates to true or false, given a set
of identities --- with the resource. Fabric contains a number of default ACLs. In this
document, we'll talk about how they're formatted and how the defaults can be overridden.

But before we can do that, it's necessary to understand a little about resources
and policies.

### Resources

Users interact with Fabric by targeting a [user chaincode](./chaincode4ade.html),
[system chaincode](./chaincode4noah.html), or an [events stream source](./peer_event_services.html).
As such, these endpoints are considered "resources" on which access control should be
exercised.

Application developers need to be aware of these resources and the default
policies associated with them. The complete list of these resources are found in
`configtx.yaml`. You can look at a [sample `configtx.yaml` file here](http://github.com/hyperledger/fabric/blob/release-1.2/sampleconfig/configtx.yaml).

The resources named in `configtx.yaml` is an exhaustive list of all internal resources
currently defined by Fabric. The loose convention adopted there is `<component>/<resource>`.
So `cscc/GetConfigBlock` is the resource for the `GetConfigBlock` call in the `CSCC`
component.

### Policies

Policies are fundamental to the way Fabric works because they allow the identity
(or set of identities) associated with a request to be checked against the policy
associated with the resource needed to fulfill the request. Endorsement policies
are used to determine whether a transaction has been appropriately endorsed. The
policies defined in the channel configuration are referenced as modification policies
as well as for access control, and are defined in the channel configuration itself.

Policies can be structured in one of two ways: as `Signature` policies or as an
`ImplicitMeta` policy.

#### `Signature` policies

These policies identify specific users who must sign in order for a policy
to be satisfied. For example:

```
Policies:
  MyPolicy:
    Type: Signature
    Rule: “Org1.Peer OR Org2.Peer”

```

This policy construct can be interpreted as: *the policy named `MyPolicy` can
only be satisfied by the signature of an identity with role of "a peer from
Org1" or "a peer from Org2"*.

Signature policies support arbitrary combinations of `AND`, `OR`, and `NOutOf`,
allowing the construction of extremely powerful rules like: "An admin of org A
and two other admins, or 11 of 20 org admins".

#### `ImplicitMeta` policies

`ImplicitMeta` policies aggregate the result of policies deeper in the
configuration hierarchy that are ultimately defined by `Signature` policies. They
support default rules like "A majority of the organization admins". These policies
use a different but still very simple syntax as compared to `Signature` policies:
`<ALL|ANY|MAJORITY> <sub_policy>`.

For example: `ANY` `Readers` or `MAJORITY` `Admins`.

*Note that in the default policy configuration `Admins` have an operational role.
Policies that specify that only Admins --- or some subset of Admins --- have access
to a resource will tend to be for sensitive or operational aspects of the network
(such as instantiating chaincode on a channel). `Writers` will tend to be able to
propose ledger updates, such as a transaction, but will not typically have
administrative permissions. `Readers` have a passive role. They can access
information but do not have the permission to propose ledger updates nor do can
they perform administrative tasks. These default policies can be added to,
edited, or supplemented, for example by the new `peer` and `client` roles (if you
have `NodeOU` support).*

Here's an example of an `ImplicitMeta` policy structure:

```
Policies:
  AnotherPolicy:
    Type: ImplicitMeta
    Rule: "MAJORITY Admins"
```

Here, the policy `AnotherPolicy` can be satisfied by the `MAJORITY` of `Admins`,
where `Admins` is eventually being specified by lower level `Signature` policy.

### Where is access control specified?

Access control defaults exist inside `configtx.yaml`, the file that `configtxgen`
uses to build channel configurations.

Access control can be updated one of two ways, either by editing `configtx.yaml`
itself, which will propagate the ACL change to any new channels, or by updating
access control in the channel configuration of a particular channel.

## How ACLs are formatted in `configtx.yaml`

ACLs are formatted as a key-value pair consisting of a resource function name
followed by a string. To see what this looks like, reference this [sample configtx.yaml file](https://github.com/hyperledger/fabric/blob/release-1.2/sampleconfig/configtx.yaml).

Two excerpts from this sample:

```
# ACL policy for invoking chaincodes on peer
peer/Propose: /Channel/Application/Writers
```

```
# ACL policy for sending block events
event/Block: /Channel/Application/Readers
```

These ACLs define that access to `peer/Propose` and `event/Block` resources
is restricted to identities satisfying the policy defined at the canonical path
`/Channel/Application/Writers` and `/Channel/Application/Readers`, respectively.

### Updating ACL defaults in `configtx.yaml`

In cases where it will be necessary to override ACL defaults when bootstrapping
a network, or to change the ACLs before a channel has been bootstrapped, the
best practice will be to update `configtx.yaml`.

Let's say you want to modify the `peer/Propose` ACL default --- which specifies
the policy for invoking chaincodes on a peer -- from `/Channel/Application/Writers`
to a policy called `MyPolicy`.

This is done by adding a policy called `MyPolicy` (it could be called anything,
but for this example we'll call it `MyPolicy`). The policy is defined in the
`Application.Policies` section inside `configtx.yaml` and specifies a rule to be
checked to grant or deny access to a user. For this example, we'll be creating a
`Signature` policy identifying `SampleOrg.admin`.

```
Policies: &ApplicationDefaultPolicies
    Readers:
        Type: ImplicitMeta
        Rule: "ANY Readers"
    Writers:
        Type: ImplicitMeta
        Rule: "ANY Writers"
    Admins:
        Type: ImplicitMeta
        Rule: "MAJORITY Admins"
    MyPolicy:
        Type: Signature
        Rule: "OR('SampleOrg.admin')"
```

Then, edit the `Application: ACLs` section inside `configtx.yaml` to change
`peer/Propose` from this:

`peer/Propose: /Channel/Application/Writers`

To this:

`peer/Propose: /Channel/Application/MyPolicy`

Once these fields have been changed in `configtx.yaml`, the `configtxgen` tool
will use the policies and ACLs defined when creating a channel creation
transaction. When appropriately signed and submitted by one of the admins of the
consortium members, a new channel with the defined ACLs and policies is created.

Once `MyPolicy` has been bootstrapped into the channel configuration, it can also
be referenced to override other ACL defaults. For example:

```
SampleSingleMSPChannel:
    Consortium: SampleConsortium
    Application:
        <<: *ApplicationDefaults
        ACLs:
            <<: *ACLsDefault
            event/Block: /Channel/Application/MyPolicy
```

This would restrict the ability to subscribe to block events to `SampleOrg.admin`.

If channels have already been created that want to use this ACL, they'll have
to update their channel configurations one at a time using the following flow:

### Updating ACL defaults in the channel config

If channels have already been created that want to use `MyPolicy` to restrict
access to `peer/Propose` --- or if they want to create ACLs they don't want
other channels to know about --- they'll have to update their channel
configurations one at a time through config update transactions.

*Note: Channel configuration transactions are an involved process we won't
delve into here. If you want to read more about them check out our document on
[channel configuration updates](./config_update.html) and our ["Adding an Org to a Channel" tutorial](./channel_update_tutorial.html).*

After pulling, translating, and stripping the configuration block of its metadata,
you would edit the configuration by adding `MyPolicy` under `Application: policies`,
where the `Admins`, `Writers`, and `Readers` policies already live.

```
"MyPolicy": {
  "mod_policy": "Admins",
  "policy": {
    "type": 1,
    "value": {
      "identities": [
        {
          "principal": {
            "msp_identifier": "SampleOrg",
            "role": "ADMIN"
          },
          "principal_classification": "ROLE"
        }
      ],
      "rule": {
        "n_out_of": {
          "n": 1,
          "rules": [
            {
              "signed_by": 0
            }
          ]
        }
      },
      "version": 0
    }
  },
  "version": "0"
},
```

Note in particular the `msp_identifer` and `role` here.

Then, in the ACLs section of the config, change the `peer/Propose` ACL from
this:

```
"peer/Propose": {
  "policy_ref": "/Channel/Application/Writers"
```

To this:

```
"peer/Propose": {
  "policy_ref": "/Channel/Application/MyPolicy"
```

Note: If you do not have ACLs defined in your channel configuration, you will
have to add the entire ACL structure.

Once the configuration has been updated, it will need to be submitted by the
usual channel update process.

### Satisfying an ACL that requires access to multiple resources

If a member makes a request that calls multiple system chaincodes, all of the ACLs
for those system chaincodes must be satisfied.

For example, `peer/Propose` refers to any proposal request on a channel. If the
particular proposal requires access to two system chaincodes that requires an
identity satisfying `Writers` and one system chaincode that requires an identity
satisfying `MyPolicy`, then the member submitting the proposal must have an identity
that evaluates to "true" for both `Writers` and `MyPolicy`.

In the default configuration, `Writers` is a signature policy whose `rule` is
`SampleOrg.member`. In other words, "any member of my organization". `MyPolicy`,
listed above, has a rule of `SampleOrg.admin`, or "any admin of my organization".
To satisfy these ACLs, the member would have to be both an administrator and a
member of `SampleOrg`. By default, all administrators are members (though not all
administrators are members), but it is possible to overwrite these policies to
whatever you want them to be. As a result, it's important to keep track of these
policies to ensure that the ACLs for peer proposals are not impossible to satisfy
(unless that is the intention).

#### Migration considerations for customers using the experimental ACL feature

Previously, the management of access control lists was done in an `isolated_data`
section of the channel creation transaction and updated via `PEER_RESOURCE_UPDATE`
transactions. Originally, it was thought that the `resources` tree would handle the
update of several functions that, ultimately, were handled in other ways, so
maintaining a separate parallel peer configuration tree was judged to be unnecessary.

Migration for customers using the experimental resources tree in v1.1 is possible.
Because the official v1.2 release does not support the old ACL methods, the network
operators should shut down all their peers.  Then, they should upgrade them to v1.2,
submit a channel reconfiguration transaction which enables the v1.2 capability and
sets the desired ACLs, and then finally restart the upgraded peers.  The restarted
peers will immediately consume the new channel configuration and enforce the ACLs as
desired.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
