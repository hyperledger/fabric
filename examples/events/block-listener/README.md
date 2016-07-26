# What is block-listener
block-listener.go will connect to a peer and recieve blocks. For every transaction in the block, it will test the ErrorCode field and print success/failure.

# To Run
1. go build

2. ./block-listener -events-address=< event address >

# Example with PBFT

## Run 4 docker peers with PBFT
docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp0 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_PEER_VALIDATOR_CONSENSUS_PLUGIN=pbft hyperledger/fabric-peer peer node start

docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp1 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_PEER_DISCOVERY_ROOTNODE=172.17.0.2:30303 -e CORE_PEER_VALIDATOR_CONSENSUS_PLUGIN=pbft hyperledger/fabric-peer peer node start

docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp2 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_PEER_DISCOVERY_ROOTNODE=172.17.0.2:30303 -e CORE_PEER_VALIDATOR_CONSENSUS_PLUGIN=pbft hyperledger/fabric-peer peer node start

docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp3 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_PEER_DISCOVERY_ROOTNODE=172.17.0.2:30303 -e CORE_PEER_VALIDATOR_CONSENSUS_PLUGIN=pbft hyperledger/fabric-peer peer node start

## Attach event client to a Peer
./block-listener -events-address=172.17.0.2:31315

Event client should output "Event Address: 172.17.0.2:31315" and wait for events.

## Create a deploy transaction
Submit a transaction to deploy chaincode_example02.

CORE_PEER_ADDRESS=172.17.0.2:30303 peer chaincode deploy -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Function":"init", "Args": ["a","100", "b", "200"]}'

Notice success transaction in the events client.

## Create an invoke transaction - good
Send a valid invoke transaction to chaincode_example02.

CORE_PEER_ADDRESS=172.17.0.2:30303 peer chaincode invoke -n 1edd7021ab71b766f4928a9ef91182c018dffb86fef7a4b5a5516ac590a87957e21a62d939df817f5105f524abddcddfc7b1a60d780f02d8235bd7af9db81b66 -c '{"Function":"invoke", "Args": ["a","b","10"]}'

Notice success transaction in events client.

## Create an invoke transaction - bad
Send an invoke transaction with invalid parameters to chaincode_example02.

CORE_PEER_ADDRESS=172.17.0.2:30303 peer chaincode invoke -n 1edd7021ab71b766f4928a9ef91182c018dffb86fef7a4b5a5516ac590a87957e21a62d939df817f5105f524abddcddfc7b1a60d780f02d8235bd7af9db81b66 -c '{"Function":"invoke", "Args": ["a","b"]}'

Notice error transaction in events client.
