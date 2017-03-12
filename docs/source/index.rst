Welcome to Fabric
=================

Hyperledger Fabric is a social innovation that is about to free innovators in startups, 
enterprises and government to transform and radically reduce the cost of working together 
across organizations. By the end of this section, you should have the essential understanding 
of Fabric you need to start *knitting* together a great business network.

Fabric is a network of networks, like the Internet itself. An application can use one or more 
networks, each managing different :ref:`Assets`, Agreements and Transactions between different 
sets of :ref:`Member` nodes.  In Fabric, the Ordering Service is the foundation of each network. 
The founder of a network selects an Ordering Service (or creates a new one) and passes in a 
config file with the rules (usually called Policies) that govern it. Examples of these rules 
include setting/defining which Members can join the network, how Members can be added or removed, 
and configuration details like block size. While it is possible for one company to set and control 
these rules as a "dictator," typically these rules will also include policies that make changing 
the rules a matter of consensus among the members of the network.  Fabric also requires some level of 
"endorsement" in order to transact. Check out the power and intricacy of :doc:`endorsement-policies` 
, which are used across the Fabric landscape - from a consortium's network configuration to a simple 
read operation.

We mentioned that the Ordering Service (OS) is the foundation of the network, and you're probably 
thinking, "It must do something beyond just ordering."  Well you're right!  All members and entities 
in the network will be tied to a higher level certificate authority, and this authority is defined 
within the configuration of the Ordering Service.   As a result, the OS can verify and authenticate 
transactions arriving from any corner of the network.  The OS plays a central and critical role in 
the functionality and integrity of the network, and skeptics might fear too much centralization of 
power and responsibility.  After all, that's a principal feature of shared ledger technology - to 
decentralize the control and provide a foundation of trust with entities who you CAN'T wholeheartedly 
trust.  Well let's assuage that fear.  The OS is agnostic to transaction details; it simply orders on 
a first-come-first-serve basis and returns blocks to their corresponding channels. Perhaps more 
importantly though, control of the ordering service can be shared and co-administered by the 
participating members in the network.  OR, if even that solution is untenable, then the OS can be 
hosted and maintained by a trusted third-party.  Fabric is built upon a modular and pluggable 
architecture, so the only real decision for business networks is how to configure an OS to meet 
their requirements. 

(This notion of the OS as a pluggable component also opens the door to exciting opportunities for 
innovative teams and individuals.  Currently there are only a few OS orchestrations - Solo and Kafka. 
However, other options such as Intel's PoET or certain BFT flavors could be powerful supplementaries to Fabric, 
and help solve challenging use cases.)

To participate in the Network, each Organization maintains a runtime called a :ref:`Peer`, which will 
allow an application to participate in transactions, interact with the Ordering Service, and maintain 
a set of ledgers. Notice we said a set of ledgers. One of Fabric's key innovations is the ability to 
run multiple :ref:`Channel` s on each network. This is how a network can conduct both highly confidential 
bilateral transactions and multilateral, or even public, transactions in the same solution without 
everyone having a copy of every transaction or run the code in every agreement. 

Watch how Fabric is `Building a Blockchain for Business <https://www.youtube.com/watch?v=EKa5Gh9whgU>`__ .

If you're still reading, you clearly have some knowledge and an interest in distributed ledger 
technology, AND you probably think a key piece is missing.  Where is consensus in all of this?  Well, 
it's embedded in the entire life cycle of a transaction.  Transactions come into the network, and the 
submitting client's identity is verified and consented upon.  Transactions then get executed and endorsed, 
and these endorsements are consented upon.  Transactions get ordered, and the validity of this order is 
consented upon.  Finally, transactions get committed to a shared ledger, and each transaction's subsequent 
impact on the state of the involved asset(s) is consented upon.  Consensus isn't pigeonholed into one 
module or one function.  It lives and exists throughout the entire DNA of Fabric.  Fabric is built 
with security at the forefront, not as an afterthought.  Members and participating entities operate with 
known identities, and no action on the network circumvents the sign/verify/authenticate mandate.  Requirements 
such as security, privacy and confidentiality are paramount in some manner to nearly all business dealings, 
and they, like consensus, are stitched into the very essence of Fabric. 

So what problem do you want to solve?  What assets are at stake?  Who are the players? What levels of 
security and encryption do you need?  Fabric is designed to provide an answer and solution to this 
challenging collective of questions and beyond.  Just like fabric - in the literal sense of the word - is 
used in everything from airplane seats to bespoke suits, solutions built on Hyperledger Fabric can range 
from diamond provenance to equities trading.  Explore the documentation and see how you can leverage Fabric 
to craft a PoC for your own business network.

.. NOTE:: This build of the docs is from the "|version|" branch

.. toctree::
   :maxdepth: 2
   :caption: Getting Started
   
   included
   asset_setup

.. toctree::
   :maxdepth: 2
   :caption: Key Concepts 

   overview
   fabric_model
   biz/usecases
   

.. toctree::
   :maxdepth: 2
   :caption: Education

   demos
   dockercompose
   learn_chaincode
   videos

.. toctree::
   :maxdepth: 2
   :caption: Configuration Considerations

   best_practices
   bootstrap
   adminops
   Setup/logging-control

.. toctree::
   :maxdepth: 2
   :caption: Architecture

   txflow
   arch-deep-dive
   Setup/ca-setup
   nodesdk
   chaincode
   endorsement-policies
   orderingservice
   ledger
   readwrite
   gossip
   
.. toctree::
   :maxdepth: 2
   :caption: Troubleshooting and FAQs

   troubleshooting
   FAQ/architecture_FAQ
   FAQ/chaincode_FAQ
   FAQ/confidentiality_FAQ
   FAQ/identity_management_FAQ

.. toctree::
   :maxdepth: 2
   :caption: Appendix

   glossary
   releases
   CONTRIBUTING
   MAINTAINERS
   jira_navigation
   dev-setup/devenv
   dev-setup/build
   Gerrit/lf-account
   Gerrit/gerrit
   Gerrit/changes
   Gerrit/reviewing
   Gerrit/best-practices
   testing
   Style-guides/go-style
   questions
   quality
   status
   license
