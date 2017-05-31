Endorsement policies
====================

Endorsement policies are used to instruct a peer on how to decide
whether a transaction is properly endorsed. When a peer receives a
transaction, it invokes the VSCC (Validation System Chaincode)
associated with the transaction's Chaincode as part of the transaction
validation flow to determine the validity of the transaction. Recall
that a transaction contains one or more endorsement from as many
endorsing peers. VSCC is tasked to make the following determinations: -
all endorsements are valid (i.e. they are valid signatures from valid
certificates over the expected message) - there is an appropriate number
of endorsements - endorsements come from the expected source(s)

Endorsement policies are a way of specifying the second and third
points.

Endorsement policy design
-------------------------

Endorsement policies have two main components: - a principal - a
threshold gate

A principal ``P`` identifies the entity whose signature is expected.

A threshold gate ``T`` takes two inputs: an integer ``t`` (the
threshold) and a list of ``n`` principals or gates; this gate
essentially captures the expectation that out of those ``n`` principals
or gates, ``t`` are requested to be satisfied.

For example: - ``T(2, 'A', 'B', 'C')`` requests a signature from any 2
principals out of 'A', 'B' or 'C'; - ``T(1, 'A', T(2, 'B', 'C'))``
requests either one signature from principal ``A`` or 1 signature from
``B`` and ``C`` each.

Endorsement policy syntax in the CLI
------------------------------------

In the CLI, a simple language is used to express policies in terms of
boolean expressions over principals.

A principal is described in terms of the MSP that is tasked to validate
the identity of the signer and of the role that the signer has within
that MSP. Currently, two roles are supported: **member** and **admin**.
Principals are described as ``MSP``.\ ``ROLE``, where ``MSP`` is the MSP
ID that is required, and ``ROLE`` is either one of the two strings
``member`` and ``admin``. Examples of valid principals are
``'Org0.admin'`` (any administrator of the ``Org0`` MSP) or
``'Org1.member'`` (any member of the ``Org1`` MSP).

The syntax of the language is:

``EXPR(E[, E...])``

where ``EXPR`` is either ``AND`` or ``OR``, representing the two boolean
expressions and ``E`` is either a principal (with the syntax described
above) or another nested call to ``EXPR``.

For example: - ``AND('Org1.member', 'Org2.member', 'Org3.member')``
requests 1 signature from each of the three principals -
``OR('Org1.member', 'Org2.member')`` requests 1 signature from either
one of the two principals -
``OR('Org1.member', AND('Org2.member', 'Org3.member'))`` requests either
one signature from a member of the ``Org1`` MSP or 1 signature from a
member of the ``Org2`` MSP and 1 signature from a member of the ``Org3``
MSP.

Specifying endorsement policies for a chaincode
-----------------------------------------------

Using this language, a chaincode deployer can request that the
endorsements for a chaincode be validated against the specified policy.
NOTE - the default policy requires one signature from a member of the
``DEFAULT`` MSP). This is used if a policy is not specified in the CLI.

The policy can be specified at deploy time using the ``-P`` switch,
followed by the policy.

For example:

::

    peer chaincode deploy -C testchainid -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args":["init","a","100","b","200"]}' -P "AND('Org1.member', 'Org2.member')"

This command deploys chaincode ``mycc`` on chain ``testchainid`` with
the policy ``AND('Org1.member', 'Org2.member')``.

Future enhancements
-------------------

In this section we list future enhancements for endorsement policies: -
alongside the existing way of identifying principals by their
relationship with an MSP, we plan to identify principals in terms of the
*Organization Unit (OU)* expected in their certificates; this is useful
to express policies where we request signatures from any identity
displaying a valid certificate with an OU matching the one requested in
the definition of the principal. - instead of the syntax ``AND(., .)``
we plan to move to a more intuitive syntax ``. AND .`` - we plan to
expose generalized threshold gates in the language as well alongside
``AND`` (which is the special ``n``-out-of-``n`` gate) and ``OR`` (which
is the special ``1``-out-of-``n`` gate)

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

