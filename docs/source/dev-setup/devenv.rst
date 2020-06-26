Setting up the development environment
--------------------------------------

Prerequisites
~~~~~~~~~~~~~

-  `Git client <https://git-scm.com/downloads>`__
-  `Go <https://golang.org/dl/>`__ version 1.14.x
-  `Docker <https://docs.docker.com/get-docker/>`__ version 18.03 or later
-  (macOS) `Xcode Command Line Tools <https://developer.apple.com/downloads/>`__
-  `SoftHSM <https://github.com/opendnssec/SoftHSMv2>`__
-  `jq <https://stedolan.github.io/jq/download/>`__


Steps
~~~~~

Install the Prerequisites
^^^^^^^^^^^^^^^^^^^^^^^^^

For macOS, we recommend using `Homebrew <https://brew.sh>`__ to manage the
development prereqs. The Xcode command line tools will be installed as part of
the Homebrew installation.

Once Homebrew is ready, installing the necessary prerequisites is very easy:

::

    brew install git go jq softhsm
    brew cask install --appdir="/Applications" docker

Docker Desktop must be launched to complete the installation so be sure to open
the application after installing it:

::

    open /Applications/Docker.app

Developing on Windows
~~~~~~~~~~~~~~~~~~~~~

On Windows 10 you should use the native Docker distribution and you
may use the Windows PowerShell. However, for the ``binaries``
command to succeed you will still need to have the ``uname`` command
available. You can get it as part of Git but beware that only the
64bit version is supported.

Before running any ``git clone`` commands, run the following commands:

::

    git config --global core.autocrlf false
    git config --global core.longpaths true

You can check the setting of these parameters with the following commands:

::

    git config --get core.autocrlf
    git config --get core.longpaths

These need to be ``false`` and ``true`` respectively.

The ``curl`` command that comes with Git and Docker Toolbox is old and
does not handle properly the redirect used in
:doc:`../getting_started`. Make sure you have and use a newer version
which can be downloaded from the `cURL downloads page
<https://curl.haxx.se/download.html>`__

Clone the Hyperledger Fabric source
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

First navigate to https://github.com/hyperledger/fabric and fork the fabric
repository using the fork button in the top-right corner. After forking, clone
the repository.

::

    mkdir -p github.com/<your_github_userid>
    cd github.com/<your_github_userid>
    git clone https://github.com/<your_github_userid>/fabric

.. note::
    If you are running Windows, before cloning the repository, run the following
    command:

    ::

        git config --get core.autocrlf

    If ``core.autocrlf`` is set to ``true``, you must set it to ``false`` by
    running:

    ::

        git config --global core.autocrlf false


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

SoftHSM configuration typically involves copying ``/etc/softhsm2.conf`` to
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

If you plan to use the Hyperledger Fabric application SDKs then be sure to check out their prerequisites in the Node.js SDK `README <https://github.com/hyperledger/fabric-sdk-node#build-and-test>`__ and Java SDK `README <https://github.com/hyperledger/fabric-gateway-java/blob/master/README.md>`__.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
