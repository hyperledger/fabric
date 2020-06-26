Policies in Hyperledger Fabric
==============================

Configuration for a Hyperledger Fabric blockchain network is managed by
policies. These policies generally reside in the channel configuration.
The primary purpose of this document is to explain how policies are
defined in and interact with the channel configuration. However,
policies may also be specified in some other places, such as chaincodes,
so this information may be of interest outside the scope of channel
configuration.

What is a Policy?
-----------------

At its most basic level, a policy is a function which accepts as input a
set of signed data and evaluates successfully, or returns an error
because some aspect of the signed data did not satisfy the policy.

More concretely, policies test whether the signer or signers of some
data meet some condition required for those signatures to be considered
'valid'. This is useful for determining that the correct parties have
agreed to a transaction, or change.

For example a policy may define any of the following:

* Administrators from 2 out 5 possible different organizations must sign.
* Any member from any organization must sign.
* Two specific certificates must both sign.

Of course these are only examples, and other more powerful rules can be
constructed.

Policy Types
------------

There are presently two different types of policies implemented:

1. **SignaturePolicy**: This policy type is the most powerful, and
   specifies the policy as a combination of evaluation rules for MSP
   Principals. It supports arbitrary combinations of *AND*, *OR*, and
   *NOutOf*, allowing the construction of extremely powerful rules like:
   "An admin of org A and 2 other admins, or 11 of 20 org admins".
2. **ImplicitMetaPolicy**: This policy type is less flexible than
   SignaturePolicy, and is only valid in the context of configuration.
   It aggregates the result of evaluating policies deeper in the
   configuration hierarchy, which are ultimately defined by
   SignaturePolicies. It supports good default rules like "A majority of
   the organization admin policies".

Policies are encoded in a ``common.Policy`` message as defined in
``fabric-protos/common/policies.proto``. They are defined by the
following message:

::

    message Policy {
        enum PolicyType {
            UNKNOWN = 0; // Reserved to check for proper initialization
            SIGNATURE = 1;
            MSP = 2;
            IMPLICIT_META = 3;
        }
        int32 type = 1; // For outside implementors, consider the first 1000 types reserved, otherwise one of PolicyType
        bytes policy = 2;
    }

To encode the policy, simply pick the policy type of either
``SIGNATURE`` or ``IMPLICIT_META``, set it to the ``type`` field, and
marshal the corresponding policy implementation proto to ``policy``.

Configuration and Policies
--------------------------

The channel configuration is expressed as a hierarchy of configuration
groups, each of which has a set of values and policies associated with
them. For a validly configured application channel with two application
organizations and one ordering organization, the configuration looks
minimally as follows:

::

    Channel:
        Policies:
            Readers
            Writers
            Admins
        Groups:
            Orderer:
                Policies:
                    Readers
                    Writers
                    Admins
                Groups:
                    OrderingOrganization1:
                        Policies:
                            Readers
                            Writers
                            Admins
            Application:
                Policies:
                    Readers
    ----------->    Writers
                    Admins
                Groups:
                    ApplicationOrganization1:
                        Policies:
                            Readers
                            Writers
                            Admins
                    ApplicationOrganization2:
                        Policies:
                            Readers
                            Writers
                            Admins

Consider the Writers policy referred to with the ``------->`` mark in
the above example. This policy may be referred to by the shorthand
notation ``/Channel/Application/Writers``. Note that the elements
resembling directory components are group names, while the last
component resembling a file basename is the policy name.

Different components of the system will refer to these policy names. For
instance, to call ``Deliver`` on the orderer, the signature on the
request must satisfy the ``/Channel/Readers`` policy. However, to gossip
a block to a peer will require that the ``/Channel/Application/Readers``
policy be satisfied.

By setting these different policies, the system can be configured with
rich access controls.

Constructing a SignaturePolicy
------------------------------

As with all policies, the SignaturePolicy is expressed as protobuf.

