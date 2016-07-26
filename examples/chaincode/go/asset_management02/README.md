# Hyperledger Fabric - Asset Management 02 

&nbsp;

## Overview

 

The *Asset Management 02* chaincode demonstrates two sample usages of leveraging tcert-attributes in an blockchain based asset depository

 

 

<b>Multiple Account IDs per user:</b> 

 

With tcert-attribute support, users can select which attribute to include inside their TCert. This capability allows users to embed attested account IDs inside their TCerts, and for obfuscation purposes, each user may own many account IDs to mask their trading activities on a blockchain network.

 

<b>Attribute Based Access Control:</b>

 

With tcert-attribute based access control mechanism, chaincode owners can create restricted APIs that can only be invoked by users with specific roles (shown as an attested attribute value insider TCerts). For example, a chaincode could have an API to allow the issuer of an asset type to allocate assets to their investors.

 

 

## Chaincode Files

 

#### Asset.yaml

<p>

 

Asset.yaml is a modified version of membersrvc.yaml that contains the attributes values used to test this chaincode example with ```go test```. 

 

User roles are defined as attributes. User roles must be attested before they can be loaded into membership service module. Entries below show that user Admin has the "issuer" role and Alice has the "client" role:

 

 

```

attribute-entry-0: admin;bank_a;role;issuer;2015-01-01T00:00:00-03:00;;

attribute-entry-1: alice;bank_a;role;client;2016-01-01T00:00:00-03:00;;

```

 

All of Alice's account IDs (account1, account2, account3 ...) are also declared as separate attribute values inside this config file, Alice will be able to select which attested account ID(s) to include in her TCerts (more on this later): 

 

- Note: In the future version of the fabric, users should be able to use APIs to dynamically send/load attribute values to the membership service module

 

```

attribute-entry-2: alice;bank_a;account1;22222-00001;2016-01-01T00:00:00-03:00;;

attribute-entry-3: alice;bank_a;account2;22222-00002;2016-01-01T00:00:00-03:00;;

attribute-entry-4: alice;bank_a;account3;22222-00003;2016-01-01T00:00:00-03:00;;

attribute-entry-5: alice;bank_a;account4;22222-00004;2016-01-01T00:00:00-03:00;;

attribute-entry-6: alice;bank_a;contactInfo;alice@gmail.com;2016-01-01T00:00:00-03:00;;        

```

 

Attributes can also be used to store/proof attested information of an individual, such as an investor's contact and payment information. 

 

- Note: Ideally such information shall be encrypted so that only authorized parties can read them (e.g. an investor probably wouldn't want his payment information to be exposed to everyone owning a copy of this asset ledger)

 

```

attribute-entry-6: alice;bank_a;contactInfo;alice@gmail.com;2016-01-01T00:00:00-03:00;;        

```

 

&nbsp;

 

#### asset_management02.go 

<p>

 

 

Defines AssetManagementChaincode struct which declares the high level APIs that users of a blockchain network can invoke. This chaincode exposes the following APIs to invokers:

 

 

- Init - this method will initialize the asset depository table in its chaincode state

 

- assignOwnership -  assigns assets to a given account ID, only entities with the "issuer" role are allowed to call this function 

 

- transferOwnership - moves x number of assets from account A to account B

 

- getOwnerContactInformation - retrieves the contact information of the investor that owns a particular account ID

 

- getBalance - retrieves the account balance information of the investor that owns a particular account ID

 

Note: TCert related logics are delegated to CertHandler (cert\_handler.go) and chaincode state operation are delegated to DepositoryHandler (depository\_handler.go) 

 

 

#### cert_handler.go 

<p>

 

Defines the CertHandler struct which provides the APIs used to perform operations on incoming TCerts, such as to extracting attribute values from incoming TCerts and verify if the invoker has the required role.

 

- isAuthorized - check if the transaction invoker has the appropriate role, return false if it doesn't

 

- getContactInfo - retrieves the contact info stored as an attribute in a Tcert

 

- getAccountIDsFromAttribute - retrieves account IDs stored in the TCert param (attributes values)

 

 

#### depository_handler.go 

<p>

 

Defines the DepositoryHandler struct which provides APIs used to perform operations on Chaincode's Key Value store, such as to initiate the asset depository table and perform operations on its record rows.

 

- createTable - initiates a new asset depository table in the chaincode state

 

- assign - assign allocates assets to account IDs in the chaincode state for each of the

 

- updateAccountBalance - updateAccountBalance updates the balance amount of an account ID

 

- deleteAccountRecord - deletes the record row associated with an account ID on the chaincode state table

 

- transfer - transfers X amount of assets from "from account IDs" to a new account ID

 

- queryContactInfo - queries the contact information matching a correponding account ID on the chaincode state table

 

- queryBalance - queries the balance information matching a correponding account ID on the chaincode state table

 

- queryAccount - queries the balance and contact information matching a correponding account ID on the chaincode state table

 

- queryTable - returns the record row matching a correponding account ID on the chaincode state table

 

 

 

## Go Test & Workflow

 

asset\_management\_02\_test.go is the go test file for this asset management chaincode. Most of the codes inside the test file are used to launch membership service (which uses the asset.yaml config file included this example folder), initialize peer nodes, create test users (admin, alice, and bob), and package/sign chaincode transactions. 

 

These boiler plate logics are necessary to launch the unit tests included in this file, but are largely unimportant for the purpose of understanding this chaincode example, so they will not be covered in the following sections.

 

Three test users will be created to simulate the business workflows to be carries out by this test file:

 

- Admin - role: issuer

- Alice - role: client

- Bob - role: client

 

#### TestChaincodeDeploy

 

Test if the asset management chaincode can be succsessfully deployed

 

<b>Workflow</b>

 

1) Create a TCert for Admin, note this Admin TCert does not carry any attribute inside its body

 

