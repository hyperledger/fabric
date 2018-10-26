Install Samples, Binaries and Docker Images
===========================================

While we work on developing real installers for the Hyperledger Fabric
binaries, we provide a script that will download and install samples and
binaries to your system. We think that you'll find the sample applications
installed useful to learn more about the capabilities and operations of
Hyperledger Fabric.


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

Determine a location on your machine where you want to place the `fabric-samples`
repository and enter that directory in a terminal window. The
command that follows will perform the following steps:

#. If needed, clone the `hyperledger/fabric-samples` repository
#. Checkout the appropriate version tag
#. Install the Hyperledger Fabric platform-specific binaries and config files
   for the version specified into the /bin and /config directories of fabric-samples
#. Download the Hyperledger Fabric docker images for the version specified

Once you are ready, and in the directory into which you will install the
Fabric Samples and binaries, go ahead and execute the following command:

.. code:: bash

  curl -sSL http://bit.ly/2ysbOFE | bash -s 1.2.1

.. note:: If you want to download different versions for Fabric, Fabric-ca and thirdparty
          Docker images, you must pass the version identifier for each.

.. code:: bash

  curl -sSL http://bit.ly/2ysbOFE | bash -s <fabric> <fabric-ca> <thirdparty>
  curl -sSL http://bit.ly/2ysbOFE | bash -s 1.2.1 1.2.1 0.4.10

.. note:: If you get an error running the above curl command, you may
          have too old a version of curl that does not handle
          redirects or an unsupported environment.

	  Please visit the :doc:`prereqs` page for additional
	  information on where to find the latest version of curl and
	  get the right environment. Alternately, you can substitute
	  the un-shortened URL:
	  https://raw.githubusercontent.com/hyperledger/fabric/master/scripts/bootstrap.sh

.. note:: You can use the command above for any published version of Hyperledger
          Fabric. Simply replace `1.2.1` with the version identifier
          of the version you wish to install.

The command above downloads and executes a bash script
that will download and extract all of the platform-specific binaries you
will need to set up your network and place them into the cloned repo you
created above. It retrieves the following platform-specific binaries:

  * ``configtxgen``,
  * ``configtxlator``,
  * ``cryptogen``,
  * ``idemixgen``
  * ``orderer``,
  * ``peer``, and
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
two instances of the same image ID - one tagged as "amd64-1.x.x" and
one tagged as "latest". Prior to 1.2.0, the image being downloaded was determined
by ``uname -m`` and showed as "x86_64-1.x.x".

.. note:: On different architectures, the x86_64/amd64 would be replaced
          with the string identifying your architecture.

.. note:: If you have questions not addressed by this documentation, or run into
          issues with any of the tutorials, please visit the :doc:`questions`
          page for some tips on where to find additional help.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
