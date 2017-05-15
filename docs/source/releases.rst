Release Notes
=============

`v1.0.0-alpha2 <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-alpha2>`__

The second alpha release of the v1.0.0 Hyperledger Fabric project. The code is
now feature complete. From now until the v1.0.0 release, the community is
focused on documentation improvements, testing, hardening, bug fixing and
tooling.  We will be releasing successive release candidates periodically as
the release firms up.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v100-alpha2-may-15-2017>`__

`v1.0.0-alpha <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-alpha>`__
March 16, 2017

The first alpha release of the v1.0.0 Hyperledger Fabric project. The code is
being made available to developers to begin exploring the v1.0 architecture.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v100-alpha-march-16-2017>`__

`v0.6-preview <https://github.com/hyperledger/fabric/tree/v0.6>`__
September 16, 2016

A developer preview release of the Hyperledger Fabric intended to
exercise the release logistics and stabilize a set of capabilities for
developers to try out. This will be the last release under the original
architecture. All subsequent releases will deliver on the v1.0
architecture.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v06-preview-september-16-2016>`__

`v0.5-developer-preview <https://github.com/hyperledger-archives/fabric/tree/v0.5-developer-preview>`__
June 17, 2016

A developer preview release of the Hyperledger Fabric intended to
exercise the release logistics and stabilize a set of capabilities for
developers to try out.

Key features:

Permissioned blockchain with immediate finality Chaincode (aka smart
contract) execution environments Docker container (user chaincode)
In-process with peer (system chaincode) Pluggable consensus with PBFT,
NOOPS (development mode), SIEVE (prototype) Event framework supports
pre-defined and custom events Client SDK (Node.js), basic REST APIs and
CLIs Known Key Bugs and work in progress

-  1895 - Client SDK interfaces may crash if wrong parameter specified
-  1901 - Slow response after a few hours of stress testing
-  1911 - Missing peer event listener on the client SDK
-  889 - The attributes in the TCert are not encrypted. This work is
   still on-going
