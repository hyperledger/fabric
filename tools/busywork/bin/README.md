# bin

This directory contains several executable scripts and a
[Makefile](Makefile). You may find it *useful* to put this directory in your
**PATH**, however this is never *required* for the **busywork** components to
work correctly.

Many simple "scripts" are actually implemented as make targets, and are
documeted at the top of the Makefile. Make targets are provided for various
types of "cleaning", building Docker images, testing, starting Hyperledger
fabric networks and obtaining logs from containers. Some of the make targets
are similar to those found in the fabric/Makefile, however are defined with a
slightly different sensibility.

## Major Scripts

The following major support scripts are provided. These scripts are called out
by other major scripts or test drivers. Scripts are typically documented in a
*usage* string embedded at the top of the script.

* [busy](busy) is a script that can be used to execute operations on any peer
  network defined by a **busywork** `network` file. Currently the chaincode
  `deploy`, `invoke` and `query` operations are supported, as well as HTTP GET
  operations using the REST API.

* [checkAgreement](checkAgreement) is a script that uses the
  [fabricLogger](fabricLogger) (see below) to check that all of the peers in a
  network agree on the contents of the blockchain.

* [fabricLogger](fabricLogger) is a simple script that converts a blockchain
  into a text file listing of transaction IDs.

* [networkStatus](networkStatus) is a script that reports on the status of
  Hyperledger fabric peer networks created with **busywork** tools, with an
  option to be used as a background *watchdog* on the health of the network.

* [pprofClient](pprofClient) is a script that takes profiles from the
  profiling server built into the Hyperledger fabric peer.

