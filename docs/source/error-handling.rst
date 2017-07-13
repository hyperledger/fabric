Error handling
==============

General Overview
----------------
The Hyperledger Fabric error handling framework can be found in the source
repository under **common/errors**. It defines a new type of error,
CallStackError, to use in place of the standard error type provided by Go.

A CallStackError consists of the following:

- Component code - a name for the general area of the code that is generating
  the error. Component codes should consist of three uppercase letters. Numerics
  and special characters are not allowed. A set of component codes is defined
  in common/errors/codes.go
- Reason code - a short code to help identify the reason the error occurred.
  Reason codes should consist of three numeric values. Letters and special
  characters are not allowed. A set of reason codes is defined in
  common/error/codes.go
- Error code - the component code and reason code separated by a colon,
  e.g. MSP:404
- Error message - the text that describes the error. This is the same as the
  input provided to ``fmt.Errorf()`` and ``Errors.New()``. If an error has been
  wrapped into the current error, its message will be appended.
- Callstack - the callstack at the time the error is created. If an error has
  been wrapped into the current error, its error message and callstack will be
  appended to retain the context of the wrapped error.

The CallStackError interface exposes the following functions:

- Error() - returns the error message with callstack appended
- Message() - returns the error message (without callstack appended)
- GetComponentCode() - returns the 3-character component code
- GetReasonCode() - returns the 3-digit reason code
- GetErrorCode() - returns the error code, which is "component:reason"
- GetStack() - returns just the callstack
- WrapError(error) - wraps the provided error into the CallStackError

Usage Instructions
------------------

The new error handling framework should be used in place of all calls to
``fmt.Errorf()`` or ``Errors.new()``. Using this framework will provide error
codes to check against as well as the option to generate a callstack that will be
appended to the error message.

Using the framework is simple and will only require an easy tweak to your code.

First, you'll need to import **github.com/hyperledger/fabric/common/errors** into
any file that uses this framework.

Let's take the following as an example from core/chaincode/chaincode_support.go:

.. code:: go

  err = fmt.Errorf("Error starting container: %s", err)

For this error, we will simply call the constructor for Error and pass a
component code, reason code, followed by the error message. At the end, we
then call the ``WrapError()`` function, passing along the error itself.

.. code:: go

  fmt.Errorf("Error starting container: %s", err)

becomes

.. code:: go

  errors.ErrorWithCallstack("CHA", "505", "Error starting container").WrapError(err)

You could also just leave the message as is without any problems:

.. code:: go

  errors.ErrorWithCallstack("CHA", "505", "Error starting container: %s", err)

With this usage you will be able to format the error message from the previous
error into the new error, but will lose the ability to print the callstack (if
the wrapped error is a CallStackError).

A second example to highlight a scenario that involves formatting directives for
parameters other than errors, while still wrapping an error, is as follows:

.. code:: go

  fmt.Errorf("failed to get deployment payload %s - %s", canName, err)

becomes

.. code:: go

  errors.ErrorWithCallstack("CHA", "506", "Failed to get deployment payload %s", canName).WrapError(err)

Displaying error messages
-------------------------

Once the error has been created using the framework, displaying the error
message is as simple as:

.. code:: go

  logger.Errorf(err)

or

.. code:: go

  fmt.Println(err)

or

.. code:: go

  fmt.Printf("%s\n", err)

An example from peer/common/common.go:

.. code:: go

  errors.ErrorWithCallstack("PER", "404", "Error trying to connect to local peer").WrapError(err)

would display the error message:

.. code:: bash

  PER:404 - Error trying to connect to local peer
  Caused by: grpc: timed out when dialing

.. note:: The callstacks have not been displayed for this example for the sake of
          brevity.

General guidelines for error handling in Hyperledger Fabric
-----------------------------------------------------------

- If it is some sort of best effort thing you are doing, you should log the
  error and ignore it.
- If you are servicing a user request, you should log the error and return it.
- If the error comes from elsewhere, you have the choice to wrap the error
  or not. Typically, it's best to not wrap the error and simply return
  it as is. However, for certain cases where a utility function is called,
  wrapping the error with a new component and reason code can help an end user
  understand where the error is really occurring without inspecting the callstack.
- A panic should be handled within the same layer by throwing an internal error
  code/start a recovery process and should not be allowed to propagate to other
  packages.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

