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

- Note: Make sure peer is not using TLS when running in dev mode.

All commands are executed from the ``fabric`` folder.

Start the orderer
-----------------

::

    ORDERER_GENERAL_GENESISPROFILE=SampleDevModeSolo orderer

The above starts the orderer in the local environment the orderer
configuration as defined in ``sampleconfig/orderer.yaml`` with the
genesisprofile directive overridden to use the SampleDevModeSolo profile
for bootstrapping the network.

Start the peer in dev mode
--------------------------

::

    peer node start --peer-chaincodedev=true

The above command starts the peer using the default ``sampleconfig/msp``
MSP. The ``--peer-chaincodedev=true`` puts it in “dev” mode.

Create channels ch1 and ch2
---------------------------

Generate the transactions for creating the channels using ``configtxgen`` tool.

::
   configtxgen -channelID ch1 -outputCreateChannelTx ch1.tx -profile SampleSingleMSPChannel
   configtxgen -channelID ch2 -outputCreateChannelTx ch2.tx -profile SampleSingleMSPChannel

where SampleSingleMSPChannel is a channel profile in ``sampleconfig/configtx.yaml``

::

    peer channel create -o 127.0.0.1:7050 -c ch1 -f ch1.tx
    peer channel create -o 127.0.0.1:7050 -c ch2 -f ch2.tx

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
    CORE_CHAINCODE_LOGLEVEL=debug CORE_PEER_ADDRESS=127.0.0.1:7052 CORE_CHAINCODE_ID_NAME=mycc:0 ./chaincode_example02

The chaincode is started with peer and chaincode logs indicating successful registration with the peer.
Note that at this stage the chaincode is not associated with any channel. This is done in subsequent steps
using the ``instantiate`` command.

Use the chaincode
-----------------

Even though you are in ``--peer-chaincodedev`` mode, you still have to install the chaincode so the life-cycle system
chaincode can go through its checks normally. This requirement may be removed in future when in ``--peer-chaincodedev``
mode.

::

    peer chaincode install -n mycc -v 0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02

Once installed, the chaincode is ready to be instantiated.

::

    peer chaincode instantiate -n mycc -v 0 -c '{"Args":["init","a","100","b","200"]}' -o 127.0.0.1:7050 -C ch1

    peer chaincode instantiate -n mycc -v 0 -c '{"Args":["init","a","100","b","200"]}' -o 127.0.0.1:7050 -C ch2

The above instantiates the chaincode with the two channels. With default
settings it might take a few seconds for the transactions to be
committed.

::

    peer chaincode invoke -n mycc -c '{"Args":["invoke","a","b","10"]}' -o 127.0.0.1:7050 -C ch1
    peer chaincode invoke -n mycc -c '{"Args":["invoke","a","b","10"]}' -o 127.0.0.1:7050 -C ch2

The above invoke the chaincode over the two channels.

Finally, query the chaincode on the two channels.

::

    peer chaincode query -n mycc -c '{"Args":["query","a"]}' -o 127.0.0.1:7050 -C ch1
    peer chaincode query -n mycc -c '{"Args":["query","a"]}' -o 127.0.0.1:7050 -C ch2

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