::

    message SignaturePolicyEnvelope {
        int32 version = 1;
        SignaturePolicy policy = 2;
        repeated MSPPrincipal identities = 3;
    }

    message SignaturePolicy {
        message NOutOf {
            int32 N = 1;
            repeated SignaturePolicy policies = 2;
        }
        oneof Type {
            int32 signed_by = 1;
            NOutOf n_out_of = 2;
        }
    }

The outer ``SignaturePolicyEnvelope`` defines a version (currently only
``0`` is supported), a set of identities expressed as
``MSPPrincipal``\ s , and a ``policy`` which defines the policy rule,
referencing the ``identities`` by index. For more details on how to
specify MSP Principals, see the MSP Principals section.

The ``SignaturePolicy`` is a recursive data structure which either
represents a single signature requirement from a specific
``MSPPrincipal``, or a collection of ``SignaturePolicy``\ s, requiring
that ``N`` of them are satisfied.

For example:

::

    SignaturePolicyEnvelope{
        version: 0,
        policy: SignaturePolicy{
            n_out_of: NOutOf{
                N: 2,
                policies: [
                    SignaturePolicy{ signed_by: 0 },
                    SignaturePolicy{ signed_by: 1 },
                ],
            },
        },
        identities: [mspP1, mspP2],
    }

This defines a signature policy over MSP Principals ``mspP1`` and
``mspP2``. It requires both that there is a signature satisfying
``mspP1`` and a signature satisfying ``mspP2``.

As another more complex example:

::

    SignaturePolicyEnvelope{
        version: 0,
        policy: SignaturePolicy{
            n_out_of: NOutOf{
                N: 2,
                policies: [
                    SignaturePolicy{ signed_by: 0 },
                    SignaturePolicy{
                        n_out_of: NOutOf{
                            N: 1,
                            policies: [
                                SignaturePolicy{ signed_by: 1 },
                                SignaturePolicy{ signed_by: 2 },
                            ],
                        },
                    },
                ],
            },
        },
        identities: [mspP1, mspP2, mspP3],
    }

This defines a signature policy over MSP Principals ``mspP1``,
``mspP2``, and ``mspP3``. It requires one signature which satisfies
``mspP1``, and another signature which either satisfies ``mspP2`` or
``mspP3``.

Hopefully it is clear that complicated and relatively arbitrary logic
may be expressed using the SignaturePolicy policy type. For code which
constructs signature policies, consult
``fabric/common/cauthdsl/cauthdsl_builder.go``.

---------

**Limitations**: When evaluating a signature policy against a signature set,
signatures are 'consumed', in the order in which they appear, regardless of
whether they satisfy multiple policy principals.

For example.  Consider a policy which requires

::

 2 of [org1.Member, org1.Admin]

The naive intent of this policy is to require that both an admin, and a member
sign. For the signature set

::

 [org1.MemberSignature, org1.AdminSignature]

the policy evaluates to true, just as expected.  However, consider the
signature set

::

 [org1.AdminSignature, org1.MemberSignature]

This signature set does not satisfy the policy.  This failure is because when
``org1.AdminSignature`` satisfies the ``org1.Member`` role it is considered
'consumed' by the ``org1.Member`` requirement.  Because the ``org1.Admin``
principal cannot be satisfied by the ``org1.MemberSignature``, the policy
evaluates to false.

To avoid this pitfall, identities should be specified from most privileged to
least privileged in the policy identities specification, and signatures should
be ordered from least privileged to most privileged in the signature set.

MSP Principals
--------------

The MSP Principal is a generalized notion of cryptographic identity.
Although the MSP framework is designed to work with types of
cryptography other than X.509, for the purposes of this document, the
discussion will assume that the underlying MSP implementation is the
default MSP type, based on X.509 cryptography.

An MSP Principal is defined in ``fabric-protos/msp_principal.proto`` as
follows:

::

    message MSPPrincipal {

        enum Classification {
            ROLE = 0;
            ORGANIZATION_UNIT = 1;
            IDENTITY  = 2;
        }

        Classification principal_classification = 1;

        bytes principal = 2;
    }

