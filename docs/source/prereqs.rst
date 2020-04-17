Prerequisites
=============

Before you begin, you should confirm that you have installed all the prerequisites below on the platform where you will be running Hyperledger Fabric.

.. note:: These prerequisites are recommended for Fabric users. If you are a Fabric developer you should refer to the instructions for :doc:`dev-setup/devenv`.

Install Git
-----------
Download the latest version of `git
<https://git-scm.com/downloads>`_ if it is not already installed,
or if you have problems running the curl commands.

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

.. note:: The following applies to linux systems running systemd.

Make sure the docker daemon is running.

.. code:: bash

  sudo systemctl start docker

Optional: If you want the docker daemon to start when the system starts, use the following:

.. code:: bash

  sudo systemctl enable docker

Add your user to the docker group.

.. code:: bash

  sudo usermod -a -G docker <username>

.. note:: Installing Docker for Mac or Windows, or Docker Toolbox will also
          install Docker Compose. If you already had Docker installed, you
          should check that you have Docker Compose version 1.14.0 or greater
          installed. If not, we recommend that you install a more recent
          version of Docker.

You can check the version of Docker Compose you have installed with the
following command from a terminal prompt:

.. code:: bash

  docker-compose --version

.. _windows-extras:

Windows extras
--------------

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

.. note:: If you have questions not addressed by this documentation, or run into
          issues with any of the tutorials, please visit the :doc:`questions`
          page for some tips on where to find additional help.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
