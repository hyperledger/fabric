## Confidentiality
&nbsp;

##### How is the confidentiality of transactions and business logic achieved?
The security module works in conjunction with the membership service module to provide access control service to any data recorded and business logic deployed on a chain network.

When a code is deployed on a chain network, whether it is used to define a business contract or an asset, its creator can put access control on it so that only transactions issued by authorized entities will be processed and validated by chain validators.

Raw transaction records are permanently stored in the ledger. While the contents of non-confidential transactions are open to all participants, the contents of confidential transactions are encrypted with secret keys known only to their originators, validators, and authorized auditors. Only holders of the secret keys can interpret transaction contents.

&nbsp;
##### What if none of the stakeholders of a business contract are validators?
In some business scenarios, full confidentiality of contract logic may be required – such that only contract counterparties and auditors can access and interpret their chaincode. Under these scenarios, counter parties would need to spin off a new child chain with only themselves as validators.
