# busywork

**busywork** is an exerciser framework for the
[Hyperledger fabric](https://github.com/hyperledger/fabric) project. As an
*exerciser*, **busywork** is not a real blockchain application, but is simply
a set of scripts, programs and utilities for stressing the blockchain fabric
in various, often randomized ways - hence the name. **busywork** applications
can be used both for correctness and performance testing, as well as for
simple benchmarks.

**busywork** is very much a work-in-progress. The effort to date has been
focussed on performance characterization, with correctness issues being a
natural fallout from stress testing.

## _**Use at Your Own Risk**_

As a system-level test environment, **busywork** assumes complete control over
Hyperledger fabric processes, containers and databases.  This environment has
been designed for a single user running in a VM or on a private system, or at
least a system where no other users are running the Hyperledger Fabric or
Docker containers. Outside of this type of environment, certain parts of
**busywork** may disrupt or destroy the work of others if used without
modification. _Caveat Emptor_.

## Prerequisites

If you are using the standard
[Vagrant environment](../../docs/dev-setup/devenv.md) then everything you need
has already been done. If you are running outside of Vagrant, some notes on
prerequisite packages can be found [here](prerequisites.md).

**busywork** scripts and procedures are written for users with password-less
`sudo` access, and `sudo`-less control of Docker. If you don't want to run as
**root**, consult your Linux distribution documentation for how to set up
password-less `sudo` access, and the Docker documentation for how to set up
`sudo`-less Docker access. The latter may be as simple as executing

    sudo usermod -a -G docker <username>
	

## What's Included?

* The [bin](bin/README.md) directory provides scripts and other support
used by all of the **busywork** applications, including numerous **make**
targets for setting up peer networks in various ways. You might find it useful
to put this directory on your **PATH**.

* The [busywork](busywork/README.md) directory defines a small Go package used
by **busywork** applications.

* The [benchmarks](benchmarks/README.md) directory contains the beginnings of
  a set of microbenchmarks for cryptographic primitives used by the
  Hyperledger fabric.

* The [tcl](tcl/README.md) directory defines Tcl packages.

The following applications (chaincodes) are currently provided. Each
application is documented separately.

* [counters](counters/README.md) is a chaincode that manages variable-sized
  arrays of counters. This provides for a simple but flexible self-checking
  test and benchmark environment. Make targets documented in the
  [Makefile](counters/Makefile) use a [driver](counters/driver) script to
  exercise the chaincode.

## BUSYWORK_HOME

**busywork** needs a well-known directory for log files and other
  configuration files that are generated dynamically. The **BUSYWORK_HOME**
  environment variable names this directory. If **BUSYWORK_HOME** is not
  defined, **busywork** scripts and applications create (if necessary) and use
  `~/.busywork` as the **BUSYWORK_HOME**. To avoid file name collisions, etc.,
  it is probably best to create a dedicated directory for **BUSYWORK_HOME** if
  you decide to define your own. The contents of **BUSYWORK_HOME** are
  described [here](bin/README.md#busywork-home).

## Getting Started

Change to the `hyperledger/fabric/tools/busywork/bin` directory and

    make build
	
to build the `peer` and `membersrvc` images. Then switch to the 
`busywork/counters` directory and 

    make stress1
	
to run a simple stress test. This stress test exercises a single peer
running `NOOPS` consensus.

At present the PBFT *batch* algorithm is the only true consensus algorithm
supported by the development team. The targets

    make stress2b
	make sweep1b
	
both test 4-peer PBFT *batch* networks. The `stress2b` test is a single run
with a single client. The `sweep1b` test sweeps a test setup over a range of
from 1 to 64 clients. At present the `sweep1b` target appears to work in
server environments, but fails in Vagrant/laptop environments.

There is also a pre-canned target

    make secure1
	
that runs a 4-peer PBFT *batch* network with security.

The **busywork/counters/Makefile** allows you to define private targets in a
private makefile, **private.mk**, which is included by the main Makefile. For
example you might define a modification of the `stress2b` target in
**private.mk** to see what happens when the data arrays go from the default 1
counter (8 bytes) to 1000 counters (8000 bytes):

    .PHONY: stress2b1k
	stress2b1k:
            @(STRESS2_CONFIG) $(NETWORK) -batch 4
	        @$(STRESS2) -size 1000
	
## Running Peers and Clients on Separate Machines

All of the current **busywork** make targets create peer networks and clients
drivers on the same system. With a little extra work it is possible to run
**busywork** tools and client drivers targeting arbitrary peer networks. The
key is that the **busywork** tools and client drivers read the
[`$BUSYWORK_HOME/network`](bin/README.md#network) and
[`$BUSYWORK_HOME/chaincodes`](bin/README.md#chaincodes) files to understand
the peer network configuration.  As long as you or your network setup script
create a suitable `network` file, and you then place that file in the
`$BUSYWORK_HOME` directory of the client machine, things will work - subject
to a few simple considerations discussed below.

### Network Setup Notes

If you want to set up a remote network using the
[`userModeNetwork`](bin/userModeNetwork) script, then you will probably want
to use the `-interface` option on the script so that the generated `network`
file will be usable on the client system. For example,

    userModeNetwork -interface `hostname` -batch 4
	
will create a `network` file where all of the network interfaces are assigned
based on the host name of the system. Assuming this host system is accesible
from the cliant machine you can then simply copy the `network` file from
host to client machine.

### Client Driver Notes

First, when running a client driver like the `busywork/counters/driver` script,
you must pass the `-remote` option to the script. This squashes the default
network watchdog process, which assumes the network is running on the local
machine. 

A broader issue may be that **busywork** tests are normally one-shot tests
that start from a clean state, run, check the results, and are done. If
setting up the peer network is time-consuming, you may want to "re-use" the
peer network for multiple tests. This is possible in several different ways as
explained below.

Normally a client driver like the `busywork/counters/driver` script deploys
chaincodes and then exercises them, every time it runs. However the script
also includes a `-noDeploy` option that can be used once chaincodes have
already been deployed in the network by an initial test. In this case the
script looks at the `$BUSYWORK_HOME/chaincodes` file to determine which
chaincodes are already available for targeting. All of the correctness
checking is designed to work under these circumstances.

There is also nothing to prevent running multiple copies of a client driver
against a single network, other then name collisions that will occur if
multiple drivers deploy the same chaincodes with the same parameters. The 
`busywork/counters/driver` now includes the `-ccPrefix` option that allows
each of multiple client drivers to manipulate chaincodes unique to that
driver. 

There is currently no support for having multiple client drivers targeting the
same set of chaincodes (e.g., using `-noDeploy`). The blockchain will
function, but the corrcetness checks will currently fail.
