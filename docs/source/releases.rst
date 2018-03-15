Release Notes
=============

`v1.1.0 <https://github.com/hyperledger/fabric/releases/tag/v1.1.0>`__ - March 15, 2018
---------------------------------------------------------------------------------------
The v1.1 release includes all of the features delivered in v1.1.0-preview
and v1.1.0-alpha.

Additionally, there are feature improvements, bug fixes, documentation and test
coverage improvements, UX improvements based on user feedback and changes to address a
variety of static scan findings (unused code, static security scanning, spelling,
linting and more).

Updated to Go version 1.9.2.
Updated baseimage version to 0.4.6.

Known Vulnerabilities
---------------------
none

Resolved Vulnerabilities
------------------------
https://jira.hyperledger.org/browse/FAB-4824
https://jira.hyperledger.org/browse/FAB-5406

Known Issues & Workarounds
--------------------------
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to `FAB-5177 <https://jira.hyperledger.org/browse/FAB-5177>`__ for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v110>`__

`v1.1.0-rc1 <https://github.com/hyperledger/fabric/releases/tag/v1.1.0-rc1>`__ - March 1, 2018
----------------------------------------------------------------------------------------------
The v1.1 release candidate 1 (rc1) includes all of the features delivered in v1.1.0-preview
and v1.1.0-alpha.

  Additionally, there are feature improvements, bug fixes, documentation and test
  coverage improvements, UX improvements based on user feedback and changes to address a
  variety of static scan findings (unused code, static security scanning, spelling,
  linting and more).

Known Vulnerabilities
---------------------
none

Resolved Vulnerabilities
------------------------
none

Known Issues & Workarounds
--------------------------
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to `FAB-5177 <https://jira.hyperledger.org/browse/FAB-5177>`__ for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v110-rc1>`__

`v1.1.0-alpha <https://github.com/hyperledger/fabric/releases/tag/v1.1.0-alpha>`__ - January 25, 2018
-----------------------------------------------------------------------------------------------------
This is a feature-complete *alpha* release of the up-coming 1.1 release. The 1.1 release
includes the following new major features:

  - `FAB-6911 <https://jira.hyperledger.org/browse/FAB-6911>`__ - Event service for blocks
  - `FAB-5481 <https://jira.hyperledger.org/browse/FAB-5481>`__ - Event service for block transaction events
  - `FAB-5300 <https://jira.hyperledger.org/browse/FAB-5300>`__ - Certificate Revocation List from CA
  - `FAB-3067 <https://jira.hyperledger.org/browse/FAB-3067>`__ - Peer management of CouchDB indexes
  - `FAB-6715 <https://jira.hyperledger.org/browse/FAB-6715>`__ - Mutual TLS between all components
  - `FAB-5556 <https://jira.hyperledger.org/browse/FAB-5556>`__ - Rolling Upgrade via configured capabilities
  - `FAB-2331 <https://jira.hyperledger.org/browse/FAB-2331>`__ - Node.js Chaincode support
  - `FAB-5363 <https://jira.hyperledger.org/browse/FAB-5363>`__ - Node.js SDK Connection Profile
  - `FAB-830 <https://jira.hyperledger.org/browse/FAB-830>`__ - Encryption library for chaincode
  - `FAB-5346 <https://jira.hyperledger.org/browse/FAB-5346>`__ - Attribute-based Access Control
  - `FAB-6089 <https://jira.hyperledger.org/browse/FAB-6089>`__ - Chaincode APIs for creator identity
  - `FAB-6421 <https://jira.hyperledger.org/browse/FAB-6421>`__ - Performance improvements

  Additionally, there are feature improvements, bug fixes, documentation and test
  coverage improvements, UX improvements based on user feedback and changes to address a
  variety of static scan findings (unused code, static security scanning, spelling,
  linting and more).

Known Vulnerabilities
---------------------
none

Resolved Vulnerabilities
------------------------
none

