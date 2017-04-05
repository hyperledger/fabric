Manually create and join peers to a new channel
-----------------------------------------------

Use the cli container to manually exercise the create channel and join
channel APIs.

Channel - ``myc1`` already exists, so let's create a new channel named
``myc2``.

Exec into the cli container:

.. code:: bash

    docker exec -it cli bash

If successful, you should see the following in your terminal:

.. code:: bash

    /opt/gopath/src/github.com/hyperledger/fabric/peer #

Send createChannel API to Ordering Service:

::

    CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer channel create -c myc2

This will return a genesis block - ``myc2.block`` - that you can issue
join commands with. Next, send a joinChannel API to ``peer0`` and pass
in the genesis block as an argument. The channel is defined within the
genesis block:

::

    CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 CORE_PEER_ADDRESS=peer0:7051 peer channel join -b myc2.block

To join the other peers to the channel, simply reissue the above command
with ``peer1`` or ``peer2`` specified. For example:

::

    CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 CORE_PEER_ADDRESS=peer1:7051 peer channel join -b myc2.block

Once the peers have all joined the channel, you are able to issues
queries against any peer without having to deploy chaincode to each of
them.

Use cli to deploy, invoke and query
-----------------------------------

Run the deploy command. This command is deploying a chaincode named
``mycc`` to ``peer0`` on the Channel ID ``myc2``. The constructor
message is initializing ``a`` and ``b`` with values of 100 and 200
respectively.

::

    CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer chaincode deploy -C myc2 -n mycc -p github.com/hyperledger/fabric/examples -c '{"Args":["init","a","100","b","200"]}'

Run the invoke command. This invocation is moving 10 units from ``a`` to
``b``.

::

    CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer chaincode invoke -C myc2 -n mycc -c '{"function":"invoke","Args":["move","a","b","10"]}'

Run the query command. The invocation transferred 10 units from ``a`` to
``b``, therefore a query against ``a`` should return the value 90.

::

    CORE_PEER_ADDRESS=peer0:7051 CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer chaincode query -C myc2 -n mycc -c '{"function":"invoke","Args":["query","a"]}'

You can issue an ``exit`` command at any time to exit the cli container.

Creating your initial channel through the cli
---------------------------------------------

If you want to manually create the initial channel through the cli
container, you will need to edit the Docker Compose file. Use an editor
to open ``docker-compose-gettingstarted.yml`` and comment out the
``channel_test.sh`` command in your cli image. Simply place a ``#`` to
the left of the command. (Recall that this script is executing the
create and join channel APIs when you run ``docker-compose up``) For
example:

.. code:: bash

    cli:
      container_name: cli
      <CONTENT REMOVED FOR BREVITY>
      working_dir: /opt/gopath/src/github.com/hyperledger/fabric/peer
    #  command: sh -c './channel_test.sh; sleep 1000'
    #  command: /bin/sh

Then use the cli commands from above.
