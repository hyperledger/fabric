Designing and Deploying a production network
============================================

This guide is a high level overview of the design and deployment sequence for setting up production Fabric network components, in addition to best practices and a few of the many considerations to keep in mind. There is an important distinction between *designing* the production network and *deploying* the network.

**Designing** is referring to determining the number of components and their configuration, including operational aspects such as how much storage space will needed. 

**Deploying** is referring to the process of setting up and updated the planned architecture in an your choice of compute environment.

Note that this topic will discuss "setting up the network" as a holistic process from the perspective of a single individual. More likely than not, real world production networks will not be set up by a single individual but as a collaborative effort directed by several individuals (a collection of banks each setting up their own components, for example) instead.

Remember the structure of the network must be designed to fit the business use case and any relevant laws and regulations government of the industry in which the network will be designed to function. This deployment guide will not go through every iteration and potential network configuration, but does give common guidelines and rules to consider.

If you are a smart contract or application developer, you should not need this level of expertise in deploying a production level Fabric network. However, you do need to be aware of how networks are deployed in order to develop effective smart contracts and applications. For a development environment to test chaincode, smart contracts, and applications check against, check out :doc:`Getting Started - Run Fabric`. These development environments do not cover issues such as security, resource management, and high availability. These issues become a priority when operating in production. 


.. toctree::
   :maxdepth: 2
   :glob:
   :caption: Designing

   production_deployment/1*

.. toctree::
   :maxdepth: 2
   :glob:
   :caption: Deploying

   production_deployment/2*

