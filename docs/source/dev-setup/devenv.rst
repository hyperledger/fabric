Setting up the development environment
--------------------------------------

Prerequisites
~~~~~~~~~~~~~

-  `Git client <https://git-scm.com/downloads>`__
-  `Go <https://golang.org/dl/>`__ - version 1.12.x
-  (macOS)
   `Xcode <https://itunes.apple.com/us/app/xcode/id497799835?mt=12>`__
   must be installed
-  `Docker <https://www.docker.com/get-docker>`__ - 17.06.2-ce or later
-  `Docker Compose <https://docs.docker.com/compose/>`__ - 1.14.0 or later
-  (macOS) you may need to install gnutar, as macOS comes with bsdtar
   as the default, but the build uses some gnutar flags. You can use
   Homebrew to install it as follows:

::

    brew install gnu-tar

-  (macOS) If you install gnutar, you should prepend the "gnubin"
   directory to the $PATH environment variable with something like:

::

    export PATH=/usr/local/opt/gnu-tar/libexec/gnubin:$PATH

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

Configuring Gerrit to Use SSH
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Gerrit uses SSH to interact with your Git client. If you already have an SSH
key pair, you can skip the part of this section that explains how to generate one.

What follows explains how to generate an SSH key pair in a Linux environment ---
follow the equivalent steps on your OS.

First, create an SSH key pair with the command:

::

    ssh-keygen -t rsa -C "John Doe john.doe@example.com"

**Note:** This will ask you for a password to protect the private key as
it generates a unique key. Please keep this password private, and DO NOT
enter a blank password.

The generated SSH key pair can be found in the files ``~/.ssh/id_rsa`` and
``~/.ssh/id_rsa.pub``.

Next, add the private key in the ``id_rsa`` file to your key ring, e.g.:

::

    ssh-add ~/.ssh/id_rsa

Finally, add the public key of the generated key pair to the Gerrit server,
with the following steps:

1. Go to
   `Gerrit <https://gerrit.hyperledger.org/r/#/admin/projects/fabric>`__.

2. Click on your account name in the upper right corner.

3. From the pop-up menu, select ``Settings``.

4. On the left side menu, click on ``SSH Public Keys``.

5. Paste the contents of your public key ``~/.ssh/id_rsa.pub`` and click
   ``Add key``.

**Note:** The ``id_rsa.pub`` file can be opened with any text editor.
Ensure that all the contents of the file are selected, copied and pasted
into the ``Add SSH key`` window in Gerrit.

**Note:** The SSH key generation instructions operate on the assumption
that you are using the default naming. It is possible to generate
multiple SSH keys and to name the resulting files differently. See the
`ssh-keygen <https://en.wikipedia.org/wiki/Ssh-keygen>`__ documentation
for details on how to do that. Once you have generated non-default keys,
you need to configure SSH to use the correct key for Gerrit. In that
case, you need to create a ``~/.ssh/config`` file modeled after the one
below.

::

    host gerrit.hyperledger.org
     HostName gerrit.hyperledger.org
     IdentityFile ~/.ssh/id_rsa_hyperledger_gerrit
     User <LFID>

where <LFID> is your Linux Foundation ID and the value of IdentityFile is the
name of the public key file you generated.

**Warning:** Potential Security Risk! Do not copy your private key
``~/.ssh/id_rsa``. Use only the public ``~/.ssh/id_rsa.pub``.

Cloning the Hyperledger Fabric source
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Since Hyperledger Fabric is written in ``Go``, you'll need to
clone the source repository to your $GOPATH/src directory. If your $GOPATH
has multiple path components, then you will want to use the first one.
There's a little bit of setup needed:

::

    cd $GOPATH/src
    mkdir -p github.com/hyperledger
    cd github.com/hyperledger

Recall that we are using ``Gerrit`` for source control, which has its
own internal git repositories. Hence, we will need to clone from
:doc:`Gerrit <../Gerrit/gerrit>`.
For brevity, the command is as follows:

::

    git clone ssh://LFID@gerrit.hyperledger.org:29418/fabric && scp -p -P 29418 LFID@gerrit.hyperledger.org:hooks/commit-msg fabric/.git/hooks/

**Note:** Of course, you would want to replace ``LFID`` with your own
Linux Foundation ID.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
