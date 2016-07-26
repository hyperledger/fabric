# Hyperledger Fabric - Asset Management App 

## Overview

The *Asset Management App* tests the behavior of *asset management* chaincode. The app bootstraps a non-validating peer and constructs fabric confidential transactions to deploy, invoke and query the asset management chaincode as described below.

In particular, we consider a scenario in which we have the following parties:

1. *Alice* is the chaincode deployer;
2. *Bob* is the chaincode administrator;
3. *Charlie* and *Bob* are asset owners;

that interact in the following way:

1. Alice deploys and assigns the administrator role to Bob;
2. Bob assigns the asset 'Picasso' to Charlie;
3. Charlie transfers the ownership of 'Picasso' to Dave;

In the following sections, we describe in more detail the interactions 
described above that the asset management app exercises.

### Alice deploys and assigns the administrator role to Bob

The following actions take place:

1. Alice obtains, via an out-of-band channel, a TCert of Bob, let us call this certificate *BobCert*;
2. Alice constructs a deploy transaction, as described in *application-ACL.md*,  setting the transaction metadata to *DER(BobCert)*.
3. Alice submits the deploy transaction to the fabric network.

Bob is now the administrator of the chaincode. 

Notice that, to assign ownership of assets, Bob has to use *BobCert* to authenticate his invocations.

### Bob assigns the asset 'Picasso' to Charlie

1. Bob obtains, via an out-of-band channel, a TCert of Charlie, let us call this certificate *CharlieCert*;
2. Bob constructs an invoke transaction, as described in *application-ACL.md* using *BobCert* to gain access, to invoke the *assign* function passing as parameters *('Picasso', Base64(DER(CharlieCert)))*. 
3. Bob submits the transaction to the fabric network.

Charlie is now the owner of 'Picasso'.

Notice that, to transfer the ownership of 'Picasso', Charlie has to use *CharlieCert* to authenticate his invocations.

### Charlie transfers the ownership of 'Picasso' to Dave

1. Charlie obtains, via an out-of-band channel, a TCert of Dave, let us call this certificate *DaveCert*;
2. Charlie constructs an invoke transaction, as described in *application-ACL.md* using *CharlieCert*, to invoke the *transfer* function passing as parameters *('Picasso', Base64(DER(DaveCert)))*. 
3. Charlie submits the transaction to the fabric network.

Dave is now the owner of 'Picasso'

## How To run the Asset Management App

In order to run the Asset Management App, the following steps are required. 

### Create app's configuration file

The app needs a 'core.yaml' configuration file in order to bootstrap a non-validating peer. This configuration file can be copied from *fabric/peer/core.yaml*. 

### Setup the fabric network

We consider the fabric network as described here: [https://github.com/hyperledger/fabric/blob/master/consensus/docker-compose-files/compose-consensus-4.md](https://github.com/hyperledger/fabric/blob/master/consensus/docker-compose-files/compose-consensus-4.md) that consists of 4 validators and the membership service.

Before setting up the network, the *fabric/peer/core.yaml* file must be modified by setting the following properties as specified below:

1. security.enabled = true
2. security.privacy = true

Moreover, the 'core.yaml' file used to configure the app must point to the fabric network setup above. This can be done by replacing:
1. '0.0.0.0' with 'vp0';
2. 'localhost' with membersrvc. 

### Run the app

Once the network is up, do the following:

1. From a vagrant terminal, run this command:
```
docker exec -it dockercomposefiles_cli_1 bash
```
2. Change folder to:
```
cd $GOPATH/src/github.com/hyperldger/fabric/examples/chaincode/go/asset_management/app
```
3. Build the app
```
go build
```
3. Run the app
```
./app
```

If everything works as expected then you should see the output of the app without errors.