## Identity Management (Membership Service)
&nbsp;
##### What is unique about the fabric's Membership Service module?
One of the things that makes the Membership Service module stand out from the pack is our implementation of the latest advances in cryptography.

In addition to ensuring private, auditable transactions, our Membership Service module introduces the concept of enrollment and transaction certificates. This innovation ensures that only verified owners can create asset tokens, allowing an infinite number of transaction certificates to be issued through parent enrollment certificates while guaranteeing the private keys of asset tokens can be regenerated if lost. 

Issuers also have the ability revoke transaction certificates or designate them to expire within a certain timeframe, allowing greater control over the asset tokens they have issued. 

Like most other modules on the fabric, you can always replace the default module with another membership service option should the need arise.


&nbsp;
##### Does its Membership Service make the fabric a centralized solution?

No. The only role of the Membership Service module is to issue digital certificates to validated entities that want to participate in the network. It does not execute transactions nor is it aware of how or when these certificates are used in any particular network.

However, because certificates are the way networks regulate and manage their users, the module serves a central regulatory and organizational role.
