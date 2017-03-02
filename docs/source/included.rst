What's Offered?
================

The getting started example uses Docker images to generate the Fabric
network components.  The scenario includes a consortium of three 
members, each managing and maintaing a peer node, as well as a "SOLO"
:ref:`Ordering-Service` and a Certificate Authority (CA). The cryptographic identity
material, based on standard PKI implementation, has been pre-generated
and is used for signing + verification on both the server (peer + ordering service) 
and client (SDK) sides.  The CA is the network entity responsible for issuing
and maintaing this identity material, which is necessary for authentication by all 
components and participants on the network.  This sample uses a single CA.  However,
in enterprise scenarios each :ref:`Member` would likely have their own CA, with more
complex security/identity measures implemented - e.g. cross-signing certificates, etc.

The members will transact on a private channel, with a shared ledger maintained by 
each peer node.  Requests to read and write data to/from the ledger are sent
as "proposals" to the peers.  These proposals are in fact a request for endorsement
from the peer, which will execute the transaction and return a response to the 
submitting client.  

The sample demonstrates two methods for interacting with the network - a programmatical 
approach exercising the Node.js SDK APIs and a CLI requiring manual commands.

It's recommended to follow the sample in the order laid forth - application first, 
followed by the optional CLI route.  



