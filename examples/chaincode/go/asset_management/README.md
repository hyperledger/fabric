# Hyperledger Fabric - Asset Management

## Overview

The *asset management* chaincode (*asset_management.go*) is a very simple chaincode designed to show how to exercise *access control* at the chaincode level as described in this document: [https://github.com/hyperledger/fabric/blob/master/docs/tech/application-ACL.md](https://github.com/hyperledger/fabric/blob/master/docs/tech/application-ACL.md)

The chaincode exposes the following functions:

1. *init(user)*: Initialize the chaincode assigning to *user* the role of *administrator*;
2. *assign(asset, user)*: Assigns the ownership of *asset* to *user*. 
Notice that, this function can be invoked only by an administrator;
3. *transfer(asset, user)*: Transfer the ownership of *asset* to *user*
Notice that this function ca be invoked only by the owner of *asset*;
4. *query(asset)*: Returns the identifier of the owner of *asset*

In the following subsections, we will describe in more detail each function.

## *init(user)*

This function initialize the chaincode by assigning to *user* the role of administrator. The function is invoked automatically at deploy time.

When generating the deploy transaction, the chaincode deployer must specify the administrator of the chaincode by setting the transaction metadata to 
the DER (Distinguished Encoding Rules) certificate encoding of one of the administrator ECert/TCert. 

For simplicity, there is only one administrator.

A possible work-flow could be the following:

1. Alice is the deployer of the chaincode;
2. Alice wants to assign the administrator role to Bob;
3. Alice obtains, via an out-of-band channel, a TCert of Bob, let us call this certificate *BobCert*;
4. Alice constructs a deploy transaction, as described in *application-ACL.md*,  setting the transaction metadata to *BobCert*.
5. Alice submits the transaction to the fabric network.

Notice that Alice can assign to herself the role of administrator.

## *assign(asset, user)*

This function assigns the ownership of *asset* to *user*. For simplicity, *asset* can be any string (the identifier of the asset, for example) and *user* is a TCert/ECert of the party the ownership of *asset* is assigned to.

Notice that, this function can only be invoked by the administrator of the chaincode that has been defined at deploy time during the chaincode initialization.

A possible work-flow could be the following:

1. Bob is the administrator of the chaincode;
2. Bob wants to assign the asset 'Picasso' to Charlie;
3. Bob obtains, via an out-of-band channel, a TCert of Charlie, let us call this certificate *CharlieCert*;
4. Bob constructs an invoke transaction, as described in *application-ACL.md*, to invoke the *assign* function passing as parameters *('Picasso', Base64(DER(CharlieCert)))*. 
5. Bob submits the transaction to the fabric network.

## *transfer(asset, user)*

This function transfers the ownership of *asset* to *user*. As for the *assign* function, *asset* is a string representing the asset and *user* is an ECert/TCert of the party the ownership of *asset* is assigned to.

Notice that, this function can only be invoked by the owner of *asset* who obtained the ownership via ans *assign* call or via a chain of *assign* and *transfer* calls.

A possible work-flow could be the following:

1. Charlie is the owner of 'Picasso';
2. Charlie wants to transfer the ownership of 'Picasso' to Dave;
3. Charlie obtains, via an out-of-band channel, a TCert of Dave, let us call this certificate *DaveCert*;
4. Charlie constructs an invoke transaction, as described in *application-ACL.md*, to invoke the *transfer* function passing as parameters *('Picasso', Base64(DER(DaveCert)))*. 
5. Charlie submits the transaction to the fabric network.

## *query(asset)*

This function returns the owner of *asset* as the DER certificate encoding of his certificate the ownership was acquired with.

Notice that, this function can be invoked by anyone. No access control is in place in this example. No one forbids to enhance the chaincode to have access control also for *query* function.