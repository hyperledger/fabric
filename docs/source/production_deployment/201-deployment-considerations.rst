Fabric Network Deployment Considerations
========================================

The structure of a blockchain network will be dictated by the use case it's serving. Use the topics below to help explorer the issues that your use case has. 

* **Chaincode deployment method.**
  Users have the option to deploy their chaincode using either the built in build and run support, a customized build and run using the :doc:`cc_launcher`, or using an :doc:`cc_service`.

* **Using firewalls.**
  In a production deployment, components belonging to one organization might need access to components from other organizations, necessitating the use of firewalls and advanced networking configuration. For example, applications using the Fabric SDK require access to all endorsing peers from all organizations and the ordering services for all channels. Similarly, peers need access to the ordering service on the channels that they are receiving new blocks from.

* **High Availability**
  How many nodes do you need to satisfy high availability, and in what data centers do you wish to deploy them in to satisfy both the needs of disaster recovery and data residency? How will you ensure that your private keys and roots of trust remain secure?

