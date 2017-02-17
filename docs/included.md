# What's Included?

This section demonstrates an example using the Hyperledger Fabric V1.0 architecture.
The scenario will include the creation and joining of channels, client side authentication,
and the deployment and invocation of chaincode.  CLI will be used for the creation and
joining of the channel and the node SDK will be used for the client authentication,
and chaincode functions utilizing the channel.

Docker Compose will be used to create a consortium of three organizations, each
running an endorsing/committing peer, as well as a "solo" orderer and a Certificate Authority (CA).
The cryptographic material, based on standard PKI implementation, has been pre-generated
and is included in the `sfhackfest.tar.gz` in order to expedite the flow.  The CA, responsible for
issuing, revoking and maintaining the crypto material, represents one of the organizations and
is needed by the client (node SDK) for authentication.  In an enterprise scenario, each
organization might have their own CA, with more complex security measures implemented - e.g.
cross-signing certificates, etc.

The network will be generated automatically upon execution of `docker-compose up`,
and the APIs for create channel and join channel will be explained and demonstrated;
as such, a user can go through the steps to manually generate their own network
and channel, or quickly jump to the application development phase.

It is recommended to run through this section in the order it is laid out - node
program first, followed by the CLI approach.
