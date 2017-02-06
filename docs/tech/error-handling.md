# Hyperledger Fabric - Error handling

## Usage instructions

The new error handling framework should be used in place of all calls to fmt.Errorf() or Errors.new(). Using this framework will provide error codes to check against as well as the option to generate a callstack that will be appended to the error message when logging.error is set to debug in peer/core.yaml.

Using the framework is simple and will only require an easy tweak to your code. First, you'll need to import “github.com/hyperledger/fabric/core/errors” in any code that uses this framework.

Let's take the following as an example: fmt.Errorf(“Error trying to connect to local peer: %s”, err.Error())

For this error, we will simply call the constructor for Error and pass a component name, error message name, followed by the error message and any arguments to format into the message (note the component and message names are case insensitive and will be converted to uppercase):

err = errors.Error(“Peer”, “ConnectionError”, “Error trying to connect to local peer: %s”, err.Error())

For more critical errors for which a callstack would be beneficial, the error can be created as follows:
err = errors.ErrorWithCallstack(“Peer”, “ConnectionError”, “Error trying to connect to local peer: %s”, err.Error())

Note: Make sure to use a consistent component name across code in related files/packages.

Examples of components of the Peer are: Peer, Orderer, Gossip, Ledger, MSP, Chaincode, Comm
Examples of components of the Orderer are: Orderer, SBFT, Kafka, Solo

We may, in the future, add constants to allow searching for currently defined components for those using an editor with code completion capabilities; we are avoiding this for now to avoid merge conflict issues.

## Displaying error messages

Once the error has been created using the framework, displaying the error message is as simple as

logger.Warningf("%s", err) or fmt.Printf("%s\n", err)

which, for the example above, would display the error message:

PEER_CONNECTIONERROR - Error trying to connect to local peer: grpc: timed out when dialing

## Setting stack trace to display for any CallStackError

The display of the stack trace with an error is tied to the logging level for the “error” module, which is initialized using logging.error in core.yaml. It can also be set dynamically for code running on the peer via CLI using “peer logging setlevel error <log-level>”. The default level of “warning” will not display the stack trace; setting it to “debug” will display the stack trace for all errors that use this error handling framework.

## General guidelines for error handling in Fabric

- If it is some sort of best effort thing you are doing, you should log the error and ignore it.
- If you are servicing a user request, you should log the error and return it.
- If the error comes from elsewhere in the Fabric, do not stack the error (errors.Error(“I am stacking errors: %s”, original_error)), simply return the original error.
- A panic should be handled within the same layer by throwing an internal error code/start a recovery process and should not be allowed to propagate to other packages.
