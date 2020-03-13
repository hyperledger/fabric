# Endorsement policies

**Audience**: Architects, Application and smart contract developers

Endorsement policies define the smallest set of organizations that are required
to endorse a transaction in order for it to be valid. To endorse, an organization's
endorsing peer needs to run the smart contract associated with the transaction
and sign its outcome. When the ordering service sends the transaction to the
committing peers, they will each individually check whether the endorsements in
the transaction fulfill the endorsement policy. If this is not the case, the
transaction is invalidated and it will have no effect on world state.

Endorsement policies work at two different granularities: they can be set for an
entire namespace, as well as for individual state keys. They are formulated using
basic logic expressions such as `AND` and `OR`. For example, in PaperNet this
could be used as follows: the endorsement policy for a paper that has been sold
from MagnetoCorp to DigiBank could be set to `AND(MagnetoCorp.peer, DigiBank.peer)`,
requiring any changes to this paper to be endorsed by both MagnetoCorp and DigiBank.



<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