Known Issues & Workarounds
--------------------------
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to `FAB-5177 <https://jira.hyperledger.org/browse/FAB-5177>`__ for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v110-alpha>`__

`v1.1.0-preview <https://github.com/hyperledger/fabric/releases/tag/v1.1.0-preview>`__ - November 1, 2017
---------------------------------------------------------------------------------------------------------
This is a *preview* release of the up-coming 1.1 release. We are not feature
complete for 1.1 just yet, but we wanted to get the following functionality
published to gain some early community feedback on the following features:

  - `FAB-2331 <https://jira.hyperledger.org/browse/FAB-2331>`__ - Node.js Chaincode
  - `FAB-5363 <https://jira.hyperledger.org/browse/FAB-5363>`__ - Node.js SDK Connection Profile
  - `FAB-830 <https://jira.hyperledger.org/browse/FAB-830>`__ - Encryption library for chaincode
  - `FAB-5346 <https://jira.hyperledger.org/browse/FAB-5346>`__ - Attribute-based Access Control
  - `FAB-6089 <https://jira.hyperledger.org/browse/FAB-6089>`__ - Chaincode APIs to retrieve creator cert info
  - `FAB-6421 <https://jira.hyperledger.org/browse/FAB-6421>`__ - Performance improvements

Additionally, there are the usual bug fixes, documentation and test coverage
improvements, UX improvements based on user feedback and changes to address a
variety of static scan findings (unused code, static security scanning, spelling,
linting and more).

Known Vulnerabilities
---------------------
none

Resolved Vulnerabilities
------------------------
none

Known Issues & Workarounds
--------------------------
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to `FAB-5177 <https://jira.hyperledger.org/browse/FAB-5177>`__ for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v110-preview>`__

`v1.0.4 <https://github.com/hyperledger/fabric/releases/tag/v1.0.4>`__ - October 31, 2017
-----------------------------------------------------------------------------------------
Bug fixes, documentation and test coverage improvements, UX improvements
based on user feedback and changes to address a variety of static scan
findings (unused code, static security scanning, spelling, linting and more).

Known Vulnerabilities
---------------------
none

Resolved Vulnerabilities
------------------------
none

Known Issues & Workarounds
--------------------------
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to https://jira.hyperledger.org/browse/FAB-5177 for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/v1.0.4/CHANGELOG.md#v104>`__

`v1.0.3 <https://github.com/hyperledger/fabric/releases/tag/v1.0.3>`__ - October 3, 2017
----------------------------------------------------------------------------------------

Bug fixes, documentation and test coverage improvements, UX improvements
based on user feedback and changes to address a variety of static scan
findings (unused code, static security scanning, spelling, linting and more).

Known Vulnerabilities
none

Resolved Vulnerabilities
none

Known Issues & Workarounds
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to https://jira.hyperledger.org/browse/FAB-5177 for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v103>`__

`v1.0.2 <https://github.com/hyperledger/fabric/releases/tag/v1.0.2>`__ - August 31, 2017
----------------------------------------------------------------------------------------

Bug fixes, documentation and test coverage improvements, UX improvements
based on user feedback and changes to address a variety of static scan
findings (unused code, static security scanning, spelling, linting and more).

Known Vulnerabilities
none

Resolved Vulnerabilities
https://jira.hyperledger.org/browse/FAB-5753
https://jira.hyperledger.org/browse/FAB-5899

Known Issues & Workarounds
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to https://jira.hyperledger.org/browse/FAB-5177 for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v102>`__

`v1.0.1 <https://github.com/hyperledger/fabric/releases/tag/v1.0.1>`__ - August 5, 2017
---------------------------------------------------------------------------------------

Bug fixes, documentation and test coverage improvements, UX improvements
based on user feedback and changes to address a variety of static scan
findings (unused code, static security scanning, spelling, linting and more).

Known Vulnerabilities
none

Resolved Vulnerabilities
https://jira.hyperledger.org/browse/FAB-5329
https://jira.hyperledger.org/browse/FAB-5330
https://jira.hyperledger.org/browse/FAB-5353
https://jira.hyperledger.org/browse/FAB-5529
https://jira.hyperledger.org/browse/FAB-5606
https://jira.hyperledger.org/browse/FAB-5627

Known Issues & Workarounds
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to https://jira.hyperledger.org/browse/FAB-5177 for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v101>`__

