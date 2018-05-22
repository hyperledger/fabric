System Chaincode Plugins
========================

System chaincodes are specialized chaincodes that run as part of the peer process
as opposed to user chaincodes that run in separate docker containers. As
such they have more access to resources in the peer and can be used for
implementing features that are difficult or impossible to be implemented through
user chaincodes. Examples of System Chaincodes include QSCC (Query System Chaincode)
for ledger and other Fabric-related queries, CSCC (Configuration System Chaincode)
which helps regulate access control, and LSCC (Lifecycle System Chaincode).

Unlike a user chaincode, a system chaincode is not installed and instantiated
using proposals from SDKs or CLI. It is registered and deployed by the peer at
start-up.

System chaincodes can be linked to a peer in two ways: statically, and dynamically
using Go plugins. This tutorial will outline how to develop and load system chaincodes
as plugins.

Developing Plugins
------------------

A system chaincode is a program written in `Go <https://golang.org>`_ and loaded
using the Go `plugin <https://golang.org/pkg/plugin>`_ package.

A plugin includes a main package with exported symbols and is built with the command
``go build -buildmode=plugin``.

Every system chaincode must implement the `Chaincode Interface <https://godoc.org/github.com/hyperledger/fabric/core/chaincode/shim#Chaincode>`_
and export a constructor method that matches the signature ``func New() shim.Chaincode``
in the main package. An example can be found in the repository at ``examples/plugin/scc``.

Existing chaincodes such as the QSCC can also serve as templates for certain
features, such as access control, that are typically implemented through
system chaincodes. The existing system chaincodes also serve as a reference for
best-practices on things like logging and testing.

.. note:: On imported packages: the Go standard library requires that a plugin must
          include the same version of imported packages as the host application
          (Fabric, in this case).

Configuring Plugins
-------------------

Plugins are configured in the ``chaincode.systemPlugin`` section in ``core.yaml``:

.. code-block:: bash

  chaincode:
    systemPlugins:
      - enabled: true
        name: mysyscc
        path: /opt/lib/syscc.so
        invokableExternal: true
        invokableCC2CC: true


A system chaincode must also be whitelisted in the ``chaincode.system`` section
in ``core.yaml``:

.. code-block:: bash

  chaincode:
    system:
      mysyscc: enable


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
