Setting up the development environment
--------------------------------------

Overview
~~~~~~~~

Prior to the v1.0.0 release, the development environment utilized Vagrant
running an Ubuntu image, which in turn launched Docker containers as a
means of ensuring a consistent experience for developers who might be
working with varying platforms, such as macOS, Windows, Linux, or
whatever. Advances in Docker have enabled native support on the most
popular development platforms: macOS and Windows. Hence, we have
reworked our build to take full advantage of these advances. While we
still maintain a Vagrant based approach that can be used for older
versions of macOS and Windows that Docker does not support, we strongly
encourage that the non-Vagrant development setup be used.

Note that while the Vagrant-based development setup could not be used in
a cloud context, the Docker-based build does support cloud platforms
such as AWS, Azure, Google and IBM to name a few. Please follow the
instructions for Ubuntu builds, below.

Prerequisites
~~~~~~~~~~~~~

-  `Git client <https://git-scm.com/downloads>`__
-  `Go <https://golang.org/dl/>`__ - version 1.12.x
-  (macOS)
   `Xcode <https://itunes.apple.com/us/app/xcode/id497799835?mt=12>`__
   must be installed
-  `Docker <https://www.docker.com/get-docker>`__ - 17.06.2-ce or later
-  `Docker Compose <https://docs.docker.com/compose/>`__ - 1.14.0 or later
-  `Pip <https://pip.pypa.io/en/stable/installing/>`__
-  (macOS) you may need to install gnutar, as macOS comes with bsdtar
   as the default, but the build uses some gnutar flags. You can use
   Homebrew to install it as follows:

::

    brew install gnu-tar --with-default-names

-  (macOS) `Libtool <https://www.gnu.org/software/libtool/>`__. You can use
   Homebrew to install it as follows:

::

    brew install libtool

-  (only if using Vagrant) - `Vagrant <https://www.vagrantup.com/>`__ -
   1.9 or later
-  (only if using Vagrant) -
   `VirtualBox <https://www.virtualbox.org/>`__ - 5.0 or later
-  BIOS Enabled Virtualization - Varies based on hardware

-  Note: The BIOS Enabled Virtualization may be within the CPU or
   Security settings of the BIOS

``pip``
~~~~~~

::

    pip install --upgrade pip


Steps
~~~~~

Set your GOPATH
^^^^^^^^^^^^^^^

Make sure you have properly setup your Host's `GOPATH environment
variable <https://github.com/golang/go/wiki/GOPATH>`__. This allows for
both building within the Host and the VM.

In case you installed Go into a different location from the standard one
your Go distribution assumes, make sure that you also set `GOROOT
environment variable <https://golang.org/doc/install#install>`__.

Note to Windows users
^^^^^^^^^^^^^^^^^^^^^

If you are running Windows, before running any ``git clone`` commands,
run the following command.

::

    git config --get core.autocrlf

If ``core.autocrlf`` is set to ``true``, you must set it to ``false`` by
running

::

    git config --global core.autocrlf false

If you continue with ``core.autocrlf`` set to ``true``, the
``vagrant up`` command will fail with the error:

``./setup.sh: /bin/bash^M: bad interpreter: No such file or directory``

Cloning the Hyperledger Fabric source
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

First navigate to https://github.com/hyperledger/fabric and fork the
fabric repository using the fork button in the top-right corner

Since Hyperledger Fabric is written in ``Go``, you'll need to
clone the forked repository to your $GOPATH/src directory. If your $GOPATH
has multiple path components, then you will want to use the first one.
There's a little bit of setup needed:

::

    mkdir -p github.com/<your_github_userid>
    cd github.com/<your_github_userid>
    git clone https://github.com/<your_github_userid>/fabric
    cd github.com/<your_github_userid>

Bootstrapping the VM using Vagrant
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

If you are planning on using the Vagrant developer environment, the
following steps apply. **Again, we recommend against its use except for
developers that are limited to older versions of macOS and Windows that
are not supported by Docker for Mac or Windows.**

::

    cd $GOPATH/src/github.com/hyperledger/fabric/devenv
    vagrant up

Go get coffee... this will take a few minutes. Once complete, you should
be able to ``ssh`` into the Vagrant VM just created.

::

    vagrant ssh

Once inside the VM, you can find the source under
``$GOPATH/src/github.com/hyperledger/fabric``. It is also mounted as
``/hyperledger``.

Building Hyperledger Fabric
~~~~~~~~~~~~~~~~~~~~~~~~~~~

Once you have all the dependencies installed, and have cloned the
repository, you can proceed to :doc:`build and test <build>` Hyperledger
Fabric.

Notes
~~~~~

**NOTE:** Any time you change any of the files in your local fabric
directory (under ``$GOPATH/src/github.com/hyperledger/fabric``), the
update will be instantly available within the VM fabric directory.

**NOTE:** If you intend to run the development environment behind an
HTTP Proxy, you need to configure the guest so that the provisioning
process may complete. You can achieve this via the *vagrant-proxyconf*
plugin. Install with ``vagrant plugin install vagrant-proxyconf`` and
then set the VAGRANT\_HTTP\_PROXY and VAGRANT\_HTTPS\_PROXY environment
variables *before* you execute ``vagrant up``. More details are
available here: https://github.com/tmatilai/vagrant-proxyconf/

**NOTE:** The first time you run this command it may take quite a while
to complete (it could take 30 minutes or more depending on your
environment) and at times it may look like it's not doing anything. As
long you don't get any error messages just leave it alone, it's all
good, it's just cranking.

**NOTE to Windows 10 Users:** There is a known problem with vagrant on
Windows 10 (see
`hashicorp/vagrant#6754 <https://github.com/hashicorp/vagrant/issues/6754>`__).
If the ``vagrant up`` command fails it may be because you do not have
the Microsoft Visual C++ Redistributable package installed. You can
download the missing package at the following address:
http://www.microsoft.com/en-us/download/details.aspx?id=8328

**NOTE:** The inclusion of the miekg/pkcs11 package introduces
an external dependency on the ltdl.h header file during
a build of fabric. Please ensure your libtool and libltdl-dev packages
are installed. Otherwise, you may get a ltdl.h header missing error.
You can download the missing package by command:
``sudo apt-get install -y build-essential git make curl unzip g++ libtool``.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

