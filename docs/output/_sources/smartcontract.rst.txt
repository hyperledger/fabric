Chaincode
=========

[WIP]

The widely-used term, smart contract, is referred to as "chaincode" in
Hyperledger Fabric.

Self-executing logic that encodes the rules for specific types of
network transactions. Chaincode (currently written in Go or Java) is
installed and instantiated onto a channel's peers by an appropriately
authorized member. End users then invoke chaincode through a client-side
application that interfaces with a network peer. Chaincode runs network
transactions, which if validated, are appended to the shared ledger and
modify world state.
