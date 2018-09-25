Building Hyperledger Fabric
---------------------------

The following instructions assume that you have already set up your
:doc:`development environment <devenv>`.

To build Hyperledger Fabric:

::

    cd $GOPATH/src/github.com/hyperledger/fabric
    make dist-clean all

Running the unit tests
~~~~~~~~~~~~~~~~~~~~~~

Use the following sequence to run all unit tests

::

    cd $GOPATH/src/github.com/hyperledger/fabric
    make unit-test

To run a subset of tests, set the TEST_PKGS environment variable.
Specify a list of packages (separated by space), for example:

::

    export TEST_PKGS="github.com/hyperledger/fabric/core/ledger/..."
    make unit-test

To run a specific test use the ``-run RE`` flag where RE is a regular
expression that matches the test case name. To run tests with verbose
output use the ``-v`` flag. For example, to run the ``TestGetFoo`` test
case, change to the directory containing the ``foo_test.go`` and
call/execute

::

    go test -v -run=TestGetFoo



Running Node.js Client SDK Unit Tests
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

You must also run the Node.js unit tests to ensure that the Node.js
client SDK is not broken by your changes. To run the Node.js unit tests,
follow the instructions
`here <https://github.com/hyperledger/fabric-sdk-node/blob/master/README.md>`__.

Building outside of Vagrant
---------------------------

It is possible to build the project and run peers outside of Vagrant.
Generally speaking, one has to 'translate' the vagrant `setup
file <https://github.com/hyperledger/fabric/blob/master/devenv/setup.sh>`__
to the platform of your choice.

Building on Z
~~~~~~~~~~~~~

To make building on Z easier and faster, `this
script <https://github.com/hyperledger/fabric/blob/master/devenv/setupRHELonZ.sh>`__
is provided (which is similar to the `setup
file <https://github.com/hyperledger/fabric/blob/master/devenv/setup.sh>`__
provided for vagrant). This script has been tested only on RHEL 7.2 and
has some assumptions one might want to re-visit (firewall settings,
development as root user, etc.). It is however sufficient for
development in a personally-assigned VM instance.

To get started, from a freshly installed OS:

::

    sudo su
    yum install git
    mkdir -p $HOME/git/src/github.com/hyperledger
    cd $HOME/git/src/github.com/hyperledger
    git clone http://gerrit.hyperledger.org/r/fabric
    source fabric/devenv/setupRHELonZ.sh

From this point, you can proceed as described above for the Vagrant
development environment.

::

    cd $GOPATH/src/github.com/hyperledger/fabric
    make peer unit-test

Building on Power Platform
~~~~~~~~~~~~~~~~~~~~~~~~~~

Development and build on Power (ppc64le) systems is done outside of
vagrant as outlined `here <#building-outside-of-vagrant>`__. For ease
of setting up the dev environment on Ubuntu, invoke `this
script <https://github.com/hyperledger/fabric/blob/master/devenv/setupUbuntuOnPPC64le.sh>`__
as root. This script has been validated on Ubuntu 16.04 and assumes
certain things (like, development system has OS repositories in place,
firewall setting etc) and in general can be improvised further.

To get started on Power server installed with Ubuntu, first ensure you
have properly setup your Host's `GOPATH environment
variable <https://github.com/golang/go/wiki/GOPATH>`__. Then, execute
the following commands to build the fabric code:

::

    mkdir -p $GOPATH/src/github.com/hyperledger
    cd $GOPATH/src/github.com/hyperledger
    git clone http://gerrit.hyperledger.org/r/fabric
    sudo ./fabric/devenv/setupUbuntuOnPPC64le.sh
    cd $GOPATH/src/github.com/hyperledger/fabric
    make dist-clean all

Building on Centos 7
~~~~~~~~~~~~~~~~~~~~

You will have to build CouchDB from source because there is no package
available from the distribution. If you are planning a multi-orderer
arrangement, you will also need to install Apache Kafka from source.
Apache Kafka includes both Zookeeper and Kafka executables and
supporting artifacts.

::

   export GOPATH={directory of your choice}
   mkdir -p $GOPATH/src/github.com/hyperledger
   FABRIC=$GOPATH/src/github.com/hyperledger/fabric
   git clone https://github.com/hyperledger/fabric $FABRIC
   cd $FABRIC
   git checkout master # <-- only if you want the master branch
   export PATH=$GOPATH/bin:$PATH
   make native

If you are not trying to build for docker, you only need the natives.


Configuration
-------------

Configuration utilizes the `viper <https://github.com/spf13/viper>`__
and `cobra <https://github.com/spf13/cobra>`__ libraries.

There is a **core.yaml** file that contains the configuration for the
peer process. Many of the configuration settings can be overridden on
the command line by setting ENV variables that match the configuration
setting, but by prefixing with *'CORE\_'*. For example, logging level
manipulation through the environment is shown below:

::

    CORE_PEER_LOGGING_LEVEL=CRITICAL peer

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
