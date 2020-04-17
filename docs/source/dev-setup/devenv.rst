Setting up the development environment
--------------------------------------

Prerequisites
~~~~~~~~~~~~~

-  Git client, Go, and Docker as described at :doc:`../prereqs`
-  (macOS)
   `Xcode <https://itunes.apple.com/us/app/xcode/id497799835?mt=12>`__
   must be installed
-  (macOS) you may need to install gnutar, as macOS comes with bsdtar
   as the default, but the build uses some gnutar flags. You can use
   Homebrew to install it as follows:

::

    brew install gnu-tar

-  (macOS) If you install gnutar, you should prepend the "gnubin"
   directory to the $PATH environment variable with something like:

::

    export PATH=/usr/local/opt/gnu-tar/libexec/gnubin:$PATH

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
:doc:`getting_started`. Make sure you have and use a newer version
which can be downloaded from the `cURL downloads page
<https://curl.haxx.se/download.html>`__

Clone the Hyperledger Fabric source
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

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
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

First navigate to https://github.com/hyperledger/fabric and fork the
fabric repository using the fork button in the top-right corner

Since Hyperledger Fabric is written in ``Go``, you'll need to
clone the forked repository to your $GOPATH/src directory. If your $GOPATH
has multiple path components, then you will want to use the first one.
There's a little bit of setup needed:

::

    cd $GOPATH/src
    mkdir -p github.com/<your_github_userid>
    cd github.com/<your_github_userid>
    git clone https://github.com/<your_github_userid>/fabric

If you plan to use the Hyperledger Fabric application SDKs then be sure to check out their prerequisites in the Node.js SDK `README <https://github.com/hyperledger/fabric-sdk-node#build-and-test>`__ and Java SDK `README <https://github.com/hyperledger/fabric-gateway-java/blob/master/README.md>`__.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
