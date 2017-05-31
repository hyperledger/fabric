# Orderer Traffic Engine (OTE)

## What does OTE do?

+ This Orderer Traffic Engine (OTE) tool creates and tests the operation of a
hyperledger fabric ordering service.
+ The focus is strictly on the orderers themselves.
No peers are involved: no endorsements or validations or committing to ledgers.
No SDK is used.

+ OTE sends transactions to
every channel on every orderer, and verifies that the correct number
of transactions and blocks are delivered on every channel from every orderer.
+ OTE generates report logs and returns
a pass/fail boolean and a resultSummaryString.

## How does OTE do it?

+ OTE invokes a local copy of the tool driver.sh (including helper files
network.json and json2yml.js) -
which is a close copy of the original version at
https://github.com/dongmingh/v1FabricGenOption.
+ The driver.sh launches an orderer service network per the user-provided
parameters including number of orderers, orderer type,
number of channels, and more.
+ Producer clients are created to connect via
grpc ports to the orderers to concurrently send traffic until the
requested number of transactions are sent.
Each client generates unique transactions - a fraction of the total
requested number of transactions.
+ Consumer clients are created to connect via
grpc ports to the orderers to concurrently receive delivered traffic
until all batches of transactions are tallied.
OTE checks if the correct number of blocks and TXs are delivered
by all the orderers on all the channels

## Prerequisites
- <a href="https://git-scm.com/downloads" target="_blank">Git client</a>
- <a href="https://www.docker.com/products/overview" target="_blank">Docker v1.12 or higher</a>
- [Docker-Compose v1.8 or higher](https://docs.docker.com/compose/overview/)
- GO

Check your Docker and Docker-Compose versions with the following commands:
```bash
    docker version
    docker-compose version
```

### Prepare binaries and images:

- Alternative 1: Prepare all binaries and images using a script
```bash
      cd $GOPATH/src/github.com/hyperledger/fabric/bddtests/regression/ote
      ./docker_images.sh
```

- Alternative 2: Prepare binaries and images manually
- - Clone the fabric repository, build the binaries and images
```bash
        cd $GOPATH/src/github.com/hyperledger/fabric
        make native docker
```
- - Clone the fabric-ca repository, build the images
```bash
        cd $GOPATH/src/github.com/hyperledger/

        # Use ONE of these methods to clone the repository:
            go get github.com/hyperledger/fabric-ca
            git clone https://github.com/hyperledger/fabric-ca.git
            git clone ssh://YOUR-ID@gerrit.hyperledger.org:29418/fabric-ca

        cd $GOPATH/src/github.com/hyperledger/fabric-ca
        make docker
```

### Environment Variables for test setup, with defaults:
```
  OTE_TXS                                      55
  OTE_CHANNELS                                 1
  OTE_ORDERERS                                 1
  OTE_KAFKABROKERS                             0
  OTE_MASTERSPY                                false
  OTE_PRODUCERS_PER_CHANNEL                    1
```

### Environment Variables for configuration
Find default values of all variables in hyperledger/fabric/sampleconfig/orderer.yaml
and hyperledger/fabric/sampleconfig/core.yaml.
```
  CONFIGTX_ORDERER_ORDERERTYPE                 solo
  CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT   10
  CONFIGTX_ORDERER_BATCHTIMEOUT                10

  Others:
  CORE_LOGGING_LEVEL                           <unset>
  CORE_LEDGER_STATE_STATEDATABASE              leveldb
  CORE_SECURITY_LEVEL                          256
  CORE_SECURITY_HASHALGORITHM                  SHA2
```

## Execute OTE on shell command line
There are several environment variables to control the test parameters,
such as number of transactions, number of orderers, ordererType, and more.
To see an example test using default settings, simply execute the following.
```bash
  cd $GOPATH/src/github.com/hyperledger/fabric/bddtests/regression/ote
  go build
  ./ote
  CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT=20 ./ote
  OTE_TXS=100  OTE_CHANNELS=4  ./ote
  CONFIGTX_ORDERER_ORDERERTYPE=kafka OTE_KAFKABROKERS=3  ./ote
```

Choose which variables to modify from default values. For example:

+ This test will create eight Producer clients.
+ Each Producer will send 125 transactions to a different orderer and channel.
+ 250 total TXs will be broadcast on each channel.
+ 500 total TXs will be broadcast to each orderer.
+ Four Consumer clients will be created to receive the delivered
  batches on each channel on each orderer.
+ 50 batches (with 10 TX each) will be delivered on channel 0, and
  a different 50 batches will be delivered on channel 1. On both Orderers.
+ 100 batches will be received on every orderer; this is the sum of the
  totals received on each channel on the orderer.
```bash
  OTE_TXS=1000 OTE_CHANNELS=4 OTE_ORDERERS=2 CONFIGTX_ORDERER_ORDERERTYPE=kafka  ./ote
```

## Execute OTE GO Tests
The tester may optionally define environment variables to
set the test parameters and to
override certain orderer configuration parameters.
Then use "go test" to execute Test functions
to execute either one test, or all go tests, or
a subset of existing functional go tests using a regular expression
to choose tests in local test files.
```bash
  cd $GOPATH/src/github.com/hyperledger/fabric/bddtests/regression/ote
  go test -run ORD77
  go test -run ORD7[79]
  go test -run batchSz -timeout 20m
  go test -timeout 90m

  go get github.com/jstemmer/go-junit-report
  go test -run ORD7 -v | go-junit-report > report.xml

```

## Execute OTE GO Tests for Continuous Improvement
Optionally, one can translate the "go test" output to xml for reports.
This is useful for automated test suites that are automatically
executed from Jenkins by Continuous Improvement processes.

#### Pre-requisite to convert "go test" output to xml
```bash
  cd $GOPATH/src/github.com/hyperledger/fabric/bddtests/regression/ote
  go get github.com/jstemmer/go-junit-report
```
#### Example command to execute all "go tests" and convert to xml:
```
  cd $GOPATH/src/github.com/hyperledger/fabric/bddtests/regression/ote
  go test -v -timeout 120m | go-junit-report > ote_report.xml
```

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