* [userModeNetwork](userModeNetwork) is a script that defines Hyperledger
  fabric peer networks as simple networks of user-mode processes, that is,
  networks that do not rely on Docker-compose, or multiple physical or virtual
  nodes. The concept of a user-mode network is discussed
  [below](#userModeNetwork).

* [wait-for-it](wait-for-it) is a general-purpose script that waits for a
  network service to be available with an optional timeout. **busywork**
  applications use `wait-for-it` to correctly sequence network startup.


<a name="userModeNetwork"></a>
## User-Mode Networks

A *user-mode network* is a Hyperledger fabric peer network in which the peers
in the network are simple user-mode processes. Running a user-mode network
does not require *root* privledges, or depend on Docker or any other
virtualization technology ([caveats](#caveats)). The script that sets up these
networks, [userModeNetwork](userModeNetwork), configures the peers such that
their network service interfaces, databases and logging directories are
unique. A description of the [network configuration](#network) is written to
`$BUSYWORK_HOME/network`, and the **busywork** tools refer to this configuration
by default in order to examine and control the network. The databases and
logging directories are stored in the [$BUSYWORK_HOME](#busywork-home)
directory.

Starting a user-mode network of 10 peers running PBFT batch consensus is as as
simple as

    userModeNetwork 10

Several useful options for setting up the network are also provided. Here is
an example of setting up a 4-node network with security, PBFT Batch consensus
and DEBUG-level logging in the peers

    userModeNetwork -security -batch -peerLogging debug 4

Normally the peer services are available on all system network interfaces
(i.e., interface 0.0.0.0). The `-interface` option can be used to limit
network services to a single interface, and can also be useful for creating
networks (and `network` files) that will be accessed by client drivers running
on remote systems.

    userModeNetwork -interface `hostname` 10

Another advantage of a user-mode network can be that the peer processes are
executed in the environment of the call of
[userModeNetwork](userModeNetwork). This makes it very easy to override
default configuration parameters from the command line or from a script. For
example

    CORE_SECURITY_TCERT_BATCH_SIZE=1000 userModeNetwork -security -batch 4


<a name="caveats"></a>
## User-Mode Caveats

Docker is still currently required for chaincode containers, so currently
users are still required to have *root* access and/or sudo-less permission
to control Docker. One goal of this project is to completely eliminate the
*requirement* for Docker from the development and test environment however,
and allow networks to be reliably set up and tested by normal users sharing a
development server.

User-mode networks are not secure. Any user on the host machine can access or
disrupt the network by targeting the service ports. We don't see this to be a
major issue for typical development and test environments however.


The mechanism used to determine when the **membersrvc** server is up and
running causes a (harmless) message like the following to appear at the top of
the log file:

    2016/05/19 08:23:40 grpc: Server.Serve failed to create ServerTransport:  connection error: desc = "transport: write tcp 127.0.0.1:7054->127.0.0.1:49084: write: broken pipe"


<a name=busywork-home></a>
## BUSYWORK_HOME

If **BUSYWORK_HOME** is not defined in the environment, **busywork** scripts
and applications create (if necessary) and use `~/.busywork` as the
**BUSYWORK_HOME** directory. The following directories and files will be found
in the **BUSYWORK_HOME** depending on which **busywork** tools are being
used.

* `chaincodes` This file lists chaincode deployments. The structure of the
  file is described [below](#chaincodes).

* `dev-vp*-*` If you excute the `make cclogs` target of the
  [Makefile](Makefile) the chaincode logs will be dumped into these
  directories.

* `fabricLog` This is a text representation of the blockchain created by the
  [fabricLogger](fabricLogger) process that driver scripts invoke to validate
  transaction execution.

* `membersrvc/` This directory contains the **membersrvc** database (`/data` -
  TBD) and the `/stderr` and `/stdout` logs from the **membersrvc** server.

* `network` The network configuration is described [below](#network).

* `pprof.*` If you use the [pprofClient](pprofClient) to collect profiles the
  profile files will be placed here by default.

* `vp[0,...N]/` These directories contain the validating peer databases
  `/data` and their `/stderr` and `/stdout` logs.

* `*.chain` These files are created by the [checkAgreement](checkAgreement)
  script, and represent the blockchains as recorded on the different peers in
  the network.

<a name="network"></a>
## *busywork* Network Configurations

**busywork** scripts that create peer networks produce a description of the
  network for the benefit of other **busywork** tools. The network description
  is written as `$BUSYWORK_HOME/network`, and contains a single JSON
  object. This is still a work in progress so some fields may be changed
  and/or added in the future, but this is an example of what one of these
  network descriptions looks like at present.

```
{
    "networkMode": "user",
    "chaincodeMode": "vm",
    "host": "local",
    "date": "2016/05/27+08:15:45",
    "createdBy": "userModeNetwork",
    "security": "true",
    "consensus": "batch",
    "peerProfileServer": "true",
    "membersrvc":
        { "service": "0.0.0.0:7054",
          "pid": "93244"
        },
    "peers": [
        { "id": "vp0",
          "grpc": "0.0.0.0:42580",
          "rest": "0.0.0.0:38048",
          "events": "0.0.0.0:45311",
          "cli": "0.0.0.0:44155",
          "profile": "0.0.0.0:44024",
          "pid": "93269"
        }
        ,
        { "id": "vp1",
          "grpc": "0.0.0.0:34801",
          "rest": "0.0.0.0:43718",
          "events": "0.0.0.0:41947",
          "cli": "0.0.0.0:43596",
          "profile": "0.0.0.0:40860",
          "pid": "93273"
        }
        ,
        { "id": "vp2",
          "grpc": "0.0.0.0:39656",
          "rest": "0.0.0.0:36277",
          "events": "0.0.0.0:36991",
          "cli": "0.0.0.0:40268",
          "profile": "0.0.0.0:34522",
          "pid": "93281"
        }
        ,
        { "id": "vp3",
          "grpc": "0.0.0.0:36270",
          "rest": "0.0.0.0:42204",
          "events": "0.0.0.0:41790",
          "cli": "0.0.0.0:44141",
          "profile": "0.0.0.0:45035",
          "pid": "93289"
        }
    ]
}
```

<a name="chaincodes"></a>
## *busywork* Chaincode Deployments

**busywork** scripts that deploy chaincodes to a peer network produce a
  description of the deployments for the benefit of other **busywork**
  tools. The deployment information is written as `$BUSYWORK_HOME/chaincodes`,
  and contains a single JSON object. This is still a work in progress so some
  fields may be changed and/or added in the future, but this is an example of
  what one of these chaincode deployment descriptions looks like at present.
```
{
    "cc0"     : {
        "name"     : "a3a4756cd347784d15262a5839838768cd1ef8d6f853893e64651a788c30c409d1a0e645c230c2a3142abed8699f3435ce5a5bacc1282d630866b949f69d6773",
        "path"     : "github.com/hyperledger/fabric/tools/busywork/counters",
        "function" : "parms",
        "args"     : ["-id","cc0"]
    },
    "cc1" : {
        "name"     : "52676ae63ebf205f0c9a2dba111a2ac75afaf62075e43b21e04ccbf29ce2e9661a6eb9963cc29cf2f38e9b12fff0c00fcb9b9e98c0fb342bb5a7eb299c6dc938",
        "path"     : "github.com/hyperledger/fabric/tools/busywork/counters",
        "function" : "parms",
        "args"     : ["-id","cc1"]
    }
}
```
