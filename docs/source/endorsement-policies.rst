Endorsement policies
====================

Endorsement policies are used to instruct a peer on how to decide
whether a transaction is properly endorsed. When a peer receives a
transaction, it invokes the VSCC (Validation System Chaincode)
associated with the transaction's Chaincode as part of the transaction
validation flow to determine the validity of the transaction. Recall
that a transaction contains one or more endorsement from as many
endorsing peers. VSCC is tasked to make the following determinations:

  - all endorsements are valid (i.e. they are valid signatures from valid
    certificates over the expected message)
  - there is an appropriate number of endorsements
  - endorsements come from the expected source(s)

Endorsement policies are a way of specifying the second and third
points.

Endorsement policy syntax in the CLI
------------------------------------

In the CLI, a simple language is used to express policies in terms of
boolean expressions over principals.

A principal is described in terms of the MSP that is tasked to validate
the identity of the signer and of the role that the signer has within
that MSP. Four roles are supported: **member**, **admin**, **client**, and **peer**.
Principals are described as ``MSP``.\ ``ROLE``, where ``MSP`` is the MSP
ID that is required, and ``ROLE`` is one of the four strings
``member``, ``admin``, ``client`` and ``peer``. Examples of valid principals are
``'Org0.admin'`` (any administrator of the ``Org0`` MSP) or
``'Org1.member'`` (any member of the ``Org1`` MSP),
``'Org1.client'`` (any client of the ``Org1`` MSP), and
``'Org1.peer'`` (any peer of the ``Org1`` MSP).

The syntax of the language is:

``EXPR(E[, E...])``

where ``EXPR`` is either ``AND`` or ``OR``, representing the two boolean
expressions and ``E`` is either a principal (with the syntax described
above) or another nested call to ``EXPR``.

For example:
  - ``AND('Org1.member', 'Org2.member', 'Org3.member')``
    requests 1 signature from each of the three principals
  - ``OR('Org1.member', 'Org2.member')`` requests 1 signature from either
    one of the two principals
  - ``OR('Org1.member', AND('Org2.member', 'Org3.member'))``
    requests either one signature from a member of the ``Org1`` MSP
    or 1 signature from a member of the ``Org2`` MSP and 1 signature
    from a member of the ``Org3`` MSP.

Specifying endorsement policies for a chaincode
-----------------------------------------------

Using this language, a chaincode deployer can request that the
endorsements for a chaincode be validated against the specified policy.

.. note:: if not specified at instantiation time, the endorsement policy
          defaults to "any member of the organizations in the channel".
          For example, a channel with "Org1" and "Org2" would have a default
          endorsement policy of "OR('Org1.member', 'Org2.member')".

The policy can be specified at instantiate time using the ``-P`` switch,
followed by the policy.

For example:

::

    peer chaincode instantiate -C <channelid> -n mycc -P "AND('Org1.member', 'Org2.member')"

This command deploys chaincode ``mycc`` with the policy ``AND('Org1.member',
'Org2.member')`` which would require that a member of both Org1 and Org2 sign
the transaction.

Notice that, if the identity classification is enabled (see `MSP Documentation <http://hyperledger-fabric.readthedocs.io/en/latest/msp.html>`_),
one can use the PEER role to restrict endorsement to only peers.

For example:

::

    peer chaincode instantiate -C <channelid> -n mycc -P "AND('Org1.peer', 'Org2.peer')"

.. note:: A new organization added to the channel after instantiation can query a chaincode
          (provided the query has appropriate authorization as defined by channel policies
          and any application level checks enforced by the chaincode) but will not be able
          to commit a transaction endorsed by it.  The endorsement policy needs to be modified
          to allow transactions to be committed with endorsements from the new organization
          (see `Upgrade & invoke <http://hyperledger-fabric.readthedocs.io/en/latest/channel_update.html>`_).

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
