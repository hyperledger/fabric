Use node SDK to register/enroll user, followed by deploy/invoke
---------------------------------------------------------------

The individual javascript programs will exercise the SDK APIs to
register and enroll the client with the provisioned Certificate
Authority. Once the client is properly authenticated, the programs will
demonstrate basic chaincode functionalities - deploy, invoke, and query.
Make sure you are in the working directory where you pulled the source
code before proceeding.

Upon success of each node program, you will receive a "200" response in
the terminal.

Register/enroll & deploy chaincode (Linux or OSX):

.. code:: bash

    # Deploy initializes key value pairs of "a","100" & "b","200".
    GOPATH=$PWD node deploy.js

Register/enroll & deploy chaincode (Windows):

.. code:: bash

    # Deploy initializes key value pairs of "a","100" & "b","200".
    SET GOPATH=%cd%
    node deploy.js

Issue an invoke. Move units 100 from "a" to "b":

.. code:: bash

    node invoke.js

Query against key value "b":

.. code:: bash

    # this should return a value of 300
    node query.js

Explore the various node.js programs, along with ``example_cc.go`` to
better understand the SDK and APIs.
