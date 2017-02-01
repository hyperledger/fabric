#!/bin/sh

#create
echo "Creating channel on Orderer"
CORE_PEER_MSPCONFIGPATH=/etc/hyperledger/fabric/msp/sampleconfig CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer channel create -c myc1 >>log.txt 2>&1
cat log.txt
   grep -q "Exiting" log.txt
   if [ $? -ne 0 ]; then
      echo "ERROR on CHANNEL CREATION" >> results.txt
      exit 1
   fi
echo "SUCCESSFUL CHANNEL CREATION" >> results.txt
sleep 5
TOTAL_PEERS=3
i=0
while test $i -lt $TOTAL_PEERS  
do
echo "###################################### Joining peer$i"
CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 CORE_PEER_ADDRESS=peer$i:7051 peer channel join -b myc1.block >>log.txt 2>&1
cat log.txt
echo '-------------------------------------------------'
grep -q "Join Result: " log.txt
   if [ $? -ne 0 ]; then
      echo "ERROR on JOIN CHANNEL" >> results.txt
      exit 1
   fi
echo "SUCCESSFUL JOIN CHANNEL on PEER$i" >> results.txt
echo "SUCCESSFUL JOIN CHANNEL on PEER$i"
i=$((i+1))
sleep 10
done
echo "Peer0 , Peer1 and Peer2 are added to the channel myc1"
cat log.txt
exit 0
