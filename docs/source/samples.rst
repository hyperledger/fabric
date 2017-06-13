Hyperledger Fabric Samples
==========================

.. note:: If you are running on **Windows** you will want to make use of the
          ``Git bash shell`` extension for the upcoming terminal commands.
          Please visit the :doc:`prereqs` if you haven't previously installed
          it.

Determine a location on your machine where you want to place the Hyperledger
Fabric samples applications repository and open that in a terminal window. Then,
execute the following commands:

.. code:: bash

  git clone https://github.com/hyperledger/fabric-samples.git
  cd fabric-samples

Download Platform-specific Binaries
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Next, we will install the Hyperledger Fabric platform-specific binaries.
To do this, execute the following command:

.. code:: bash

  curl -sSL https://goo.gl/LQkuoh | bash

The curl command above downloads and executes a bash script
that will download and extract all of the platform-specific binaries you
will need to set up your network and place them into the cloned repo you
created above. It retrieves the three platform-specific binaries:
  * ``cryptogen``,
  * ``configtxgen`` and,
  * ``configtxlator``

and places them in the ``fabric-samples/bin`` directory.

Finally, the script will download the Hyperledger Fabric docker images from
`DockerHub <https://hub.docker.com/u/hyperledger/>`__ into
your local Docker registry and tag them as 'latest'.

The script lists out the docker images installed upon conclusion.

Look at the names for each image; these are the components that will ultimately
comprise our Fabric network.  You will also notice that you have two instances
of the same image ID - one tagged as "x86_64-1.0.0-rc1" and one tagged as "latest".

.. note:: Note that on different architectures, the x86_64 would be replaced
          with the string identifying your architecture.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
