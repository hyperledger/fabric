Setting up the Full Hyperledger fabric Developer's Environment
==============================================================

-  See :doc:`Setting Up The Development
   Environment <../dev-setup/devenv>` to set up your development
   environment.

-  The following commands are all issued from the vagrant environment.
   The following will open a terminal session:

::

       cd <your cloned location>/fabric/devenv
       vagrant up
       vagrant ssh

-  Issue the following commands to build the Hyperledger fabric client
   (HFC) Node.js SDK including the API reference documentation

::

       cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
       make all

-  Issue the following command where your Node.js application is located
   if you wish to use the ``require("hfc")``, this will install the HFC
   locally.

::

      npm install /opt/gopath/src/github.com/hyperledger/fabric/sdk/node

Or point to the HFC directly by using the following ``require()`` in
your code:

.. code:: javascript

       require("/opt/gopath/src/github.com/hyperledger/fabric/sdk/node");

-  To build the API reference documentation:

::

       cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
       make doc

-  To build the reference documentation in the
   :doc:`Fabric-starter-kit <../starter/fabric-starter-kit>`

::

       docker exec -it nodesdk /bin/bash
       cd /opt/gopath/src/github.com/hyperledger/fabric/sdk/node
       make doc

-  The the API reference documentation will be available in:
   ``/opt/gopath/src/github.com/hyperledger/fabric/sdk/node/doc``
