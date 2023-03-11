Logging Control
===============

Overview
--------

Logging in the ``peer`` and ``orderer`` is provided by the
``common/flogging`` package. This package supports

-  Logging control based on the severity of the message
-  Logging control based on the software *logger* generating the message
-  Different pretty-printing options based on the severity of the
   message

All logs are currently directed to ``stderr``. Global and logger-level
control of logging by severity is provided for both users and developers.
There are currently no formalized rules for the types of information
provided at each severity level. When submitting bug reports, developers
may want to see full logs down to the DEBUG level.

In pretty-printed logs the logging level is indicated both by color and
by a four-character code, e.g, "ERRO" for ERROR, "DEBU" for DEBUG, etc. In
the logging context a *logger* is an arbitrary name (string) given by
developers to groups of related messages. In the pretty-printed example
below, the loggers ``ledgermgmt``, ``kvledger``, and ``peer`` are
generating logs.

::

   2018-11-01 15:32:38.268 UTC [ledgermgmt] initialize -> INFO 002 Initializing ledger mgmt
   2018-11-01 15:32:38.268 UTC [kvledger] NewProvider -> INFO 003 Initializing ledger provider
   2018-11-01 15:32:38.342 UTC [kvledger] NewProvider -> INFO 004 ledger provider Initialized
   2018-11-01 15:32:38.357 UTC [ledgermgmt] initialize -> INFO 005 ledger mgmt initialized
   2018-11-01 15:32:38.357 UTC [peer] func1 -> INFO 006 Auto-detected peer address: 172.24.0.3:7051
   2018-11-01 15:32:38.357 UTC [peer] func1 -> INFO 007 Returning peer0.org1.example.com:7051

An arbitrary number of loggers can be created at runtime, therefore there is
no "global list" of loggers, and logging control constructs can not check
whether logging loggers actually do or will exist.

Logging specification
---------------------

The logging levels of the ``peer`` and ``orderer`` commands are controlled
by a logging specification, which is set via the ``FABRIC_LOGGING_SPEC``
environment variable.

The full logging level specification is of the form

::

    [<logger>[,<logger>...]=]<level>[:[<logger>[,<logger>...]=]<level>...]

Logging severity levels are specified using case-insensitive strings
chosen from

::

   FATAL | PANIC | ERROR | WARNING | INFO | DEBUG


A logging level by itself is taken as the overall default. Otherwise,
overrides for individual or groups of loggers can be specified using the

::

    <logger>[,<logger>...]=<level>

syntax. Examples of specifications:

::

    info                                        - Set default to INFO
    warning:msp,gossip=warning:chaincode=info   - Default WARNING; Override for msp, gossip, and chaincode
    chaincode=info:msp,gossip=warning:warning   - Same as above

.. note:: Logging specification terms are separated by a colon. If a term does not include a specific logger, for example `info:` then it is applied as the default log level
   across all loggers on the component. The string `info:dockercontroller,endorser,chaincode,chaincode.platform=debug` sets
   the default log level to `INFO` for all loggers and then the `dockercontroller`, `endorser`, `chaincode`, and
   `chaincode.platform` loggers are set to `DEBUG`. The order of the terms does not matter. In the examples above,
   the second and third options produce the same result although the order of the terms is reversed.

Logging format
--------------

The logging format of the ``peer`` and ``orderer`` commands is controlled
via the ``FABRIC_LOGGING_FORMAT`` environment variable. This can be set to
a format string, such as the default

::

   "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"

to print the logs in a human-readable console format. It can be also set to
``json`` to output logs in JSON format.

Typical debug levels
--------------------

This section provides some typical log levels to debug various areas of a peer node or ordering service node by setting the ``FABRIC_LOGGING_SPEC`` environment variable.

Peer smart contract debug:

::

    FABRIC_LOGGING_SPEC=info:dockercontroller,endorser,chaincode,chaincode.platform=debug

Peer private data debug:

::

    FABRIC_LOGGING_SPEC=info:kvledger,ledgerstorage,transientstore,pvtdatastorage,gossip.privdata=debug

Peer ledger and state database debug:

::

    FABRIC_LOGGING_SPEC=info:kvledger,lockbasedtxmgr,ledgerstorage,stateleveldb,statecouchdb,couchdb=debug

Peer full debug, with the noisy components set to info:

::

    FABRIC_LOGGING_SPEC=debug:cauthdsl,policies,msp,grpc,peer.gossip.mcs,gossip,leveldbhelper=info

Ordering node full debug, with the noisy components set to info:

::

    FABRIC_LOGGING_SPEC=debug:cauthdsl,policies,msp,grpc,orderer.consensus.etcdraft,orderer.common.cluster,orderer.common.cluster.step,common.configtx,blkstorage=info


Chaincode
---------

**Chaincode logging is the responsibility of the chaincode developer.**

As independently executed programs, user-provided chaincodes may technically
also produce output on stdout/stderr. While naturally useful for “devmode”,
these channels are normally disabled on a production network to mitigate abuse
from broken or malicious code. However, it is possible to enable this output
even for peer-managed containers (e.g. “netmode”) on a per-peer basis
via the CORE_VM_DOCKER_ATTACHSTDOUT=true configuration option.

Once enabled, each chaincode will receive its own logging channel keyed by its
container-id. Any output written to either stdout or stderr will be integrated
with the peer’s log on a per-line basis. It is not recommended to enable this
for production.

Stdout and stderr not forwarded to the peer container can be viewed from the
chaincode container using standard commands for your container platform.

::

    docker logs <chaincode_container_id>
    kubectl logs -n <namespace> <pod_name>
    oc logs -n <namespace> <pod_name>



.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
