#!/bin/sh

# find address of peer0 in your network
PEER0_IP_ADDRESS=`perl -e 'use Socket; $a = inet_ntoa(inet_aton("peer0")); print "$a\n";'`

# create an anchor file
cat<<EOF>anchorPeer.txt
$PEER0_IP_ADDRESS
7051
-----BEGIN CERTIFICATE-----
MIICjDCCAjKgAwIBAgIUBEVwsSx0TmqdbzNwleNBBzoIT0wwCgYIKoZIzj0EAwIw
fzELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNh
biBGcmFuY2lzY28xHzAdBgNVBAoTFkludGVybmV0IFdpZGdldHMsIEluYy4xDDAK
BgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhhbXBsZS5jb20wHhcNMTYxMTExMTcwNzAw
WhcNMTcxMTExMTcwNzAwWjBjMQswCQYDVQQGEwJVUzEXMBUGA1UECBMOTm9ydGgg
Q2Fyb2xpbmExEDAOBgNVBAcTB1JhbGVpZ2gxGzAZBgNVBAoTEkh5cGVybGVkZ2Vy
IEZhYnJpYzEMMAoGA1UECxMDQ09QMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE
HBuKsAO43hs4JGpFfiGMkB/xsILTsOvmN2WmwpsPHZNL6w8HWe3xCPQtdG/XJJvZ
+C756KEsUBM3yw5PTfku8qOBpzCBpDAOBgNVHQ8BAf8EBAMCBaAwHQYDVR0lBBYw
FAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHQYDVR0OBBYEFOFC
dcUZ4es3ltiCgAVDoyLfVpPIMB8GA1UdIwQYMBaAFBdnQj2qnoI/xMUdn1vDmdG1
nEgQMCUGA1UdEQQeMByCCm15aG9zdC5jb22CDnd3dy5teWhvc3QuY29tMAoGCCqG
SM49BAMCA0gAMEUCIDf9Hbl4xn3z4EwNKmilM9lX2Fq4jWpAaRVB97OmVEeyAiEA
25aDPQHGGq2AvhKT0wvt08cX1GTGCIbfmuLpMwKQj38=
-----END CERTIFICATE-----
EOF

#create
echo "Creating channel on Orderer"
CORE_PEER_GOSSIP_IGNORESECURITY=true CORE_PEER_MSPCONFIGPATH=/etc/hyperledger/fabric/msp/sampleconfig CORE_PEER_COMMITTER_LEDGER_ORDERER=orderer:7050 peer channel create -c myc1 -a anchorPeer.txt >>log.txt 2>&1
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
