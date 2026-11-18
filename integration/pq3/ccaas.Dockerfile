FROM ubuntu:24.04

COPY chaincode /usr/local/bin/pq3-chaincode

USER 10001:10001

ENTRYPOINT ["/usr/local/bin/pq3-chaincode"]
