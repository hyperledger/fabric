Getting Started
===============

.. toctree::
   :maxdepth: 1
   :hidden:

   prereqs
   install
   test_network

Before we begin, if you haven't already done so, you may wish to check that
you have all the :doc:`prereqs` installed on the platform(s)
on which you'll be developing blockchain applications and/or operating
Hyperledger Fabric.

Once you have the prerequisites installed, you are ready to download and
install HyperLedger Fabric. While we work on developing real installers for the
Fabric binaries, we provide a script that will :doc:`install` to your system.
The script also will download the Docker images to your local registry.

After you have downloaded the Fabric Samples and Docker images to your local
machine, you can get started working with Fabric with the
:doc:`test_network` tutorial.

Hyperledger Fabric smart contract (chaincode) APIs
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Hyperledger Fabric offers a number of APIs to support developing smart contracts (chaincode)
in various programming languages. Smart contract APIs are available for Go, Node.js, and Java:

  * `Go contract-api <https://github.com/hyperledger/fabric-contract-api-go>`__.
  * `Node.js contract API <https://github.com/hyperledger/fabric-chaincode-node>`__ and `Node.js contract API documentation <https://hyperledger.github.io/fabric-chaincode-node/>`__.
  * `Java contract API <https://github.com/hyperledger/fabric-chaincode-java>`__ and `Java contract API documentation <https://hyperledger.github.io/fabric-chaincode-java/>`__.

Hyperledger Fabric application SDKs
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Hyperledger Fabric offers a number of SDKs to support developing applications
in various programming languages. SDKs are available for Node.js and Java:

  * `Node.js SDK <https://github.com/hyperledger/fabric-sdk-node>`__ and `Node.js SDK documentation <https://hyperledger.github.io/fabric-sdk-node/>`__.
  * `Java SDK <https://github.com/hyperledger/fabric-gateway-java>`__ and `Java SDK documentation <https://hyperledger.github.io/fabric-gateway-java/>`__.

In addition, there are two more application SDKs that have not yet been officially released
(for Python and Go), but they are still available for downloading and testing:

  * `Python SDK <https://github.com/hyperledger/fabric-sdk-py>`__.
  * `Go SDK <https://github.com/hyperledger/fabric-sdk-go>`__.

Currently, Node.js and Java support the new application programming model delivered in
Hyperledger Fabric v1.4. Support for Go is planned to be delivered in a later release.

Hyperledger Fabric CA
^^^^^^^^^^^^^^^^^^^^^

Hyperledger Fabric provides an optional
`certificate authority service <http://hyperledger-fabric-ca.readthedocs.io/en/latest>`_
that you may choose to use to generate the certificates and key material
to configure and manage identity in your blockchain network. However, any CA
that can generate ECDSA certificates may be used.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
