Tutorials
=========

Application developers can use the Fabric tutorials to get started building their
own solutions. Start working with Fabric by deploying the `test network <./test_network.html>`_
on your local machine. You can then use the steps provided by the :doc:`deploy_chaincode`
tutorial to deploy and test your smart contracts. The :doc:`write_first_app`
tutorial provides an introduction to how to use the APIs provided by the Fabric
SDKs to invoke smart contracts from your client applications. For an in depth
overview of how Fabric applications and smart contracts work together, you
can visit the :doc:`developapps/developing_applications` topic.

The :doc:`deploy_chaincode` tutorial can also be used by network operators to learn
how to use the Fabric chaincode lifecycle to manage smart contracts deployed on
a running network. Both network operators and application developers can use the
tutorials on `Private data <./private_data_tutorial.html>`_ and `CouchDB <./couchdb_tutorial.html>`_
to explore important Fabric features. When you are ready to deploy Hyperledger
Fabric in production, see the guide for :doc:`deployment_guide_overview`.

There are two tutorials for updating a channel: :doc:`config_update` and
:doc:`updating_capabilities`, while :doc:`upgrading_your_components` shows how
to upgrade components like peers, ordering nodes, SDKs, and more.

Finally, we provide an introduction to how to write a basic smart contract,
:doc:`chaincode4ade`.

.. note:: If you have questions not addressed by this documentation, or run into
          issues with any of the tutorials, please visit the :doc:`questions`
          page for some tips on where to find additional help.

.. toctree::
   :maxdepth: 1
   :caption: Tutorials

   test_network
   deploy_chaincode.md
   write_first_app
   tutorial/commercial_paper
   private_data_tutorial
   couchdb_tutorial
   channel_update_tutorial
   config_update.md
   chaincode4ade
   build_network
   videos

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
