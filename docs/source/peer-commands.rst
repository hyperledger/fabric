Peer Commands
=============

The ``peer`` command allows administrators to interact with a peer. Use this
command when you want to perform peer operations such as deploying a smart
contract chaincode or joining a channel.

Syntax
^^^^^^

The ``peer`` command has different subcommands within it:

.. code:: bash

  peer [subcommand]

as follows

.. code:: bash

  peer chaincode
  peer channel
  peer logging
  peer node
  peer version
  peer

These subcommands separate the different functions provided by a peer into their
own category. For example, use the ``peer chaincode`` command to perform smart
contract chaincode operations on the peer, or the ``peer channel`` command to
perform channel related operations.

Within each subcommand there are many different options available and because of
this, each is described in its own section in this topic.

If a command option is not specified then ``peer`` will return some high level
help text as described in in the ``--help`` flag below.

``peer`` flags
^^^^^^^^^^^^^^

The ``peer`` command also has a set of associated flags:

.. code:: bash

  peer [flags]

as follows

.. code:: bash

  peer --help
  peer channel --help
  peer channel list --help

These flags provide more information about a peer, and are designated *global*
because they can be used at any command level. For example the ``--help`` flag can
provide help on the ``peer`` command, the ``peer channel`` command, as well as their
respective options.

Flag details
^^^^^^^^^^^^

* ``--help``

  Use `help` to get brief help text for the ``peer`` command. The ``help`` flag can
  often be used at different levels to get individual command help, or even a
  help on a command option. See individual commands for more detail.

* ``--logging-level <string>``

  This flag sets the log level for the single peer command it is supplied with.
  Once the command has been executed, the log level will not persist. There are
  six possible log levels: ``debug``, ``info``, ``warning``, ``error``, ``panic``,
  and ``fatal``. Note that there is no single logging level for the peer. You
  can find the current logging level for a specific component on the peer by
  running ``peer logging getlevel <component-name>``. The defaults are defined in
  ``sampleconfig/core.yaml`` if you'd like to take a look at what logging levels are
  set if the system admin doesn't modify anything.

  This command is overridden by the ``CORE_LOGGING_LEVEL`` environment variable
  if it is set.

* ``--version``

  Use this flag to determine the build version for the peer. This flag provides
  a set of detailed information on how the peer was built.

Usage
^^^^^

Here's some examples using the different available flags on the `peer` command.

* ``--help`` flag

.. code:: bash

  peer --help

  Usage:
    peer [flags]
    peer [command]

  Available Commands:
    chaincode   Operate a chaincode: install|instantiate|invoke|package|query|signpackage|upgrade.
    channel     Operate a channel: create|fetch|join|list|update.
    logging     Log levels: getlevel|setlevel|revertlevels.
    node        Operate a peer node: start|status.
    version     Print fabric peer version.

  Flags:
        --logging-level string       Default logging level and overrides, see core.yaml for full syntax
    -v, --version                    Display the build version for this fabric peer

  Use "peer [command] --help" for more information about a command.


* ``--version`` flag

.. code:: bash

  peer --version

  peer:
   Version: 1.0.4
   Go version: go1.7.5
   OS/Arch: linux/amd64
   Chaincode:
    Base Image Version: 0.3.2
    Base Docker Namespace: hyperledger
    Base Docker Label: org.hyperledger.fabric
    Docker Namespace: hyperledger

The ``peer channel`` Command
----------------------------

The ``peer channel`` command allows administrators to perform channel related
operations on a peer, such as joining a channel or instantiating smart contract
chaincode.

Syntax
^^^^^^

The ``peer channel`` command has the following syntax:

.. code:: bash

  peer channel [command]

as follows

.. code:: bash

  peer channel create
  peer channel fetch
  peer channel join
  peer channel list
  peer channel update

These commands relate to the different channel operations that are relevant to a
peer. For example, use the ``peer channel join`` command to join a peer to a
channel, or the ``peer channel list`` command to show the channels to which a peer
is joined.

``peer channel`` flags
^^^^^^^^^^^^^^^^^^^^^^

Each ``peer channel`` command has different flags available to it, and because of
this, each flag is described in the relevant command topic.

The ``peer channel`` command also has a set of flags that relate to every
`peer channel` command.

.. code:: bash

  peer channel [flags]

as follows

