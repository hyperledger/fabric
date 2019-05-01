Logging Control
===============

Overview
--------

Logging in the ``peer`` and ``orderer`` is provided by the
``common/flogging`` package. Chaincodes written in Go also use this
package if they use the logging methods provided by the ``shim``.
This package supports

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
no "master list" of loggers, and logging control constructs can not check
whether logging loggers actually do or will exist.

Logging specification
----

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

Logging format
----

The logging format of the ``peer`` and ``orderer`` commands is controlled
via the ``FABRIC_LOGGING_FORMAT`` environment variable. This can be set to
a format string, such as the default

::

   "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"

to print the logs in a human-readable console format. It can be also set to
``json`` to output logs in JSON format.


Go chaincodes
-------------

The standard mechanism to log within a chaincode application is to
integrate with the logging transport exposed to each chaincode instance
via the peer. The chaincode ``shim`` package provides APIs that allow a
chaincode to create and manage logging objects whose logs will be
formatted and interleaved consistently with the ``shim`` logs.

As independently executed programs, user-provided chaincodes may
technically also produce output on stdout/stderr. While naturally useful
for "devmode", these channels are normally disabled on a production
network to mitigate abuse from broken or malicious code. However, it is
possible to enable this output even for peer-managed containers (e.g.
"netmode") on a per-peer basis via the
CORE\_VM\_DOCKER\_ATTACHSTDOUT=true configuration option.

Once enabled, each chaincode will receive its own logging channel keyed
by its container-id. Any output written to either stdout or stderr will
be integrated with the peer's log on a per-line basis. It is not
recommended to enable this for production.

API
~~~

``NewLogger(name string) *ChaincodeLogger`` - Create a logging object
for use by a chaincode

``(c *ChaincodeLogger) SetLevel(level LoggingLevel)`` - Set the logging
level of the logger

``(c *ChaincodeLogger) IsEnabledFor(level LoggingLevel) bool`` - Return
true if logs will be generated at the given level

``LogLevel(levelString string) (LoggingLevel, error)`` - Convert a
string to a ``LoggingLevel``

A ``LoggingLevel`` is a member of the enumeration

::

    LogDebug, LogInfo, LogNotice, LogWarning, LogError, LogCritical

which can be used directly, or generated by passing a case-insensitive
version of the strings

::

    DEBUG, INFO, NOTICE, WARNING, ERROR, CRITICAL

to the ``LogLevel`` API.

Formatted logging at various severity levels is provided by the
functions

::

    (c *ChaincodeLogger) Debug(args ...interface{})
    (c *ChaincodeLogger) Info(args ...interface{})
    (c *ChaincodeLogger) Notice(args ...interface{})
    (c *ChaincodeLogger) Warning(args ...interface{})
    (c *ChaincodeLogger) Error(args ...interface{})
    (c *ChaincodeLogger) Critical(args ...interface{})

    (c *ChaincodeLogger) Debugf(format string, args ...interface{})
    (c *ChaincodeLogger) Infof(format string, args ...interface{})
    (c *ChaincodeLogger) Noticef(format string, args ...interface{})
    (c *ChaincodeLogger) Warningf(format string, args ...interface{})
    (c *ChaincodeLogger) Errorf(format string, args ...interface{})
    (c *ChaincodeLogger) Criticalf(format string, args ...interface{})

The ``f`` forms of the logging APIs provide for precise control over the
formatting of the logs. The non-\ ``f`` forms of the APIs currently
insert a space between the printed representations of the arguments, and
arbitrarily choose the formats to use.

In the current implementation, the logs produced by the ``shim`` and a
``ChaincodeLogger`` are timestamped, marked with the logger *name* and
severity level, and written to ``stderr``. Note that logging level
control is currently based on the *name* provided when the
``ChaincodeLogger`` is created. To avoid ambiguities, all
``ChaincodeLogger`` should be given unique names other than "shim". The
logger *name* will appear in all log messages created by the logger. The
``shim`` logs as "shim".

The default logging level for loggers within the Chaincode container can
be set in the
`core.yaml <https://github.com/hyperledger/fabric/blob/master/sampleconfig/core.yaml>`__
file. The key ``chaincode.logging.level`` sets the default level for all
loggers within the Chaincode container. The key ``chaincode.logging.shim``
overrides the default level for the ``shim`` logger.

::

    # Logging section for the chaincode container
    logging:
      # Default level for all loggers within the chaincode container
      level:  info
      # Override default level for the 'shim' logger
      shim:   warning

The default logging level can be overridden by using environment
variables. ``CORE_CHAINCODE_LOGGING_LEVEL`` sets the default logging
level for all loggers. ``CORE_CHAINCODE_LOGGING_SHIM`` overrides the
level for the ``shim`` logger.

Go language chaincodes can also control the logging level of the
chaincode ``shim`` interface through the ``SetLoggingLevel`` API.

``SetLoggingLevel(LoggingLevel level)`` - Control the logging level of
the shim

Below is a simple example of how a chaincode might create a private
logging object logging at the ``LogInfo`` level.

::

    var logger = shim.NewLogger("myChaincode")

    func main() {

        logger.SetLevel(shim.LogInfo)
        ...
    }

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

