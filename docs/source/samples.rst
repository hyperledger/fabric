Hyperledger Fabric Samples
==========================

.. note:: If you are running on **Windows** you will want to make use of the
	  Docker Quickstart Terminal for the upcoming terminal commands.
          Please visit the :doc:`prereqs` if you haven't previously installed
          it.

          If you are using Docker Toolbox on Windows 7 or macOS, you
          will need to use a location under ``C:\Users`` (Windows 7) or
          ``/Users`` (macOS) when installing and running the samples.

          If you are using Docker for Mac, you will need to use a location
          under ``/Users``, ``/Volumes``, ``/private``, or ``/tmp``.  To use a different
          location, please consult the Docker documentation for
          `file sharing <https://docs.docker.com/docker-for-mac/#file-sharing>`__.

          If you are using Docker for Windows, please consult the Docker
          documentation for `shared drives <https://docs.docker.com/docker-for-windows/#shared-drives>`__
          and use a location under one of the shared drives.

Determine a location on your machine where you want to place the Hyperledger
Fabric samples applications repository and open that in a terminal window. Then,
execute the following commands:

.. code:: bash

  git clone -b master https://github.com/hyperledger/fabric-samples.git
  cd fabric-samples
  git checkout {TAG}　

.. note:: To ensure the samples are compatible with the version of Fabric binaries you download below,
          checkout the samples ``{TAG}`` that matches your Fabric version, for example, v1.1.0.
          To see a list of all fabric-samples tags, use command "git tag".

.. _binaries:

Download Platform-specific Binaries
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Next, we will install the Hyperledger Fabric platform-specific binaries.
This process was designed to complement the Hyperledger Fabric Samples
above, but can be used independently. If you are not installing the
samples above, then simply create and enter a directory into which to
extract the contents of the platform-specific binaries.

Please execute the following command from within the directory into which
you will extract the platform-specific binaries:

.. code:: bash

  curl -sSL https://goo.gl/6wtTN5 | bash -s 1.1.0

.. note:: If you get an error running the above curl command, you may
          have too old a version of curl that does not handle
          redirects or an unsupported environment.

	  Please visit the :doc:`prereqs` page for additional
	  information on where to find the latest version of curl and
	  get the right environment. Alternately, you can substitute
	  the un-shortened URL:
	  https://github.com/hyperledger/fabric/blob/master/scripts/bootstrap.sh

.. note:: You can use the command above for any published version of Hyperledger
          Fabric. Simply replace '1.1.0' with the version identifier
          of the version you wish to install.

The command above downloads and executes a bash script
that will download and extract all of the platform-specific binaries you
will need to set up your network and place them into the cloned repo you
created above. It retrieves four platform-specific binaries:

  * ``cryptogen``,
  * ``configtxgen``,
  * ``configtxlator``,
  * ``peer``
  * ``orderer`` and
  * ``fabric-ca-client``

and places them in the ``bin`` sub-directory of the current working
directory.

You may want to add that to your PATH environment variable so that these
can be picked up without fully qualifying the path to each binary. e.g.:

.. code:: bash

  export PATH=<path to download location>/bin:$PATH

Finally, the script will download the Hyperledger Fabric docker images from
`Docker Hub <https://hub.docker.com/u/hyperledger/>`__ into
your local Docker registry and tag them as 'latest'.

The script lists out the Docker images installed upon conclusion.

Look at the names for each image; these are the components that will ultimately
comprise our Hyperledger Fabric network.  You will also notice that you have
two instances of the same image ID - one tagged as "x86_64-1.x.x" and
one tagged as "latest".

.. note:: On different architectures, the x86_64 would be replaced
          with the string identifying your architecture.

.. note:: If you have questions not addressed by this documentation, or run into
          issues with any of the tutorials, please visit the :doc:`questions`
          page for some tips on where to find additional help.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
