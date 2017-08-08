Install and Instantiate
=======================

This tutorial requires the latest builds for
``hyperledger/fabric-baseimage``, ``hyperledger/fabric-peer`` and
``hyperledger/fabric-orderer``. Rather than pull from Docker Hub, you
can compile these images locally to ensure they are up to date. It is up
to the user how to build the images, although a typical approach is
through vagrant. If you do choose to build through vagrant, make sure
you have followed the steps outlined in `setting up the development
environment <dev-setup/devenv.html>`__. Then from the fabric directory
within your vagrant environment, execute the ``make peer-docker`` and
``make orderer-docker`` commands.

Start the network of 2 peers, an orderer, and a CLI
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Navigate to the fabric/docs directory in your vagrant environment and
start your network:

.. code:: bash

    docker-compose -f docker-2peer.yml up

View all your containers:

.. code:: bash

    # active and non-active
    docker ps -a

Get into the CLI container
~~~~~~~~~~~~~~~~~~~~~~~~~~

Now, open a second terminal and navigate once again to your vagrant
environment.

.. code:: bash

    docker exec -it cli bash

You should see the following in your terminal:

.. code:: bash

    root@ccd3308afc73:/opt/gopath/src/github.com/hyperledger/fabric/peer#

Create and join channel from the remote CLI
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

From your second terminal, lets create a channel by the name of "myc":

.. code:: bash

    peer channel create -c myc -o orderer:5005

This will generate a genesis block - ``myc.block`` - and place it into
the same directory from which you issued your ``peer channel create``
command. Now, from the same directory, direct both peers to join channel
- ``myc`` - by passing in the genesis block - ``myc.block`` - with a
``peer channel join`` command:

.. code:: bash

    CORE_PEER_ADDRESS=peer0:7051 peer channel join -b myc.block
    CORE_PEER_ADDRESS=peer1:7051 peer channel join -b myc.block

Install the chaincode on peer0 from the remote CLI
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

From your second terminal, and still within the CLI container, issue the
following command to install a chaincode named ``mycc`` with a version
of ``v0`` onto ``peer0``.

.. code:: bash

    CORE_PEER_ADDRESS=peer0:7051 peer chaincode install -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -v v0

Instantiate the chaincode on the channel from the remote CLI
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Now, still within the cli container in your second terminal, instantiate
the chaincode ``mycc`` with version ``v0`` onto ``peer0``. This
instantiation will initialize the chaincode with key value pairs of
["a","100"] and ["b","200"].

.. code:: bash

    CORE_PEER_ADDRESS=peer0:7051 peer chaincode instantiate -o orderer:5005 -C myc -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -v v0 -c '{"Args":["init","a","100","b","200"]}'

**Continue operating within your second terminal for the remainder of
the commands**

Query for the value of "a" to make sure the chaincode container has successfully started
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Send a query to ``peer0`` for the value of key ``"a"``:

.. code:: bash

    CORE_PEER_ADDRESS=peer0:7051 peer chaincode query -C myc -n mycc -v v0 -c '{"Args":["query","a"]}'

This query should return "100".

Invoke to make a state change
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Send an invoke request to ``peer0`` to move 10 units from "a" to "b":

.. code:: bash

    CORE_PEER_ADDRESS=peer0:7051 peer chaincode invoke -C myc -n mycc -v v0 -c '{"Args":["invoke","a","b","10"]}'

Query on the second peer
~~~~~~~~~~~~~~~~~~~~~~~~

Issue a query against the key "a" to ``peer1``. Recall that ``peer1``
has successfully joined the channel.

.. code:: bash

    CORE_PEER_ADDRESS=peer1:7051 peer chaincode query -C myc -n mycc -v v0 -c '{"Args":["query","a"]}'

This will return an error response because ``peer1`` does not have the
chaincode installed.

Install on the second peer
~~~~~~~~~~~~~~~~~~~~~~~~~~

Now add the chaincode to ``peer1`` so that you can successfully perform
read/write operations.

.. code:: bash

    CORE_PEER_ADDRESS=peer1:7051 peer chaincode install -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -v v0

| **Note**: The initial instantiation applies to all peers in the
  channel, and is affected upon any peer that has the chaincode
  installed. Therefore, we installed the chaincode on ``peer0`` in order
  to execute the instantiate command through it.
| Now that we want to access the chaincode on ``peer1``, we must install
  the chaincode on ``peer1`` as well. In general, a chaincode has to be
  installed only on those peers through which the chaincode needs to be
  accessed from. In particular, the chaincode must be installed on any
  peer receiving endorsement requests for that chaincode.

Query on the second peer
~~~~~~~~~~~~~~~~~~~~~~~~

Now issue the same query request to ``peer1``.

.. code:: bash

    CORE_PEER_ADDRESS=peer1:7051 peer chaincode query -C myc -n mycc -v v0 -c '{"Args":["query","a"]}'

Query will now succeed.

What does this demonstrate?
~~~~~~~~~~~~~~~~~~~~~~~~~~~

-  The ability to invoke (alter key value states) is restricted to peers
   that have the chaincode installed.
-  Just as state changes due to invoke on a peer affects all peers in
   the channel, the instantiate on a peer will likewise affect all peers
   in the channel.
-  The world state of the chaincode is available to all peers on the
   channel - even those that do not have the chaincode installed.
-  Once the chaincode is installed on a peer, invokes and queries can
   access those states normally.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