.. code:: bash

  peer channel --cafile <string>
  peer channel --orderer <string>
  peer channel --tls

The global ``peer`` command flags also apply as described in the `peer command`
flags:

* ``--help``
* ``--logging-level <string>``
* ``--version``

Flag details
^^^^^^^^^^^^

* ``--cafile <string>``

  a fully qualified path to a file containing PEM-encoded certificates for the
  orderer being communicated with.

* ``--orderer <string>``

  the fully qualified IP address and port of the orderer being communicated with
  for this channel operation.  If the port is not specified, it will default to
  port 7050. An IP address must be specified if the ``--orderer`` flag is used.

* ``--tls``

  Use this flag to enable TLS communications for the `peer channel` command. The
  certificates specified with ``--cafile`` will be used for TLS communications to
  authenticate the orderer identified by ``--orderer``.

Usage
^^^^^

Here's some examples using the different available flags on the ``peer channel``
command.

* Using the ``--orderer`` flag to list the channels to which a peer is joined.

.. code:: bash

  peer channel list --orderer orderer.example.com:7050

  2017-11-30 12:07:51.317 UTC [msp] GetLocalMSP -> DEBU 001 Returning existing local MSP
  2017-11-30 12:07:51.317 UTC [msp] GetDefaultSigningIdentity -> DEBU 002 Obtaining default signing identity
  2017-11-30 12:07:51.321 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
  2017-11-30 12:07:51.323 UTC [msp/identity] Sign -> DEBU 004 Sign: plaintext: 0A8A070A5C08031A0C0897E9FFD00510...631A0D0A0B4765744368616E6E656C73
  2017-11-30 12:07:51.323 UTC [msp/identity] Sign -> DEBU 005 Sign: digest: D170CD2D6FEB04E49033B54B0AC53744991ADAA320C5733074BC5227BD19E863
  2017-11-30 12:07:51.335 UTC [channelCmd] list -> INFO 006 Channels peers has joined to:
  2017-11-30 12:07:51.335 UTC [channelCmd] list -> INFO 007 drivenet.channel.001
  2017-11-30 12:07:51.335 UTC [main] main -> INFO 008 Exiting.....

You can see that the peer joined to a channel called ``drivenet.channel.001``.

The ``peer channel fetch`` command
----------------------------------

The ``peer channel fetch`` command allows administrators to fetch channel
transaction blocks from the network orderer. The retrieved blocks will typically
contain user transactions but they can also contain configuration transactions
such as the initial genesis block for the channel or any subsequent channel
configuration update.

The peer must have joined the channel, and have read access to it, in order for
the command to complete successfully.

Syntax
^^^^^^

The ``peer channel fetch`` command has the following syntax:

.. code:: bash

  peer channel fetch <newest|oldest|config|(block number)> [flags]

where:

* ``newest``

  returns the most recent channel block available to the network orderer. This
  may be a user transaction block or a configuration transaction.

  This option will also return the block number of the most recent transaction.

* ``oldest``

  returns the oldest channel block available to the network orderer. This may be
  a user transaction block or a configuration transaction.

  This option will also return the block number of the oldest available
  transaction.

* ``config``

  returns the most recent channel configuration block available to the network
  orderer. This can only be a configuration transaction.

  This option will also return the block number of the most recent configuration
  transaction.

* ``(block number)``

  returns the specified channel block. This may be a user transaction block or a
  configuration transaction.

  Specifying 0 will result in the genesis block for this channel being returned
  (if it is still available to the network orderer).

``peer channel fetch`` flags
----------------------------

The ``peer channel fetch`` command has the following command specific flags:

Flag details
^^^^^^^^^^^^

* ``--channelID <string>``

  the name of the channel for which the blocks are to be fetched from the
  network orderer.

  The global ``peer`` command flags also apply as described in the
  ``peer command`` section.

*  ``--cafile``
* ``--orderer``
* ``--tls``

Usage
^^^^^

Output from the ``peer channel fetch`` command is written to a file named
according to the fetch options. It will be one of the following:

* ``<channelID>_newest.block``
* ``<channelID>_oldest.block``
* ``<channelID>_config.block``
* ``<channelID>_(block number).block``

Here's some examples using the different available flags on the ``peer channel fetch`` command.

* Using the ``newest`` option to retrieve the most recent channel block.

