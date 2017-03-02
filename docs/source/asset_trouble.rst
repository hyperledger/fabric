Troubleshooting
---------------

If you have existing containers running, you may receive an error
indicating that a port is already occupied. If this occurs, you will
need to kill the container that is using said port.

If a file cannot be located, make sure your curl commands executed
successfully and make sure you are in the directory where you pulled the
source code.

If you are receiving timeout or GRPC communication errors, make sure you
have the correct version of Docker installed - v1.13.0. Then try
restarting your failing docker process. For example:

.. code:: bash

    docker stop peer0

Then:

.. code:: bash

    docker start peer0

Another approach to GRPC and DNS errors (peer failing to resolve with
orderer and vice versa) is to hardcode the IP addresses for each. You
will know if there is a DNS issue, because a ``more results.txt``
command within the cli container will display something similar to:

.. code:: bash

    ERROR CREATING CHANNEL
    PEER0 ERROR JOINING CHANNEL

Issue a ``docker inspect <container_name>`` to ascertain the IP address.
For example:

.. code:: bash

    docker inspect peer0 | grep IPAddress

AND

.. code:: bash

    docker inspect orderer | grep IPAddress

Take these values and hard code them into your cli commands. For
example:

.. code:: bash

    CORE_PEER_COMMITTER_LEDGER_ORDERER=172.21.0.2:7050 peer channel create -c myc1

AND THEN

.. code:: bash

    CORE_PEER_COMMITTER_LEDGER_ORDERER=<IP_ADDRESS> CORE_PEER_ADDRESS=<IP_ADDRESS> peer channel join -b myc1.block

If you are seeing errors while using the node SDK, make sure you have
the correct versions of node.js and npm installed on your machine. You
want node v6.9.5 and npm v3.10.10.

If you ran through the automated channel create/join process (i.e. did
not comment out ``channel_test.sh`` in the
``docker-compose-gettingstarted.yml``), then channel - ``myc1`` - and
genesis block - ``myc1.block`` - have already been created and exist on
your machine. As a result, if you proceed to execute the manual steps in
your cli container:

::

    CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer channel create -c myc1

Then you will run into an error similar to:

::

    <EXACT_TIMESTAMP>       UTC [msp] Sign -> DEBU 064 Sign: digest: 5ABA6805B3CDBAF16C6D0DCD6DC439F92793D55C82DB130206E35791BCF18E5F
    Error: Got unexpected status: BAD_REQUEST
    Usage:
      peer channel create [flags]

This occurs because you are attempting to create a channel named
``myc1``, and this channel already exists! There are two options. Try
issuing the peer channel create command with a different channel name -
``myc2``. For example:

::

    CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer channel create -c myc2

Then join:

::

    CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 CORE_PEER_ADDRESS=peer0:7051 peer channel join -b myc2.block

If you do choose to create a new channel, and want to run
deploy/invoke/query with the node.js programs, you also need to edit the
"channelID" parameter in the ``config.json`` file to match the new
channel's name. For example:

::

    {
       "chainName":"fabric-client1",
       "chaincodeID":"mycc",
       "channelID":"myc2",
       "goPath":"../../test/fixtures",
       "chaincodePath":"github.com/example_cc",

OR, if you want your channel called - ``myc1`` -, remove your docker
containers and then follow the same commands in the **Manually create
and join peers to a new channel** topic.

Clean up
--------

Shut down your containers:

.. code:: bash

    docker-compose -f docker-compose-gettingstarted.yml down

Helpful Docker tips
-------------------

Remove a specific docker container:

.. code:: bash

    docker rm <containerID>

Force removal:

.. code:: bash

    docker rm -f <containerID>

Remove all docker containers:

.. code:: bash

    docker rm -f $(docker ps -aq)

This will merely kill docker containers (i.e. stop the process). You
will not lose any images.

Remove an image:

.. code:: bash

    docker rmi <imageID>

Forcibly remove:

.. code:: bash

    docker rmi -f <imageID>

Remove all images:

.. code:: bash

    docker rmi -f $(docker images -q)
