Setting up the development environment
--------------------------------------

Prerequisites
~~~~~~~~~~~~~

-  Git client
-  `Go <https://golang.org/dl/>`__ version 1.14.x
-  `Docker <https://docs.docker.com/get-docker/>`__ version 18.03 or later
-  (macOS)
   `Xcode <https://itunes.apple.com/us/app/xcode/id497799835?mt=12>`__
   must be installed
-  (macOS) GNU tar. You can use `Homebrew <https://brew.sh/>`__ to install
   it as follows:

::

    brew install gnu-tar

-  (macOS) If you've installed GNU tar, you should prepend the "gnubin"
   directory to your $PATH with something like:

::

    export PATH=/usr/local/opt/gnu-tar/libexec/gnubin:$PATH

-  `Libtool <https://www.gnu.org/software/libtool/>`__. You can use
   Homebrew to install it as follows:

::

    brew install libtool

-  `SoftHSM <https://github.com/opendnssec/SoftHSMv2>`__. You can use
   Homebrew or apt to install it as follows:

::

    brew install softhsm              # macOS
    sudo apt-get install libsofthsm2  # Ubuntu

-  `jq <https://stedolan.github.io/jq/download/>`__. You can use
   Homebrew to install it as follows:

::

    brew install jq

Steps
~~~~~

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
    If you are running Windows, before cloning the repository, run thefollowing
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

    export PKCS11_LIB="/usr/local/Cellar/softhsm/2.5.0/lib/softhsm/libsofthsm2.so"
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
a few commands.

::

    make basic-checks docker-test-prereqs
    ginkgo -r ./integration/nwo

If those commands completely successfully, you're ready to Go!

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
