Command-line Interface (CLI)
============================

To view the currently available CLI commands, execute the following:

::

        cd /opt/gopath/src/github.com/hyperledger/fabric
        build/bin/peer

You will see output similar to the example below (**NOTE:** rootcommand
below is hardcoded in
`main.go <https://github.com/hyperledger/fabric/blob/master/main.go>`__.
Currently, the build will create a *peer* executable file).

::

        Usage:
          peer [flags]
          peer [command]

        Available Commands:
          version     Print fabric peer version.
          node        node specific commands.
          network     network specific commands.
          chaincode   chaincode specific commands.
          help        Help about any command

        Flags:
          -h, --help[=false]: help for peer
              --logging-level="": Default logging level and overrides, see core.yaml for full syntax
              --test.coverprofile="coverage.cov": Done
          -v, --version[=false]: Show current version number of fabric peer server


        Use "peer [command] --help" for more information about a command.

The ``peer`` command supports several subcommands and flags, as shown
above. To facilitate its use in scripted applications, the ``peer``
command always produces a non-zero return code in the event of command
failure. Upon success, many of the subcommands produce a result on
**stdout** as shown in the table below:

Command \| **stdout** result in the event of success --- \| ---
``version`` \| String form of ``peer.version`` defined in
`core.yaml <https://github.com/hyperledger/fabric/blob/master/peer/core.yaml>`__
``node start`` \| N/A ``node status`` \| String form of
`StatusCode <https://github.com/hyperledger/fabric/blob/master/protos/server_admin.proto#L36>`__
``node stop`` \| String form of
`StatusCode <https://github.com/hyperledger/fabric/blob/master/protos/server_admin.proto#L36>`__
``network login`` \| N/A ``network list`` \| The list of network
connections to the peer node. ``chaincode deploy`` \| The chaincode
container name (hash) required for subsequent ``chaincode invoke`` and
``chaincode query`` commands ``chaincode invoke`` \| The transaction ID
(UUID) ``chaincode query`` \| By default, the query result is formatted
as a printable string. Command line options support writing this value
as raw bytes (-r, --raw), or formatted as the hexadecimal representation
of the raw bytes (-x, --hex). If the query response is empty then
nothing is output.

Deploy a Chaincode
------------------

Deploy creates the docker image for the chaincode and subsequently
deploys the package to the validating peer. An example is below.

::

    peer chaincode deploy -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Function":"init", "Args": ["a","100", "b", "200"]}'

Or:

::

    peer chaincode deploy -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -c '{"Args": ["init", "a","100", "b", "200"]}'

The response to the chaincode deploy command will contain the chaincode
identifier (hash) which will be required on subsequent
``chaincode invoke`` and ``chaincode query`` commands in order to
uniquely identify the deployed chaincode.

**Note:** If your GOPATH environment variable contains more than one
element, the chaincode must be found in the first one or deployment will
fail.
