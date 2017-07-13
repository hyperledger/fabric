Why Hyperledger Fabric?
=======================

Hyperledger Fabric is delivering a blockchain platform
designed to allow the exchange of an asset or the state of an asset to
be consented upon, maintained, and viewed by all parties in a
permissioned group. A key characteristic of Hyperledger Fabric is that
the asset is defined digitally, with all participants simply agreeing on
its representation/characterization. As such, Hyperledger Fabric can
support a broad range of asset types; ranging from the tangible (real
estate and hardware) to the intangible (contracts and intellectual
property).

The technology is based on a standard blockchain concept - a shared,
replicated ledger. However, Hyperledger Fabric is based on a
`permissioned network <glossary.md#permissioned-network>`__, meaning all
participants are required to be authenticated in order to participate
and transact on the blockchain. Moreover, these identities can be used
to govern certain levels of access control (e.g. this user can read the
ledger, but cannot exchange or transfer assets). This dependence on
identity is a great advantage in that varying consensus algorithms (e.g.
byzantine or crash fault tolerant) can be implemented in place of the
more compute-intensive Proof-of-Work and Proof-of-Stake varieties. As a
result, permissioned networks tend to provide higher transaction
throughput rates and performance.

Once an organization is granted access to the `blockchain
network <glossary.md#blockchain-network>`__, it then has the ability to
create and maintain a private `channel <glossary.md#channel>`__ with
other specified members. For example, let's assume there are four
organizations trading jewels. They may decide to use Hyperledger Fabric
because they trust each other, but not to an unconditional extent. They
can all agree on the business logic for trading the jewels, and can all
maintain a global ledger to view the current state of their jewel market
(call this the consortium channel). Additionally, two or more of these
organizations might decide to form an alternate private blockchain for a
certain exchange that they want to keep confidential (e.g. price X for
quantity Y of asset Z). They can perform this trade without affecting
their broader consortium channel, or, if desired, this private channel
can broadcast some level of reference data to their consortium channel.

This is powerful! This provides for great flexibility and potent
capabilities, along with the interoperability of multiple blockchain
ledgers within one consortium. This is the first of its kind and allows
organizations to curate Hyperledger Fabric to support the myriad use
cases for different businesses and industries. Hyperledger Fabric has
already been successfully implemented in the banking, finance, and
retail industries.

We welcome you to the Hyperledger Fabric community and are keen to learn
of your architectural and business requirements, and help determine how
Hyperledger Fabric can be leveraged to support your use cases.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

