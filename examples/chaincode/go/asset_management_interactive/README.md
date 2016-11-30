# Hyperledger Fabric - Interactive Asset Management App

## Overview

The *Interactive Asset Management App* consists of three separate applications which test the behavior of the standard *asset management* chaincode example. The apps each bootstrap a non-validating peer and construct fabric confidential transactions to deploy, invoke, and/or query the asset management chaincode as described below.

In particular, we consider a scenario in which we have the following parties:

1. *Alice* is the chaincode deployer, represented by `app1`
2. *Bob* is the chaincode administrator, represented by `app2`
3. *Charlie*, *Dave*, and *Edwina* are asset owners, each represented by a separate instance of `app3`

that interact in the following way:

1. *Alice* deploys and assigns the administrator role to *Bob*
2. *Bob* assigns the assets listed in [assets.txt](app2/assets.txt) to *Charlie*, *Dave*, and *Edwina*
3. *Charlie*, *Dave*, and *Edwina* can transfer the ownership of their assets between each other

In the following sections, we describe in more detail the interactions
described above that the asset management app exercises.

### Alice deploys and assigns the administrator role to Bob via `app1`

The following actions take place:

1. *Alice* obtains, via an out-of-band channel, the ECert of Bob, let us call these certificates *BobCert*
2. Alice constructs a deploy transaction, as described in *application-ACL.md*,  setting the transaction metadata to *DER(BobCert)*
3. Alice submits the deploy transaction to the fabric network

*Bob* is now the administrator of the chaincode.

### Bob assigns ownership of all the assets via `app2`

1. *Bob* obtains, via out-of-band channels, the ECerts of *Charlie*, *Dave*, and *Edwina*; let us call these certificates *CharlieCert*, *DaveCert*, and *EdwinaCert*, respectively
2. *Bob* constructs an invoke transaction, as described in *application-ACL.md* using *BobCert* to gain access, to invoke the *assign* function passing as parameters *('ASSET_NAME_HERE', Base64(DER(CharlieCert)))*
3. *Bob* submits the transaction to the fabric network
4. *Bob* repeates the above steps for every asset listed in [assets.txt](app2/assets.txt), rotating between *Charlie*, *Dave*, and *Edwina* as the assigned owner

*Charlie*, *Dave*, and *Edwina* are now each the owners of 1/3 of the assets.

Notice that, to assign ownership of assets, *Bob* has to use *BobCert* to authenticate his invocations.

### Charlie, Dave, and Edwina transfer the ownership of their assets between each other via separate instances of `app3`

1. *Charlie*, *Dave*, and *Edwina* each obtain, via out-of-band channels, the ECerts of the other owners; let us call these certificates *CharlieCert*, *DaveCert*, and *EdwinaCert*, respectively
2. Owner *X* constructs an invoke transaction, as described in *application-ACL.md* using *XCert*, to invoke the *transfer* function passing as parameters *('ASSET_NAME_HERE', Base64(DER(YCert)))*.
3. *X* submits the transaction to the fabric network.
4. These steps can be repeated for any asset to be transferred between any two users *X* -> *Y*, if and only if *X* is the current owner of the asset

*Y* is now the owner of the asset.

Notice that, to transfer the ownership of a given asset, user *X* has to use *XCert* to authenticate his/her invocations.

## How To run the Interactive Asset Management App

In order to run the Interactive Asset Management App, the following steps are required.

### Setup the fabric network and create app's configuration file

