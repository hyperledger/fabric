# Why Fabric?

Hyperledger Fabric (hereafter referred to as Fabric) is a blockchain platform
designed to allow the exchange of an asset or the state of an asset to be consented
upon, maintained, and viewed by all parties in a permissioned group.  A key characteristic
of Fabric is that the asset is defined digitally, with all participants simply agreeing
on its representation/characterization.  As such, Fabric can support a broad range of
asset types; ranging from the tangible (real estate & hardware) to the intangible
(contracts and IP).

The technology is based on a standard blockchain concept -  a shared, replicated ledger.
However, Fabric is based on a permissioned network, meaning all participants are
required to be authenticated in order to participate and transact on
the blockchain.  Moreover, these identities can be used to govern certain levels of
access control (e.g. this user can read the ledger, but cannot exchange or transfer assets).  This
dependence on identity is a great advantage in that varying consensus algorithms (e.g.
BFT based or Kafka) can be implemented in place of the more compute-intensive Proof-of-Work and
Proof-of-Stake varieties.  As a result, permissioned networks tend to provide higher
throughput and performance.

Once an organization is granted access to the blockchain network, it then has the ability
to create and maintain a private channel with other specified members. For example,
let's assume there are four organizations trading jewels.  They may decide to use
Fabric because they trust each other, but not to an unconditional extent. They can
all agree on the business logic for trading the jewels, and can all maintain a global
ledger to view the current state of their jewel market (call this the consortium channel).
Additionally, two or more of these organizations might decide to form an alternate
private blockchain for a certain exchange that they want to keep confidential
(e.g. price X for quantity Y of asset Z).  They can perform this trade without affecting
their broader consortium channel, or, if desired, this private channel can broadcast some
level of reference data to their consortium channel.

This is powerful! This provides for great flexibility and potent capabilities, along with
the interoperability of multiple blockchain ledgers within one consortium. This is the
first of its kind and allows organizations to curate Fabric to support the myriad use
cases for different businesses and industries.  Hyperledger Fabric has already been
successfully implemented in the banking, finance, and retail industries.

We welcome you to the Hyperledger Fabric community and are glad to hear your architectural
and business needs, and help determine how Fabric can be leveraged to support your use case.