`v1.0.0 <https://github.com/hyperledger/fabric/releases/tag/v1.0.0>`__ - July 11, 2017
--------------------------------------------------------------------------------------

Bug fixes, documentation and test coverage improvements, UX improvements
based on user feedback and changes to address a variety of static scan
findings (removal of unused code, static security scanning, spelling, linting
and more).

Known Vulnerabilities
none

Resolved Vulnerabilities
https://jira.hyperledger.org/browse/FAB-5207

Known Issues & Workarounds
The fabric-ccenv image which is used to build chaincode, currently includes
the github.com/hyperledger/fabric/core/chaincode/shim ("shim") package.
This is convenient, as it provides the ability to package chaincode
without the need to include the "shim". However, this may cause issues in future
releases (and/or when trying to use packages which are included by the "shim").

In order to avoid any issues, users are advised to manually vendor the "shim"
package with their chaincode prior to using the peer CLI for packaging and/or
for installing chaincode.

Please refer to https://jira.hyperledger.org/browse/FAB-5177 for more details,
and kindly be aware that given the above, we may end up changing the
fabric-ccenv in the future.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v100>`__

`v1.0.0-rc1 <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-rc1>`__ - June 23, 2017
----------------------------------------------------------------------------------------------

Bug fixes, documentation and test coverage improvements, UX improvements
based on user feedback and changes to address a variety of static scan
findings (unused code, static security scanning, spelling, linting and more).

Known Vulnerabilities
none

Resolved Vulnerabilities
https://jira.hyperledger.org/browse/FAB-4856
https://jira.hyperledger.org/browse/FAB-4848
https://jira.hyperledger.org/browse/FAB-4751
https://jira.hyperledger.org/browse/FAB-4626
https://jira.hyperledger.org/browse/FAB-4567
https://jira.hyperledger.org/browse/FAB-3715

Known Issues & Workarounds
none

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v100-rc1>`__

`v1.0.0-beta <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-beta>`__ - June 8, 2017
-----------------------------------------------------------------------------------------------

Bug fixes, documentation and test coverage improvements, UX improvements based
on user feedback and changes to address a variety of static scan findings (unused
code, static security scanning, spelling, linting and more).

Upgraded to `latest version <https://github.com/grpc/grpc-go/releases/>`__ (a
precursor to 1.4.0) of gRPC-go and implemented keep-alive feature for improved
resiliency.

Added a `new tool <https://github.com/hyperledger/fabric/tree/master/examples/configtxupdate>`__
`configtxlator` to enable users to translate the contents of a channel configuration transaction
into a human readable form.

Known Vulnerabilities

none

Resolved Vulnerabilities

none

Known Issues & Workarounds

BCCSP content in the configtx.yaml has been `removed <https://github.com/hyperledger/fabric/commit/a997c30>`__. This change will cause a panic when running `configtxgen` tool with a configtx.yaml file that includes the removed BCCSP content.

Java Chaincode support has been disabled until post 1.0.0 as it is not yet fully mature. It may be re-enabled for experimentation by cloning the hyperledger/fabric repository, reversing `this commit <https://github.com/hyperledger/fabric/commit/29e0c40>`__ and building your own fork.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v100-beta>`__

`v1.0.0-alpha2 <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-alpha2>`__
------------------------------------------------------------------------------------

The second alpha release of the v1.0.0 Hyperledger Fabric. The code is
now feature complete. From now until the v1.0.0 release, the community is
focused on documentation improvements, testing, hardening, bug fixing and
tooling.  We will be releasing successive release candidates periodically as
the release firms up.

`Change Log <https://github.com/hyperledger/fabric/blob/master/CHANGELOG.md#v100-alpha2-may-15-2017>`__

`v1.0.0-alpha <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-alpha>`__ - March 16, 2017
---------------------------------------------------------------------------------------------------

The first alpha release of the v1.0.0 Hyperledger Fabric. The code is
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

`v0.5-developer-preview <https://github.com/hyperledger-archives/fabric/tree/v0.5-developer-preview>`__ - June 17, 2016
-----------------------------------------------------------------------------------------------------------------------

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

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
