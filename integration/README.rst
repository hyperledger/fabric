Integration and Feature Testing
====================================
Ginkgo
--------
Ginkgo is a package used for Behavior Driven Development (BDD) testing. Ginkgo uses Golang's existing
testing package.

Full documentation and usage examples for Ginkgo can be found in the `online documentation`_.

.. _online documentation: http://onsi.github.io/ginkgo/


BDD is an agile software development technique that encourages collaboration between developers, QA,
and non-technical or business participants in a software project. Feel free to read more about `BDD`_.

.. _BDD: https://semaphoreci.com/community/tutorials/getting-started-with-bdd-in-go-using-ginkgo


This directory contains implementations of integration and feature testing for Hyperledger Fabric.


Pre-requisites
--------------
You must have the following installed:
    * `go`_
    * `docker`_
    * `docker-compose`_

::

    $ go get github.com/onsi/ginkgo/ginkgo

Ensure that you have Docker for `Linux`_, `Mac`_ or `Windows`_ 1.12 or higher properly installed on
your machine.

.. _ginkgo: https://golang.org/
.. _docker: https://www.docker.com/
.. _docker-compose: https://docs.docker.com/compose/
.. _Linux: https://docs.docker.com/engine/installation/#supported-platforms
.. _Mac: https://docs.docker.com/engine/installation/mac/
.. _Windows: https://docs.docker.com/engine/installation/windows/


Caveats, Gotchas, and Good-To-Knows
-----------------------------------
* The tests in this repository only exercise components that originate in the Fabric repository.
* Currently, docker is only used in the ginkgo tests when using chaincode, kafka, zookeeper,
  or couchdb images.


Getting Started
---------------
The current ginkgo based tests perform some setup for you. The peer and orderer images as well as
tools such as configtxgen and cryptogen are built during the setup of the tests. The config files
are also generated based on the test setup.

Developers who add integration tests are responsible for adding and updating the necessary helper
tools and functionalities.

When adding a new test, determine if it makes more sense to mock any aspect of what you are trying
to accomplish. If you need to verify that the actual interfaces between components work as expected,
the ginkgo tooling will be the direction to go. (Hint: mocked interfaces allow more focused testing
and run faster.)

The "integration/" directory contains tests that will use multiple components from this repository.
These tests include ensuring that transactions move through the Fabric system correctly.

The "integration/chaincode" directory contains the different chaincodes that are currently available
for use in these tests.

The "integration/e2e" directory contains tests for different scenarios with different setups. If the
desired test scenario setup is the same as a test that is already defined ("Describe"), use the same
setup. If there is no need for a new network to be defined, add a new test case ("It") that can use
an existing network setup. If the setup needs to be altered, add a new setup ("Describe"). Please be
sure to add a short description of what your setup is and how it is different from the existing
setup.


==============
How to execute
==============
The integration tests can be executed by using the ``ginkgo`` command or ``go test ./...``


::

    cd integration; go test ./...

Local Execution
---------------
When executing the ginkgo tests locally, there are some simple commands that may be useful.

**Executes all tests in directory**
::

    $ ginkgo

**Executes all tests in current directory recursively**
::

    $ ginkgo -r

**Executes tests that contain the specified string from descriptions**
::

    $ ginkgo -focus <some string>

For example, `ginkgo -r -focus "2 orgs"` executes the tests with the string "2 orgs" in the
description.


Continuous Integration (CI) Execution
-------------------------------------
There is a target in the Hyperledger Fabric Makefile for executing `integration`_ tests.

.. _integration: https://jenkins.hyperledger.org/view/fabric/job/fabric-verify-integration-tests-x86_64

To execute the integration tests using the Makefile, execute the following:

::

    make integration-tests


============
Contributing
============
There are different ways to contribute in this area.
 * Writing helper functions
 * Writing test code

To add your contributions to the Hyperledger Fabric project, please refer to the
`Hyperledger Fabric Contribution`_ page for more details.

.. _Hyperledger Fabric Contribution: http://hyperledger-fabric.readthedocs.io/en/latest/CONTRIBUTING.html


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
