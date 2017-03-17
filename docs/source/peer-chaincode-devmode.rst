Using dev mode
==============

Normally chaincodes are started and maintained by peer. However in “dev”
mode, chaincode is built and started by the user. This mode is useful
during chaincode development phase for rapid code/build/run/debug cycle
turnaround.

To keep this a realistic “dev” environment, we are going to keep it “out
of the box” - with one exception: we create two channels instead of
using the default ``testchainid`` channel to show how the single running
instance can be accessed from multiple channels.

Start the orderer
-----------------

::

    orderer

The above starts the orderer in the local environment using default
orderer configuration as defined in ``orderer/orderer.yaml``.

Start the peer in dev mode
--------------------------

::

    peer node start --peer-defaultchain=false --peer-chaincodedev=true

The above command starts the peer using the default ``msp/sampleconfig``
MSP. The ``--peer-chaincodedev=true`` puts it in “dev” mode. ##Create
channels ch1 and ch2

::

    peer channel create -o 127.0.0.1:7050 -c ch1
    peer channel create -o 127.0.0.1:7050 -c ch2

Above assumes orderer is reachable on ``127.0.0.1:7050``. The orderer
now is tracking channels ch1 and ch2 for the default configuration.

::

    peer channel join -b ch1.block
    peer channel join -b ch2.block

The peer has now joined channels cha1 and ch2.

Start the chaincode
-------------------

::

    cd examples/chaincode/go/chaincode_example02
    go build
    CORE_CHAINCODE_LOGLEVEL=debug CORE_PEER_ADDRESS=127.0.0.1:7051 CORE_CHAINCODE_ID_NAME=mycc:0 ./chaincode_example02

The chaincode is started with peer and chaincode logs showing it got
registered successfully with the peer.

Use the chaincode
-----------------

::

    peer chaincode instantiate -n mycc -v 0 -c '{"Args":["init","a","100","b","200"]}' -o 127.0.0.1:7050 -C ch1

    peer chaincode instantiate -n mycc -v 0 -c '{"Args":["init","a","100","b","200"]}' -o 127.0.0.1:7050 -C ch2

The above instantiates the chaincode with the two channels. With default
settings it might take a few seconds for the transactions to be
committed.

::

    peer chaincode invoke -n mycc -c '{"Args":["invoke","a","b","10"]}' -o 127.0.0.1:7050 -C ch1
    peer chaincode invoke -n mycc -c '{"Args":["invoke","a","b","10"]}' -o 127.0.0.1:7050 -C ch2

The above invokes the chaincode using the two channels.

::

    peer chaincode query -n mycc -c '{"Args":["query","a"]}' -o 127.0.0.1:7050 -C ch1
    peer chaincode invoke -n mycc -c '{"Args":["query","a"]}' -o 127.0.0.1:7050 -C ch2
