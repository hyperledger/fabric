
## Roles & Personas

#### _Roles_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Chain Member</b></td>
<td>
Entities that do not participate in the validation process of a blockchain network, but help to maintain the integrity of a network. Unlike Chain transactors, chain members maintain a local copy of the ledger.
</td>
</tr>
<tr>
<td width="20%"><b>Chain Transactor</b></td>
<td>
Entities that have permission to create transactions and query network data.
</td>
</tr>
<tr>
<td width="20%"><b>Chain Validator</b></td>
<td>
Entities that own a stake of a chain network. Each chain validator has a voice in deciding whether a transaction is valid, therefore chain validators can interrogate all transactions sent to their chain.
</td>
</tr>
<tr>
<td width="20%"><b>Chain Auditor</b></td>
<td>
Entities with the permission to interrogate transactions.
</td>
</tr>
</table>

#### _Participants_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Solution User</b></td>
<td>
End users are agnostic about the details of chain networks, they typically initiate transactions on a chain network through applications made available by solutions providers.
<p><p>
<span style="text-decoration:underline">Roles:</span> None
</td>
</tr>
<tr>
<td width="20%"><b>Solution Provider</b></td>
<td>
Organizations that develop mobile and/or browser based applications for end (solution) users to access chain networks. Some application owners may also be network owners.
<p><p>
Roles: Chain Transactor
</td>
</tr>
<tr>
<td width="20%"><b>Network Proprietor</b></td>
<td>
Proprietor(s) setup and define the purpose of a chain network. They are the stakeholders of a network.
<p><p>
Roles: Chain Transactor, Chain Validator
</td>
</tr>
<tr>
<td width="20%"><b>Network Owner</b></td>
<td>
Owners are stakeholders of a network that can validate transactions. After a network is first launched, its proprietor (who then becomes an owner) will invite business partners to co-own the network (by assigning them validating nodes). Any new owner added to a network must be approved by its existing owners.
<p><p>
Roles: Chain Transactor, Chain Validator
</td>
</tr>
<tr>
<td width="20%"><b>Network Member</b></td>
<td>
Members are participants of a blockchain network that cannot validate transactions but has the right to add users to the network.
<p><p>
Roles: Chain Transactor, Chain Member
</td>
</tr>
<tr>
<td width="20%"><b>Network Users</b></td>
<td>
End users of a network are also solution users. Unlike network owners and members, users do not own nodes. They transact with the network through an entry point offered by a member or an owner node.
<p><p>
Roles: Chain Transactor
</td>
</tr>
<tr>
<td width="20%"><b>Network Auditors</b></td>
<td>
Individuals or organizations with the permission to interrogate transactions.
<p><p>
Roles: Chain Auditor
</td>
</tr>
</table>

&nbsp;

## Business Network

#### _Types of Networks (Business View)_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Industry Network</b></td>
<td>
A chain network that services solutions built for a particular industry.
</td>
</tr>
<tr>
<td width="20%"><b>Regional Industry Network</b></td>
<td>
A chain network that services applications built for a particular industry and region.
</td>
</tr>
<tr>
<td width="20%"><b>Application Network</b></td>
<td>
A chain network that only services a single solution.
</td>
</tr>
</table>

#### _Types of Chains (Conceptual View)_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Main Chain</b></td>
<td>
A business network; each main chain operates one or multiple applications/solutions validated by the same group of organizations.
</td>
</tr>
<tr>
<td width="20%"><b>Confidential Chain</b></td>
<td>
A special purpose chain created to run confidential business logic that is only accessible by contract stakeholders.
</td>
</tr>
</table>


&nbsp;

## Network Management

#### _Member management_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Owner Registration</b></td>
<td>
The process of registering and inviting new owner(s) to a blockchain network. Approval from existing network owners is required when adding or deleting a participant with ownership right
</td>
</tr>
<tr>
<td width="20%"><b>Member Registration</b></td>
<td>
The process of registering and inviting new network members to a blockchain network.
</td>
</tr>
<tr>
<td width="20%"><b>User Registration</b></td>
<td>
The process of registering new users to a blockchain network. Both members and owners can register users on their own behalf as long as they follow the policy of their network.
</td>
</tr>
</table>


&nbsp;

## Transactions

#### _Types of Transactions_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Deployment Transaction</b></td>
<td>
Transactions that deploy a new chaincode to a chain.
</td>
</tr>
<tr>
<td width="20%"><b>Invocation Transaction</b></td>
<td>
Transactions that invoke a function on a chaincode.
</td>
</tr>
</table>


#### _Confidentiality of Transactions_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Public Transaction</b></td>
<td>
A transaction with its payload in the open. Anyone with access to a chain network can interrogate the details of public transactions.
</td>
</tr>
<tr>
<td width="20%"><b>Confidential Transaction</b></td>
<td>
A transaction with its payload cryptographically hidden such that no one besides the stakeholders of a transaction can interrogate its content.
</td>
</tr>
<tr>
<td width="20%"><b>Confidential Chaincode Transaction</b></td>
<td>
A transaction with its payload encrypted such that only validators can decrypt them. Chaincode confidentiality is determined during deploy time. If a chaincode is deployed as a confidential chaincode, then the payload of all subsequent invocation transactions to that chaincode will be encrypted.
</td>
</tr>
</table>


#### _Inter-chain Transactions_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Inter-Network Transaction</b></td>
<td>
Transactions between two business networks (main chains).
</td>
</tr>
<tr>
<td width="20%"><b>Inter-Chain Transaction</b></td>
<td>
Transactions between confidential chains and main chains. Chaincodes in a confidential chain can trigger transactions on one or multiple main chain(s).
</td>
</tr>
</table>

