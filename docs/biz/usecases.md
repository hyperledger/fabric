# Canonical Use Cases

&nbsp;

### B2B Contract

Business contracts can be codified to allow two or more parties to automate contractual agreements in a trusted way.  Although information on blockchain is naturally “public”, B2B contracts may require privacy control to protect sensitive business information from being disclosed to outside parties that also have access to the ledger.


<img src="../images/Canonical-Use-Cases_B2BContract.png" width="900" height="456">

While confidential agreements are a key business case, there are many scenarios where contracts can and should be easily discoverable by all parties on a ledger. For example, a ledger used to create offers (asks) seeking bids, by definition, requires access without restriction. This type of contract may need to be standardized so that bidders can easily find them, effectively creating an electronic trading platform with smart contracts (aka chaincode).

#### Persona

*  Contract participant – Contract counter parties

*  Third party participant – A third party stakeholder guaranteeing the integrity of the contract.

#### Key Components

*  Multi-sig contract activation - When a contract is first deployed by one of the counter parties, it will be in the pending activation state. To activate a contract, signatures from other counterparties and/or third party participants are required.

*  Multi-sig contract execution - Some contracts will require one of many signatures to execute. For example, in trade finance, a payment instruction can only be executed if either the recipient or an authorized third party (e.g. UPS) confirms the shipment of the good.

*  Discoverability - If a contract is a business offer seeking bids, it must be easily searchable. In addition, such contracts must have the built-in intelligence to evaluate, select and honor bids.

*  Atomicity of contract execution - Atomicity of the contract is needed to guarantee that asset transfers can only occur when payment is received (Delivery vs. Payment). If any step in the execution process fails, the entire transaction must be rolled back.

*  Contract to chain-code communication - Contracts must be able to communicate with chaincodes that are deployed on the same ledger.

*  Longer Duration contract - Timer is required to support B2B contracts that have long execution windows.

*  Reuseable contracts - Often-used contracts can be standardized for reuse.

*  Auditable contractual agreement - Any contract can be made auditable to third parties.

*  Contract life-cycle management - B2B contracts are unique and cannot always be standardized. An efficient contract management system is needed to enhance the scalability of the ledger network.

*  Validation access – Only nodes with validation rights are allowed to validate transactions of a B2B contract.

*  View access – B2B contracts may include confidential information, so only accounts with predefined access rights are allowed to view and interrogate them.

&nbsp;


### Manufacturing Supply Chain

Final assemblers, such as automobile manufacturers, can create a supply chain network managed by its peers and suppliers so that a final assembler can better manage its suppliers and be more responsive to events that would require vehicle recalls (possibly triggered by faulty parts provided by a supplier). The blockchain fabric must provide a standard protocol to allow every participant on a supply chain network to input and track numbered parts that are produced and used on a specific vehicle.

Why is this specific example an abstract use case? Because while all blockchain cases store immutable information, and some add the need for transfer of assets between parties, this case emphasizes the need to provide deep searchability backwards through as many as 5-10 transaction layers. This backwards search capability is the core of establishing provenance of any manufactured good that is made up of other component goods and supplies.

<img src="../images/Canonical-Use-Cases_Manufacturing-Supply-Chain.png" width="900" height="552">

#### Persona

*  Final Assembler – The business entity that performs the final assembly of a product.

*  Part supplier – Supplier of parts. Suppliers can also be assemblers by assembling parts that they receive from their  sub-suppliers, and then sending their finished product to the final (root) assembler.

#### Key Components

*  Payment upon delivery of goods - Integration with off-chain payment systems is required, so that payment instructions can be sent when parts are received.

*  Third party Audit -  All supplied parts must be auditable by third parties. For example, regulators might need to track the total number of parts supplied by a specific supplier, for tax accounting purposes.

*  Obfuscation of shipments - Balances must be obfuscated so that no supplier can deduce the business activities of any other supplier.

*  Obfuscation of market size - Total balances must be obfuscated so that part suppliers cannot deduce their own market share to use as leverage when negotiating contractual terms.

*  Validation Access – Only nodes with validation rights are allowed to validate transactions (shipment of parts).

*  View access – Only accounts with view access rights are allowed to interrogate balances of shipped parts and available parts.

&nbsp;


### Asset Depository

