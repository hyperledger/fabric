Logging Control
===============

Overview
--------

Logging in the ``peer`` application and in the ``shim`` interface to
chaincodes is programmed using facilities provided by the
``github.com/op/go-logging`` package. This package supports

-  Logging control based on the severity of the message
-  Logging control based on the software *module* generating the message
-  Different pretty-printing options based on the severity of the
   message

All logs are currently directed to ``stderr``, and the pretty-printing
is currently fixed. However global and module-level control of logging
by severity is provided for both users and developers. There are
currently no formalized rules for the types of information provided at
each severity level, however when submitting bug reports the developers
may want to see full logs down to the DEBUG level.

In pretty-printed logs the logging level is indicated both by color and
by a 4-character code, e.g, "ERRO" for ERROR, "DEBU" for DEBUG, etc. In
the logging context a *module* is an arbitrary name (string) given by
developers to groups of related messages. In the pretty-printed example
below, the logging modules "peer", "rest" and "main" are generating
logs.

::

    16:47:09.634 [peer] GetLocalAddress -> INFO 033 Auto detected peer address: 9.3.158.178:7051
    16:47:09.635 [rest] StartOpenchainRESTServer -> INFO 035 Initializing the REST service...
    16:47:09.635 [main] serve -> INFO 036 Starting peer with id=name:"vp1" , network id=dev, address=9.3.158.178:7051, discovery.rootnode=, validator=true

An arbitrary number of logging modules can be created at runtime,
therefore there is no "master list" of modules, and logging control
constructs can not check whether logging modules actually do or will
exist. Also note that the logging module system does not understand
hierarchy or wildcarding: You may see module names like "foo/bar" in the
code, but the logging system only sees a flat string. It doesn't
understand that "foo/bar" is related to "foo" in any way, or that
"foo/\*" might indicate all "submodules" of foo.

peer
----

The logging level of the ``peer`` command can be controlled from the
command line for each invocation using the ``--logging-level`` flag, for
example

::

    peer node start --logging-level=debug

The default logging level for each individual ``peer`` subcommand can
also be set in the
`core.yaml <https://github.com/hyperledger/fabric/blob/master/sampleconfig/core.yaml>`__
file. For example the key ``logging.node`` sets the default level for
the ``node`` subcommand. Comments in the file also explain how the
logging level can be overridden in various ways by using environment
variables.

Logging severity levels are specified using case-insensitive strings
chosen from

::

    CRITICAL | ERROR | WARNING | NOTICE | INFO | DEBUG

The full logging level specification for the ``peer`` is of the form

::

    [<module>[,<module>...]=]<level>[:[<module>[,<module>...]=]<level>...]

A logging level by itself is taken as the overall default. Otherwise,
overrides for individual or groups of modules can be specified using the

::

    <module>[,<module>...]=<level>

syntax. Examples of specifications (valid for all of
``--logging-level``, environment variable and
`core.yaml <https://github.com/hyperledger/fabric/blob/master/sampleconfig/core.yaml>`__
settings):

::

    info                                       - Set default to INFO
    warning:main,db=debug:chaincode=info       - Default WARNING; Override for main,db,chaincode
    chaincode=info:main=debug:db=debug:warning - Same as above

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
overrides the default level for the ``shim`` module.

::

    # Logging section for the chaincode container
    logging:
      # Default level for all loggers within the chaincode container
      level:  info
      # Override default level for the 'shim' module
      shim:   warning

The default logging level can be overridden by using environment
variables. ``CORE_CHAINCODE_LOGGING_LEVEL`` sets the default logging
level for all modules. ``CORE_CHAINCODE_LOGGING_SHIM`` overrides the
level for the ``shim`` module.

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