.. code:: bash

  peer channel fetch newest  -c drivenet.channel.001 --orderer orderer.example.com:7050

    2017-11-30 17:02:56.234 UTC [msp] GetLocalMSP -> DEBU 001 Returning existing local MSP
    2017-11-30 17:02:56.234 UTC [msp] GetDefaultSigningIdentity -> DEBU 002 Obtaining default signing identity
    2017-11-30 17:02:56.237 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
    2017-11-30 17:02:56.237 UTC [msp] GetLocalMSP -> DEBU 004 Returning existing local MSP
    2017-11-30 17:02:56.237 UTC [msp] GetDefaultSigningIdentity -> DEBU 005 Obtaining default signing identity
    2017-11-30 17:02:56.240 UTC [msp] GetLocalMSP -> DEBU 006 Returning existing local MSP
    2017-11-30 17:02:56.240 UTC [msp] GetDefaultSigningIdentity -> DEBU 007 Obtaining default signing identity
    2017-11-30 17:02:56.240 UTC [msp/identity] Sign -> DEBU 008 Sign: plaintext: 0AC9060A1B08021A0608C0F380D10522...DC7F80E9BEE612080A020A0012020A00
    2017-11-30 17:02:56.241 UTC [msp/identity] Sign -> DEBU 009 Sign: digest: D3F6C959BCFCD78B5895A466276C181EEA3B54C1CF8E8707238FE3A3D358F769
    2017-11-30 17:02:56.245 UTC [channelCmd] readBlock -> DEBU 00a Received block: 32
    2017-11-30 17:02:56.246 UTC [main] main -> INFO 00b Exiting.....

  ls -alt

    total 276
    drwxr-xr-x 2 root root   4096 Nov 30 16:17 .
    -rw-r--r-- 1 root root  13307 Nov 30 17:02 drivenet.channel.001_newest.block
    drwxr-xr-x 3 root root   4096 Nov 21 13:38 ..

You can see that the retrieved block is number 32.

* Using the ``(block number)``` option to retrieve a specific block -- in this
  case, block number 16.

.. code:: bash

    peer channel fetch 16  -c drivenet.channel.001 --orderer orderer.example.com:7050

    2017-11-30 17:08:12.039 UTC [msp] GetLocalMSP -> DEBU 001 Returning existing local MSP
    2017-11-30 17:08:12.039 UTC [msp] GetDefaultSigningIdentity -> DEBU 002 Obtaining default signing identity
    2017-11-30 17:08:12.042 UTC [channelCmd] InitCmdFactory -> INFO 003 Endorser and orderer connections initialized
    2017-11-30 17:08:12.042 UTC [msp] GetLocalMSP -> DEBU 004 Returning existing local MSP
    2017-11-30 17:08:12.042 UTC [msp] GetDefaultSigningIdentity -> DEBU 005 Obtaining default signing identity
    2017-11-30 17:08:12.042 UTC [msp] GetLocalMSP -> DEBU 006 Returning existing local MSP
    2017-11-30 17:08:12.042 UTC [msp] GetDefaultSigningIdentity -> DEBU 007 Obtaining default signing identity
    2017-11-30 17:08:12.042 UTC [msp/identity] Sign -> DEBU 008 Sign: plaintext: 0AC9060A1B08021A0608FCF580D10522...B092120C0A041A02081012041A020810
    2017-11-30 17:08:12.042 UTC [msp/identity] Sign -> DEBU 009 Sign: digest: CD6F4ADB7E00E79E4FADBE627CBE7CAA6F2A4471A9A0BE780CD4BE65AF8B96DE
    2017-11-30 17:08:12.046 UTC [channelCmd] readBlock -> DEBU 00a Received block: 16
    2017-11-30 17:08:12.046 UTC [main] main -> INFO 00b Exiting.....

    ls -alt

    total 276
    drwxr-xr-x 2 root root   4096 Nov 30 16:17 .
    -rw-r--r-- 1 root root  10474 Nov 30 17:08 drivenet.channel.001_16.block
    -rw-r--r-- 1 root root  13307 Nov 30 17:02 drivenet.channel.001_newest.block
    drwxr-xr-x 3 root root   4096 Nov 21 13:38 ..

You can see that the retrieved block is number 16.

For configuration blocks, the file can be formatted using the ``configtxlator``
command.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
