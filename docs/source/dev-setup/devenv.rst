Setting up the development environment
--------------------------------------

Prerequisites
~~~~~~~~~~~~~

<<<<<<< HEAD
-  `Git client <https://git-scm.com/downloads>`__
-  `Go <https://golang.org/dl/>`__ version 1.15.x
-  `Docker <https://docs.docker.com/get-docker/>`__ version 18.03 or later
-  (macOS) `Xcode Command Line Tools <https://developer.apple.com/downloads/>`__
-  `SoftHSM <https://github.com/opendnssec/SoftHSMv2>`__
-  `jq <https://stedolan.github.io/jq/download/>`__
=======
In addition to the standard :doc:`../prereqs` for Fabric, the following prerequisites are also required:
>>>>>>> e7ebce15b (Address windows platform in documentation)

-  (macOS) `Xcode Command Line Tools <https://developer.apple.com/downloads/>`__
-  (All platforms) `SoftHSM <https://github.com/opendnssec/SoftHSMv2>`__ use version 2.5 as 2.6 is not operable in this environment
-  (All platforms) `jq <https://stedolan.github.io/jq/download/>`__

For Linux platforms, including WSL2 on Windows, also required are various build tools such as gnu-make and 
C compiler. On ubuntu and it's derivatives you can install the required toolset by using the command 
``sudo apt install build-essential``. Other distributions may already have the appropriate tools installed
or provide a convenient way to install the various build tools.

Steps
~~~~~

Install the Prerequisites
^^^^^^^^^^^^^^^^^^^^^^^^^

For macOS, we recommend using `Homebrew <https://brew.sh>`__ to manage the
development prereqs. The Xcode command line tools will be installed as part of
the Homebrew installation.

Once Homebrew is ready, installing the necessary prerequisites is very easy:

::

<<<<<<< HEAD
    brew install git go jq softhsm
    brew cask install --appdir="/Applications" docker
=======
    brew install git jq
    brew install --cask docker

Go and SoftHSM are also available from Homebrew, but make sure you install the appropriate versions
>>>>>>> e7ebce15b (Address windows platform in documentation)

Docker Desktop must be launched to complete the installation, so be sure to open
the application after installing it:

::

    open /Applications/Docker.app

Developing on Windows
~~~~~~~~~~~~~~~~~~~~~

It is recommended that all development be done within your WSL2 Linux distribution.

Clone the Hyperledger Fabric source
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

First navigate to https://github.com/hyperledger/fabric and fork the fabric
repository using the fork button in the top-right corner. After forking, clone
the repository.

::

    mkdir -p github.com/<your_github_userid>
    cd github.com/<your_github_userid>
    git clone https://github.com/<your_github_userid>/fabric


Configure SoftHSM
^^^^^^^^^^^^^^^^^

A PKCS #11 cryptographic token implementation is required to run the unit
tests. The PKCS #11 API is used by the bccsp component of Fabric to interact
with hardware security modules (HSMs) that store cryptographic information and
perform cryptographic computations.  For test environments, SoftHSM can be used
to satisfy this requirement.

SoftHSM generally requires additional configuration before it can be used. For
example, the default configuration will attempt to store token data in a system
directory that unprivileged users are unable to write to.

SoftHSM configuration typically involves copying ``/etc/softhsm/softhsm2.conf`` to
``$HOME/.config/softhsm2/softhsm2.conf`` and changing ``directories.tokendir``
to an appropriate location. Please see the man page for ``softhsm2.conf`` for
details.

After SoftHSM has been configured, the following command can be used to
initialize the token required by the unit tests:

::

    softhsm2-util --init-token --slot 0 --label "ForFabric" --so-pin 1234 --pin 98765432

If tests are unable to locate the libsofthsm2.so library in your environment,
specify the library path, the PIN, and the label of your token in the
appropriate environment variables. For example, on macOS:

::

    export PKCS11_LIB="/usr/local/Cellar/softhsm/2.6.1/lib/softhsm/libsofthsm2.so"
    export PKCS11_PIN=98765432
    export PKCS11_LABEL="ForFabric"

<<<<<<< HEAD
=======
If you installed SoftHSM on ubuntu from source then the environment variables may look like

::

    export PKCS11_LIB="/usr/local/lib/softhsm/libsofthsm2.so"
    export PKCS11_PIN=98765432
    export PKCS11_LABEL="ForFabric"


The tests don't always clean up after themselves and, over time, this causes
the PKCS #11 tests to take a long time to run. The easiest way to recover from
this is to delete and recreate the token.

::

    softhsm2-util --delete-token --token ForFabric
    softhsm2-util --init-token --slot 0 --label ForFabric --so-pin 1234 --pin 98765432

Debugging with ``pkcs11-spy``
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

The `OpenSC Project <https://github.com/OpenSC/OpenSC>`__ provides a shared
library called ``pkcs11-spy`` that logs all interactions between an application
and a PKCS #11 module. This library can be very useful when troubleshooting
interactions with a cryptographic token device or service.

Once the library has been installed, configure Fabric to use ``pkcs11-spy`` as
the PKCS #11 library and set the ``PKCS11SPY`` environment variable to the real
library. For example:

::

    export PKCS11SPY="/usr/lib/softhsm/libsofthsm2.so"
    export PKCS11_LIB="/usr/lib/x86_64-linux-gnu/pkcs11/pkcs11-spy.so"


>>>>>>> e7ebce15b (Address windows platform in documentation)
Install the development tools
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Once the repository is cloned, you can use ``make`` to install some of the
tools used in the development environment. By default, these tools will be
installed into ``$HOME/go/bin``. Please be sure your ``PATH`` includes that
directory.

::

    make gotools

After installing the tools, the build environment can be verified by running a
few commands.

::

    make basic-checks integration-test-prereqs
    ginkgo -r ./integration/nwo

If those commands completely successfully, you're ready to Go!

If you plan to use the Hyperledger Fabric application SDKs then be sure to check out their prerequisites in the Node.js SDK `README <https://github.com/hyperledger/fabric-sdk-node#build-and-test>`__, Java SDK `README <https://github.com/hyperledger/fabric-gateway-java/blob/master/README.md>`__, and Go SDK `README <https://github.com/hyperledger/fabric-sdk-go/blob/main/README.md>`__.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
