Prerequisites
=============

Before we begin, if you haven't already done so, you may wish to check that
you have all the prerequisites below installed on the platform(s)
on which you'll be developing blockchain applications and/or operating
Hyperledger Fabric.

Install cURL
------------

Download the latest version of the `cURL
<https://curl.haxx.se/download.html>`__ tool if it is not already
installed or if you get errors running the curl commands from the
documentation.

.. note:: If you're on Windows please see the specific note on `Windows
   extras`_ below.

Docker and Docker Compose
-------------------------

You will need the following installed on the platform on which you will be
operating, or developing on (or for), Hyperledger Fabric:

  - MacOSX, \*nix, or Windows 10: `Docker <https://www.docker.com/get-docker>`__
    Docker version 17.06.2-ce or greater is required.
  - Older versions of Windows: `Docker
    Toolbox <https://docs.docker.com/toolbox/toolbox_install_windows/>`__ -
    again, Docker version Docker 17.06.2-ce or greater is required.

You can check the version of Docker you have installed with the following
command from a terminal prompt:

.. code:: bash

  docker --version

.. note:: Installing Docker for Mac or Windows, or Docker Toolbox will also
          install Docker Compose. If you already had Docker installed, you
          should check that you have Docker Compose version 1.14.0 or greater
          installed. If not, we recommend that you install a more recent
          version of Docker.

You can check the version of Docker Compose you have installed with the
following command from a terminal prompt:

.. code:: bash

  docker-compose --version

.. _Golang:

Go Programming Language
-----------------------

Hyperledger Fabric uses the Go Programming Language for many of its
components.

  - `Go <https://golang.org/dl/>`__ version 1.10.x is required.

Given that we will be writing chaincode programs in Go, there are two
environment variables you will need to set properly; you can make these
settings permanent by placing them in the appropriate startup file, such
as your personal ``~/.bashrc`` file if you are using the ``bash`` shell
under Linux.

First, you must set the environment variable ``GOPATH`` to point at the
Go workspace containing the downloaded Fabric code base, with something like:

.. code:: bash

  export GOPATH=$HOME/go

.. note:: You **must** set the GOPATH variable

  Even though, in Linux, Go's ``GOPATH`` variable can be a colon-separated list
  of directories, and will use a default value of ``$HOME/go`` if it is unset,
  the current Fabric build framework still requires you to set and export that
  variable, and it must contain **only** the single directory name for your Go
  workspace. (This restriction might be removed in a future release.)

Second, you should (again, in the appropriate startup file) extend your
command search path to include the Go ``bin`` directory, such as the following
example for ``bash`` under Linux:

.. code:: bash

  export PATH=$PATH:$GOPATH/bin

While this directory may not exist in a new Go workspace installation, it is
populated later by the Fabric build system with a small number of Go executables
used by other parts of the build system. So even if you currently have no such
directory yet, extend your shell search path as above.

Node.js Runtime and NPM
-----------------------

If you will be developing applications for Hyperledger Fabric leveraging the
Hyperledger Fabric SDK for Node.js, you will need to have version 8.9.x of Node.js
installed.

.. note:: Node.js version 9.x is not supported at this time.

  - `Node.js <https://nodejs.org/en/download/>`__ - version 8.9.x or greater

.. note:: Installing Node.js will also install NPM, however it is recommended
          that you confirm the version of NPM installed. You can upgrade
          the ``npm`` tool with the following command:

.. code:: bash

  npm install npm@5.6.0 -g

Python
^^^^^^

.. note:: The following applies to Ubuntu 16.04 users only.

By default Ubuntu 16.04 comes with Python 3.5.1 installed as the ``python3`` binary.
The Fabric Node.js SDK requires an iteration of Python 2.7 in order for ``npm install``
operations to complete successfully.  Retrieve the 2.7 version with the following command:

.. code:: bash

  sudo apt-get install python

Check your version(s):

.. code:: bash

  python --version

.. _windows-extras:

Windows extras
--------------

If you are developing on Windows 7, you will want to work within the
Docker Quickstart Terminal which uses `Git Bash
<https://git-scm.com/downloads>`__ and provides a better alternative
to the built-in Windows shell.

However experience has shown this to be a poor development environment
with limited functionality. It is suitable to run Docker based
scenarios, such as :doc:`getting_started`, but you may have
difficulties with operations involving the ``make`` and ``docker``
commands.

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
:doc:`getting_started`. Make sure you install and use a newer version
from the `cURL downloads page <https://curl.haxx.se/download.html>`__

For Node.js you also need the necessary Visual Studio C++ Build Tools
which are freely available and can be installed with the following
command:

.. code:: bash

	  npm install --global windows-build-tools

See the `NPM windows-build-tools page
<https://www.npmjs.com/package/windows-build-tools>`__ for more
details.

Once this is done, you should also install the NPM GRPC module with the
following command:

.. code:: bash

	  npm install --global grpc

Your environment should now be ready to go through the
:doc:`getting_started` samples and tutorials.

.. note:: If you have questions not addressed by this documentation, or run into
          issues with any of the tutorials, please visit the :doc:`questions`
          page for some tips on where to find additional help.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