The ``principal_classification`` must be set to either ``ROLE`` or
``IDENTITY``. The ``ORGANIZATIONAL_UNIT`` is at the time of this writing
not implemented.

In the case of ``IDENTITY`` the ``principal`` field is set to the bytes
of a certificate literal.

However, more commonly the ``ROLE`` type is used, as it allows the
principal to match many different certs issued by the MSP's certificate
authority.

In the case of ``ROLE``, the ``principal`` is a marshaled ``MSPRole``
message defined as follows:

::

   message MSPRole {
       string msp_identifier = 1;

       enum MSPRoleType {
           MEMBER = 0; // Represents an MSP Member
           ADMIN  = 1; // Represents an MSP Admin
           CLIENT = 2; // Represents an MSP Client
           PEER = 3; // Represents an MSP Peer
       }

       MSPRoleType role = 2;
   }

The ``msp_identifier`` is set to the ID of the MSP (as defined by the
``MSPConfig`` proto in the channel configuration for an org) which will
evaluate the signature, and the ``Role`` is set to either ``MEMBER``,
``ADMIN``, ``CLIENT`` or ``PEER``. In particular:

1. ``MEMBER`` matches any certificate issued by the MSP.
2. ``ADMIN`` matches certificates enumerated as admin in the MSP definition.
3. ``CLIENT`` (``PEER``) matches certificates that carry the client (peer) Organizational unit.

(see `MSP Documentation <http://hyperledger-fabric.readthedocs.io/en/latest/msp.html>`_)

Constructing an ImplicitMetaPolicy
----------------------------------

The ``ImplicitMetaPolicy`` is only validly defined in the context of
channel configuration. It is ``Implicit`` because it is constructed
implicitly based on the current configuration, and it is ``Meta``
because its evaluation is not against MSP principals, but rather against
other policies. It is defined in ``fabric-protos/common/policies.proto``
as follows:

::

    message ImplicitMetaPolicy {
        enum Rule {
            ANY = 0;      // Requires any of the sub-policies be satisfied, if no sub-policies exist, always returns true
            ALL = 1;      // Requires all of the sub-policies be satisfied
            MAJORITY = 2; // Requires a strict majority (greater than half) of the sub-policies be satisfied
        }
        string sub_policy = 1;
        Rule rule = 2;
    }

For example, consider a policy defined at ``/Channel/Readers`` as

::

    ImplicitMetaPolicy{
        rule: ANY,
        sub_policy: "foo",
    }

This policy will implicitly select the sub-groups of ``/Channel``, in
this case, ``Application`` and ``Orderer``, and retrieve the policy of
name ``foo``, to give the policies ``/Channel/Application/foo`` and
``/Channel/Orderer/foo``. Then, when the policy is evaluated, it will
check to see if ``ANY`` of those two policies evaluate without error.
Had the rule been ``ALL`` it would require both.

Consider another policy defined at ``/Channel/Application/Writers``
where there are 3 application orgs defined, ``OrgA``, ``OrgB``, and
``OrgC``.

::

    ImplicitMetaPolicy{
        rule: MAJORITY,
        sub_policy: "bar",
    }

In this case, the policies collected would be
``/Channel/Application/OrgA/bar``, ``/Channel/Application/OrgB/bar``,
and ``/Channel/Application/OrgC/bar``. Because the rule requires a
``MAJORITY``, this policy will require that 2 of the three
organization's ``bar`` policies are satisfied.

Policy Defaults
---------------

The ``configtxgen`` tool uses policies which must be specified explicitly in configtx.yaml.

Note that policies higher in the hierarchy are all defined as
``ImplicitMetaPolicy``\ s while leaf nodes necessarily are defined as
``SignaturePolicy``\ s. This set of defaults works nicely because the
``ImplicitMetaPolicies`` do not need to be redefined as the number of
organizations change, and the individual organizations may pick their
own rules and thresholds for what is means to be a Reader, Writer, and
Admin.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

