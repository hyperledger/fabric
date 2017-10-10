Chaincode Packaging and Signing
===============================

Introduction
------------

A chaincode will be placed on the file system of the peer on
installation simply as a file with name
``<chaincode name>.<chaincode version``. The contents of that file is
called a chaincode package.

This document describes how a chaincode package can be created and
signed from CLI. It also describes how the ``install`` command can
be used to install the chaincode package.

What’s in the package ?
-----------------------

The package consists of 3 parts \* the chaincode as defined by
``ChaincodeDeploymentSpec``. This defines the code and other meta
properties such as name and version \* an instantiation policy which can
be syntactically described by the same policy used for endorsement and
described in ``endorsement-policies.rst`` \* a set of signatures by the
entities that “own” the chaincode.

The signatures serve the following purposes \* establish an ownership of
the chaincode \* allows verification that the signatures are over the
same content \* allows detection of package tampering

The creator of the instantiation of the chaincode on a channel is
validated against the instantiation policy of the chaincode.

Chaincode Packaging
-------------------

The package is created and signed using the command

::

    peer chaincode package -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -v 0 -s -S -i "AND('OrgA.admin')" ccpack.out

where ``-s`` specifies creating the package as opposed to generating raw
ChaincodeDeploymentSpec ``-S`` specifies instructs to sign the package
using the Local MSP (as defined by ``localMspid`` property in
``core.yaml``)

The ``-S`` option is optional. However if a package is created without a
signature, it cannot be signed by any other owner using the
``signpackage`` command in the next section.

The ``-i`` option is optional. It allows specifying an instantiation policy
for the chaincode. The instantiation policy has the same format as an
endorsement policy and specifies who can instantiate the chaincode. In the
example above, only the admin of OrgA is allowed to instantiate the chaincode.
If no policy is provided, the default policy is used, which only allows the
admin of the peer's MSP to instantiate chaincode.

Package signing
---------------

A package can be handed over to other owners for inspection and signing.
The workflow supports out of band signing of package.

A previously created package can be signed using the command

::

    peer chaincode signpackage ccpack.out signedccpack.out

where ``ccpack.out`` and ``signedccpack.out`` are input and output
packages respectively. ``signedccpack.out`` contains an additional
signature over the package signed using the Local MSP.

Installing the package
----------------------
The package can be installed using the ``install`` command as follows

::

    peer chaincode install ccpack.out

where ``ccpack.out`` is a package filecreated using the ``package``
or ``signedpackage`` commands.

Conclusion
----------

The peer will support use of both raw ChaincodeDeploymentSpec and the
package structure described in this document. This will allow existing
commands and workflows to work which is especially useful in development
and test phases.