Assets such as financial securities must be able to be dematerialized on a blockchain network so that all stakeholders of an asset type will have direct access to that asset, allowing them to initiate trades and acquire information on an asset without going through layers of intermediaries. Trades should be settled in near real time and all stakeholders must be able to access asset information in near real time. A stakeholder should be able to add business rules on any given asset type, as one example of using automation logic to further reduce operating costs.
<img src="../images/Canonical-Use-Cases_Asset-Depository.png" width="900" height="464">

#### Persona

*  Investor – Beneficial and legal owner of an asset.

*  Issuer – Business entity that issued the asset which is now dematerialized on the ledger network.

*  Custodian – Hired by investors to manage their assets, and offer other value-add services on top of the assets being managed.

*  Securities Depository – Depository of dematerialized assets.

#### Key Components

*  Asset to cash - Integration with off-chain payment systems is necessary so that issuers can make payments to and receive payments from investors.

*  Reference Rate - Some types of assets (such as floating rate notes) may have attributes linked to external data (such as  reference rate), and such information must be fed into the ledger network.

*  Asset Timer - Many types of financial assets have predefined life spans and are required to make periodic payments to their owners, so a timer is required to automate the operation management of these assets.

*  Asset Auditor - Asset transactions must be made auditable to third parties. For example, regulators may want to audit transactions and movements of assets to measure market risks.

*  Obfuscation of account balances - Individual account balances must be obfuscated so that no one can deduce the exact amount that an investor owns.

*  Validation Access – Only nodes with validation rights are allowed to validate transactions that update the balances of an asset type (this could be restricted to CSD and/or the issuer).

*  View access – Only accounts with view access rights are allowed to interrogate the chaincode that defines an asset type. If an asset represents shares of publicly traded companies, then the view access right must be granted to every entity on the network.

&nbsp;


# Extended Use Cases

The following extended use cases examine additional requirements and scenarios.

### One Trade, One Contract

From the time that a trade is captured by the front office until the trade is finally settled, only one contract that specifies the trade will be created and used by all participants. The middle office will enrich the same electronic contract submitted by the front office, and that same contract will then be used by counter parties to confirm and affirm the trade. Finally, securities depository will settle the trade by executing the trading instructions specified on the contract. When dealing with bulk trades, the original contract can be broken down into sub-contracts that are always linked to the original parent contract.

<img src="../images/Canonical-Use-Cases_One-Trade-One-Contract.png" width="900" height="624">

&nbsp;

### Direct Communication

Company A announces its intention to raise 2 Billion USD by way of rights issue. Because this is a voluntary action, Company A needs to ensure that complete details of the offer are sent to shareholders in real time, regardless of how many intermediaries are involved in the process (such as receiving/paying agents, CSD, ICSD, local/global custodian banks, asset management firms, etc). Once a shareholder has made a decision, that decision will also be processed and settled (including the new issuance of shares) in real time. If a shareholder sold its rights to a third party, the securities depository must be able to record the new shares under the name of their new rightful owner.

<img src="../images/Canonical-Use-Cases_Direct-Communication.png" width="900" height="416">

&nbsp;

### Separation of Asset Ownership and Custodian’s Duties

Assets should always be owned by their actual owners, and asset owners must be able to allow third-party professionals to manage their assets without having to pass legal ownership of assets to third parties (such as nominee or street name entities). If issuers need to send messages or payments to asset owners (for example, listed share holders), issuers send them directly to asset owners. Third-party asset managers and/or custodians can always buy, sell, and lend assets on behalf of their owners. Under this arrangement, asset custodians can focus on providing value-add services to shareowners, without worrying about asset ownership duties such as managing and redirecting payments from issuers to shareowners.

<img src="../images/Canonical-Use-Cases_Separation-of-Asset-Ownership-and-Custodians-Duties.png" width="900" height="628">

&nbsp;

### Interoperability of Assets

If an organization requires 20,000 units of asset B, but instead owns 10,000 units of asset A, it needs a way to exchange asset A for asset B. Though the current market might not offer enough liquidity to fulfill this trade quickly, there might be plenty of liquidity available between asset A and asset C, and also between asset C and asset B. Instead of settling for market limits on direct trading (A for B) in this case, a chain network connects buyers with "buried" sellers, finds the best match (which could be buried under several layers of assets), and executes the transaction.

<img src="../images/Canonical-Use-Cases_Interoperability-of-Assets.png" width="900" height="480">