Follow the [create](https://github.com/hyperledger/fabric/blob/master/examples/chaincode/go/asset_management/app/README.md#create-apps-configuration-file) and [setup](https://github.com/hyperledger/fabric/blob/master/examples/chaincode/go/asset_management/app/README.md#setup-the-fabric-network) steps from the original asset management demo instructions. Note that the `core.yaml` that is described should be placed in this directory on the CLI node.

### Build the apps

Follow the first two instructions of the [run step](https://github.com/hyperledger/fabric/blob/master/examples/chaincode/go/asset_management/app/README.md#run-the-app) from the original asset management demo instructions, but change into the following directory instead:

```
cd $GOPATH/src/github.com/hyperldger/fabric/examples/chaincode/go/asset_management_interactive
```

Then build all three applications by issuing the following command:

```
./build.sh
```

### Run `app1`

Run `app1` by issuing the following command:

```
cd app1
./app1
```

Wait for `app1` to complete, and note the output value of `ChaincodeName`, which needs to be supplied to the following applications. For the rest of these instructions, we will use `8bc69f84aa55195b9eba6aed6261a01ba528acd3413f37b34df580f96e0d8a525e0fe8d41f967b38f192f7a731d63596eedcbd08ee636afdadb96be02d5d150d` as the `ChaincodeName` value.

### Run `app2`

Run `app2` by issuing the following command:

```
cd ../app2
./app2 8bc69f84aa55195b9eba6aed6261a01ba528acd3413f37b34df580f96e0d8a525e0fe8d41f967b38f192f7a731d63596eedcbd08ee636afdadb96be02d5d150d
```

### Run `app3` for each owner

Run `app3` for each owner by issuing the following commands (note that a separate terminal window for each owner is recommended):

```
cd ../app3
```

Run as *Charlie*:

```
./app3 8bc69f84aa55195b9eba6aed6261a01ba528acd3413f37b34df580f96e0d8a525e0fe8d41f967b38f192f7a731d63596eedcbd08ee636afdadb96be02d5d150d charlie
```

Run as *Dave*:

```
./app3 8bc69f84aa55195b9eba6aed6261a01ba528acd3413f37b34df580f96e0d8a525e0fe8d41f967b38f192f7a731d63596eedcbd08ee636afdadb96be02d5d150d dave
```

Run as *Edwina*:

```
./app3 8bc69f84aa55195b9eba6aed6261a01ba528acd3413f37b34df580f96e0d8a525e0fe8d41f967b38f192f7a731d63596eedcbd08ee636afdadb96be02d5d150d edwina
```

Each app will then provide a simple console interface for perfoming one of two actions: listing owned assets and transferring an asset to another owner.

#### List Assets

List an owner's assets by issuing the `list` command. For example, this is the expected output of *Charlie's* first `list`:

```
charlie$ list
...
'charlie' owns the following 16 assets:
'Lot01: BERNARD LEACH VASE WITH 'LEAPING FISH' DESIGN'
'Lot04: PETER LANYON TREVALGAN'
'Lot07: DAVID BOMBERG MOORISH RONDA, ANDALUCIA'
'Lot10: HAROLD GILMAN INTERIOR (MRS MOUNTER)'
'Lot13: WILLIAM SCOTT, R.A. GIRL SEATED AT A TABLE'
'Lot16: JACK BUTLER YEATS, R.H.A. SLEEP SOUND'
'Lot19: SIR EDUARDO PAOLOZZI, R.A. WEDDING OF THE ROCK AND DYNAMO'
'Lot22: JEAN-MICHEL BASQUIAT AIR POWER'
'Lot25: KENNETH ARMITAGE, R.A. MODEL FOR DIARCHY (SMALL VERSION)'
'Lot28: FRANK AUERBACH E.O.W'S RECLINING HEAD'
'Lot31: SIR EDUARDO PAOLOZZI, R.A. FORMS ON A BOW NO. 2'
'Lot34: MARCEL DUCHAMP WITH HIDDEN NOISE (A BRUIT SECRET)'
'Lot37: FRANCIS PICABIA MENDICA'
'Lot40: DAVID BOMBERG PLAZUELA DE LA PAZ, RONDA'
'Lot43: PATRICK CAULFIELD, R.A., C.B.E. FOYER'
'Lot46: DAMIEN HIRST UNTITLED FISH FOR DAVID'
```

#### Transfer Assets

Transfer an asset to another owner by issuing the `transfer LotXX owner_here` command. For example, this is the command to transfer *Lot01* from *Charlie* to *Dave*.

```
charlie$ transfer Lot01 dave
...
'dave' is the new owner of 'Lot01: BERNARD LEACH VASE WITH 'LEAPING FISH' DESIGN'!
------------- Done!
```

## Compatibility

This demo application has been successfully tested against the
[v0.6.0-preview](https://github.com/hyperledger/fabric/releases/tag/v0.6.0-preview) and
[v0.6.1-preview](https://github.com/hyperledger/fabric/releases/tag/v0.6.1-preview) releases.

## Copyright Note

The new source code files retain `Copyright IBM Corp.` because they are closely adopted from a preexisting example that contains an IBM copyright.