&nbsp;

## Network Entities

#### _Systems_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Application Backend</b></td>
<td>
  Purpose: Backend application service that supports associated mobile and/or browser based applications.
  <p><p>
  Key Roles:<p>
  1)	Manages end users and registers them with the membership service
  <p>
  2)	Initiates transactions requests, and sends the requests to a node
  <p><p>
  Owned by: Solution Provider, Network Proprietor
</td>
</tr>
<tr>
<td width="20%"><b>Non Validating Node (Peer)</b></td>
<td>
  Purpose: Constructs transactions and forwards them to validating nodes. Peer nodes keep a copy of all transaction records so that solution providers can query them locally.
  <p><p>
  Key Roles:<p>
  1)	Manages and maintains user certificates issued by the membership service<p>
  2)	Constructs transactions and forwards them to validating nodes <p>
  3)	Maintains a local copy of the ledger, and allows application owners to query information locally.
  <p><p>
	Owned by: Solution Provider, Network Auditor
</td>
</tr>
<tr>
<td width="20%"><b>Validating Node (Peer)</b></td>
<td>
  Purpose: Creates and validates transactions, and maintains the state of chaincodes<p><p>
  Key Roles:<p>
  1)	Manages and maintains user certificates issued by membership service<p>
  2)	Creates transactions<p>
  3)	Executes and validates transactions with other validating nodes on the network<p>
  4)	Maintains a local copy of ledger<p>
  5)	Participates in consensus and updates ledger
  <p><p>
  Owned by: Network Proprietor, Solution Provider (if they belong to the same entity)
</td>
</tr>
<tr>
<td width="20%"><b>Membership Service</b></td>
<td>
  Purpose: Issues and manages the identity of end users and organizations<p><p>
  Key Roles:<p>
  1)	Issues enrollment certificate to each end user and organization<p>
  2)	Issues transaction certificates associated to each end user and organization<p>
  3)	Issues TLS certificates for secured communication between Hyperledger fabric entities<p>
  4)	Issues chain specific keys
  <p><p>
  Owned by: Third party service provider
</td>
</tr>
</table>


#### _Membership Service Components_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Registration Authority</b></td>
<td>
Assigns registration username & registration password pairs to network participants. This username/password pair will be used to acquire enrollment certificate from ECA.
</td>
</tr>
<tr>
<td width="20%"><b>Enrollment Certificate Authority (ECA)</b></td>
<td>
Issues enrollment certificates (ECert) to network participants that have already registered with a membership service. ECerts are long term certificates used to identify individual entities participating in one or more networks.
</td>
</tr>
<tr>
<td width="20%"><b>Transaction Certificate Authority (TCA)</b></td>
<td>
Issues transaction certificates (TCerts) to ECert owners. An infinite number of TCerts can be derived from each ECert. TCerts are used by network participants to send transactions. Depending on the level of security requirements, network participants may choose to use a new TCert for every transaction.
</td>
</tr>
<tr>
<td width="20%"><b>TLS-Certificate Authority (TLS-CA)</b></td>
<td>
Issues TLS certificates to systems that transmit messages in a chain network. TLS certificates are used to secure the communication channel between systems.
</td>
</tr>
</table>

&nbsp;

## Hyperledger Fabric Entities

#### _Chaincode_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Public Chaincode</b></td>
<td>
Chaincodes deployed by public transactions, these chaincodes can be invoked by any member of the network.
</td>
</tr>
<tr>
<td width="20%"><b>Confidential Chaincode</b></td>
<td>
Chaincodes deployed by confidential transactions, these chaincodes can only be invoked by validating members (Chain validators) of the network.
</td>
</tr>
<tr>
<td width="20%"><b>Access Controlled Chaincode</b></td>
<td>
Chaincodes deployed by confidential transactions that also embed the tokens of approved invokers. These invokers are also allowed to invoke confidential chaincodes even though they are not validators.
</td>
</tr>
</table>


#### _Ledger_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>Chaincode-State</b></td>
<td>
HPL provides state support; Chaincodes access internal state storage through state APIs. States are created and updated by transactions calling chaincode functions with state accessing logic.
</td>
</tr>
<tr>
<td width="20%"><b>Transaction List</b></td>
<td>
All processed transactions are kept in the ledger in their original form (with payload encrypted for confidential transactions), so that network participants can interrogate past transactions to which they have access permissions.
</td>
</tr>
<tr>
<td width="20%"><b>Ledger Hash</b></td>
<td>
A hash that captures the present snapshot of the ledger. It is a product of all validated transactions processed by the network since the genesis transaction.
</td>
</tr>
</table>


#### _Node_
---
<table border="0">
<col>
<col>
<tr>
<td width="20%"><b>DevOps Service</b></td>
<td>
The frontal module on a node that provides APIs for clients to interact with their node and chain network. This module is also responsible to construct transactions, and work with the membership service component to receive and store all types of certificates and encryption keys in its storage.
</td>
</tr>
<tr>
<td width="20%"><b>Node Service</b></td>
<td>
The main module on a node that is responsible to process transactions, deploy and execute chaincodes, maintain ledger data, and trigger the consensus process.
</td>
</tr>
<tr>
<td width="20%"><b>Consensus</b></td>
<td>
The default consensus algorithm of Hyperledger fabric is an implementation of PBFT.
</td>
</tr>
</table>
