Ordering Service
================

[WIP] ...coming soon

This topic will outline the role and functionalities of the ordering
service, and explain its place in the broader network and in the lifecycle of a transaction.
The v1 architecture has been designed such that the ordering service is the centralized point 
of trust in a decentralized network, but also such that the specific implementation of "ordering" 
(solo, kafka, BFT) becomes a pluggable component.

Refer to the design document on a `Kafka-based Ordering
Service <https://docs.google.com/document/d/1vNMaM7XhOlu9tB_10dKnlrhy5d7b1u8lSY8a-kVjCO4/edit>`__
for more information on the default v1 implementation.