```

administrator.GetTCertificateHandlerNext()

```

 

2) Use admin's TCert to create a deployment transaction to deploy asset management chaincode

 

3) Verify the results of the previous steps

 

 

&nbsp;

 

#### TestAuthorization

 

 

TestAuthorization tests attribute based role access control by making sure that only callers with "issuer" role are allowed to invoke the ```assign``` method and allocate assets to investors.

 

<b>Workflow</b>

 

1) create a TCert for Alice carrying her role, account ID(s), and contact information

 

```

alice.GetTCertificateHandlerNext("role", "account1", "contactInfo")

```

 

2) Use Alice's client to invoke ```assign``` function (since Alice does not have the "issuer" role, this operation must fail.)

 

3) Verify the results of the previous steps

 

&nbsp;

 

#### TestAssigningAssets

 

TestAssigningAssets tests the ```assign``` method by making sure authorized users (callers with 'issuer' role) can use the ```assign``` method to allocate assets to its investors

 

<b>Workflow</b>

 

1) Create three TCerts for Alice, each carrying one of Alice's account IDs

 

```

aliceCert1, err := alice.GetTCertificateHandlerNext("role", "account1", "contactInfo")

aliceCert2, err := alice.GetTCertificateHandlerNext("role", "account2", "contactInfo")

aliceCert3, err := alice.GetTCertificateHandlerNext("role", "account3", "contactInfo")

```

2) Create a TCert for Bob, 

 

```

bobCert1, err := bob.GetTCertificateHandlerNext("role", "account1", "contactInfo")

```

3) Assign balances to Alice and Bobs' account IDs, and record Alice and Bobs' contact information to the record rows storing their account information (Note: Each static attribute value within each TCert must be encrypted with a unique secret key per attribute per TCert, so that malicious users cannot link user records by looking at static user properties such as their email address)

 

- Alice's account1 = 100

 

- Alice's account2 = 200

 

- Alice's account3 = 300

 

- Bob's account1 = 1,000

 

4) Verify the results of the previous steps



&nbsp;
 

#### TestAssetTransfer


Test the ability for asset owners to transfer their assets to other owners.
 

<b>Workflow</b>


1) Create a new TCert for Alice. The number of account IDs to be embedded into this TCert depends on the total amount of assets Alice needs to transfer. That is, the sum of account balances of account IDs included in this TCert must be greater than the total amount that Alice will transfer to Bob in this transaction 
 

```

aliceCert, err := alice.GetTCertificateHandlerNext("role", "account1", "account2", "account3")

```
 

2) Create a new TCert for Bob. Note that for obfuscation purpose, Bob must pick a new account ID (Bob's account2) that was never used before.


```
bobCert, err := bob.GetTCertificateHandlerNext("role", "account2", "contactInfo")
``` 

3) Transfer X amount of assets from Alice to Bob

4) Verify the results of the previous steps




