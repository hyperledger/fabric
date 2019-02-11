# Using FabToken

**FabToken** allows users to easily tokenize assets on Hyperledger Fabric.
Tokens are being introduced as an Alpha feature in Fabric v2.0. You can use
the following operations guide to learn about FabToken and get started with
tokens. You can find an example creating tokens on Fabric that extends the
Building your First Network tutorial at the end of this guide.

- [What is FabToken](#what-is-fabtoken)
- [The Token lifecycle](#the-token-lifecycle)
- [The Token transaction flow](#the-token-transaction-flow)
- [FabToken example](#fabtoken-example)
- [Future features](#future-features)

## What is FabToken

Representing assets as tokens allows you to use the blockchain ledger to
establish the unique state and ownership of an item, and transfer ownership
using a consensus mechanism that is trusted by multiple parties. As long as the
ledger is secure, the asset is immutable and cannot be transferred without the
owners consent.

Tokens can represent *tangible assets*, such as goods moving through a supply
chain or a financial instrument being traded. Tokens can also represent
*intangible assets* such as loyalty points. Because tokens cannot be transferred
without the consent of the owner, and transactions are validated on a
distributed ledger, representing assets as tokens allows you to reduce the risk
and difficulty of transferring assets across multiple parties.

**FabToken** is a token management system that allows you to issue, transfer,
and redeem tokens using Hyperledger Fabric. Tokens are stored on channel
ledgers and can be owned by any member of the channel. FabToken uses the
membership services of Fabric to authenticate the identity of token owners and
manage their public and private keys. Fabric token transactions are only valid
if they are issued by a token owner with a valid MSP identifier.

FabToken provides a simple interface for tokenizing assets on Fabric channels,
while taking advantage of the validation and trust that channels provide. Tokens
use the channel ordering services and peers for consensus and validation. Tokens
also use channel policies to govern which members are allowed to own and issue
tokens. However, users do not need to use smart contracts to create or manage
tokens. Tokens can establish immutability and ownership of an asset without
requiring that channel members write and approve complicated business logic to
create and govern those assets. Token owners can use trusted peers to create
token transactions, without having to rely on peers belonging to other
organizations to execute and endorse a transaction.

## The token lifecycle

Tokens have a closed lifecycle within Hyperledger Fabric. They can be **issued**,
**transferred**, and then **redeemed**.

- Tokens are created by being **issued**. The token issuer defines the type of
  asset represented by the tokens and the quantity. The issuer also assigns
  issued tokens to their original owners.

- Tokens are "spent" by being **transferred**. The token owner transfers the asset
  represented by token to a new owner that is a member of the fabric channel.
  Once the token has been transferred, it can no longer be spent or accessed by
  the previous owner.

- Tokens are removed from the channel by being **redeemed**. Redeemed tokens are
  no longer owned by any channel member and thus can no longer be spent.

FabToken uses an **Unspent Transaction Output (UTXO)** model to validate token
transactions. UTXO transactions are a powerful guarantee that an asset is
unique, can only be transferred by its owner, and cannot be double spent. Each
transaction needs to have a specific set of outputs and inputs. The output are
new tokens created by the transaction. These are listed on the ledger in an
"unspent" state. The input needs to be unspent tokens created as the output of
another transaction. When a transaction is validated, the spent tokens are
destroyed by being removed from the state database of the channel ledger.

The token lifecycle builds on the UTXO model to ensure that tokens are unique and
can only be spent once. When a token is **issued**, it is created in an unspent
state belonging to the owner that was specified by the issuer. The owner can then
transfer or redeem the token. When the token is **transferred**, the token owned
by the creator is taken as input. The output of the transaction are new tokens
owned by the recipients of the transfer. The input token becomes "spent" and is
removed from the state database. The quantity of assets represented by the
tokens transferred need to be of the same quantity as the output. Tokens that are
**redeemed** are transferred to an empty owner. This makes redeemed tokens
impossible to transfer again by any member of the channel.

The following guide describes how tokens are created and used in Fabric. The
instructions provide details on what steps and information are required to work
with FabToken whether you are using the Fabric token client, the API's provided
by the Fabric SDKs, or Token CLI. You can find a [FabToken example](#fabtoken-example)
at the end of this guide.

### Issuing tokens

Tokens can only be created by **Issuers**. Issuers are channel members that are
given permission to issue tokens by the `IssuingPolicy`. Users that meet the
policy can add tokens to the ledger using an `issue` transaction.

Tokens are created with three attributes:

- `Owner` identifies the channel member that can transfer or redeem the new
  token through its MSP identity.
- `Type` describes the asset the token represents, such as USD, EUR, or
  BYFNcoins in the example below.
- `Quantity` is the number of units of `Type` that the `Token` represents.

For example, each token of type `US dollar` can represent 100 dollars. Each
dollar does not need to be a separate token. In order to spend 50 dollars of
the token, or add 50 dollars, new tokens are created which represent the new
quantities.

The `IssuingPolicy` can also restrict which users can issue tokens of specific
types. Within the Fabric v2.0 Alpha release, `IssuingPolicy` is set to `ANY`,
meaning that all channel members will be allowed to issue tokens of any type.
Users will be allowed to restrict this policy in a future release.

### List

You can use the `List` method or command to query the unspent tokens that you
own. A successful list command returns the following values:

- `TokenID` is the identifier of each token you own.
- `Type` is the asset your tokens represent.
- `Quantity` is number of units of `Type` in hexadecimal format of each asset
  that you own.

### Transfer

You can spend the tokens that you own by transferring them to other channel
members. You can transfer a token by providing the following values:

- `Token ID`: The ID of the tokens you want to transfer.
- `Quantity`: The amount of the asset represented by each token to be
  transferred.
- `Recipient`: The MSP identifier of the channel member you want to transfer the
  assets to.

Note that the `transfer` transaction is against the underlying asset that the
tokens represent, and does not transfer the tokens themselves. Rather, new tokens
are created by the transfer transaction. For example, if you own a token that is
worth 100 dollars, you can spend 50 dollars using that token. The transfer
transaction will create two new tokens as output. One token worth 50 dollars will
belong to you, and another token worth 50 dollars will belong to the recipient.

The quantity of the assets being transferred to the recipients of the transaction
needs to be the same quantity as the input tokens. If you do not want to
transfer the entire quantity of the asset represented by the token, you can
transfer a portion of the asset and the transaction will automatically make you
the owner of the remaining balance. Using the example above, if only spend 50
dollars of the 100 dollar token, the transfer transaction will automatically
create a new token worth 50 dollars with you as the owner.

To be successful, a transfer needs to meet the following conditions:

- The tokens being transferred need to belong to the transaction initiator and
  are unspent.
- All input tokens of the transaction need to be of the same type.

### Redeem

Redeemed tokens can no longer be spent. Redeeming a token removes an asset from
the business network being managed by the channel and guarantees that it can no
longer be transferred or changed. If an item in a supply chain reaches its final
destination, or a financial asset reaches its term, the token representing the
asset can be redeemed since the asset no longer needs to be used by the members
of the channel.

An owner needs to provide the following arguments to `redeem` tokens:
- `Token ID`: The ID of the token you want to redeem.
- `Quantity`: The quantity of the asset represented by each token you want to
  redeem.

Tokens can only be redeemed if the token owner submits the redeem transaction.
It is not necessary to redeem the entire quantity of the asset represented by
the token. For example, if you have a token representing 100 dollars, and want
to redeem 50, the redeem transaction will create a new token worth 50 dollars,
and transfer another 50 to a restricted account without an owner. Because the
account has no owner, the 50 dollars can no longer be transferred by any members
of the channel.

## The token transaction flow

Fabtoken bypasses the standard Hyperledger Fabric endorsement flow. Transactions
against chaincode need to be invoked on the peers of enough organizations to
meet the chaincode endorsement policy. This ensures that the result of the
transaction is consistent with the logic of the smart contract and that the
result of that logic has been validated by multiple organizations. Because
tokens are unique representations of an asset that can only be transferred or
redeemed by their owner, there is no need for multiple organizations to validate
the initial transaction.

The FabToken client used by the token CLI and the Fabric SDK for Node.js leverages
trusted peers, referred to as **prover peers** to create token transactions.
For example, a user belonging to an organization that operates a peer could use
that peer to query their tokens and spend them. Any peer with the Fabric 2.0
Alpha code can be used as a prover peer if it is joined to a channel with `V2_0`
capabilities enabled.

- In the case of an `issue` transaction, the prover peer will verify that
  the requested operation satisfies the `IssuingPolicy` associated with the
  tokens being created.
- In the case of `transfer`, `redeem` and `list`, the peer checks that the
  input tokens are unspent and belong to the entity requesting the transaction.
- In the case of `transfer` and `redeem`, the peer checks that the input
  and output tokens are all of the same `type` and that the output tokens have
  the same `type` and sum up to the same `quantity` as the input tokens.

Once the client has generated the token transaction with the help of the prover
peer, it sends the transaction to the ordering service. The ordering service
then sends the transaction to **committing peers** to be validated and added
to the ledger. The committing peers check that the transaction conforms to the
**UTXO** transaction model, and that the underlying asset is not being double
spent or over spent.

## FabToken Example

You can try working with tokens yourself using the sample network inside the
[Building your first network tutorial](../build_network.html) to issue and transfer
tokens. In this example, we will use the Token CLI to trade some tokenized
_BYFNcoins_  on a channel created by the `./byfn.sh` script.


You can also work with tokens using the Fabric SDK for Node.js. Visit the
[How to perform token operations](https://fabric-sdk-node.github.io/master/tutorial-fabtoken.html) tutorial in the Node.js Fabric SDK documentation. You can also find a sample that uses the Node.js
Fabric SDK to issue, transfer, and redeem tokens in the
[Fabric Samples repository](https://github.com/hyperledger/fabric-samples/tree/master/fabtoken).

### Start the network

The first step is to bring up the sample network. The `./byfn.sh` script
creates a Fabric network with two organizations, Org1 and Org2, with peers
joined to a channel called `mychannel`. We are going to use `mychannel`
to issue tokens and transfer them between Org1 and Org2.

First we need to clean up our environment. The following command will
navigate to the `fabric-samples` directory, kill any active or stale Docker
containers, and remove previously generated artifacts:

```
cd fabric-samples/first-network
./byfn.sh down
```
You first need to generate the artifacts required by the sample network. Run the
following command:
```
./byfn.sh generate
```

We need to add some files that we will need in future steps. Navigate to the
`crypto-config` directory inside the `first-network` directory.
```
cd crypto-config
```
The Token CLI uses configuration files from each organization with information
about which peers the organization trusts, and which ordering service to send
the transactions. Below is the configuration file for Org1. Notice that Org1
uses its own peer as a prover peer, and provides the peer endpoint information
in the `"ProverPeer"` section of the file.

<details>
  <summary>
    **Org1 Configuration file**
  </summary>
```
{
  "ChannelID":"",
  "MSPInfo":{
    "MSPConfigPath":"",
    "MSPID":"Org1MSP",
    "MSPType":"bccsp"
  },
  "Orderer":{
    "Address":"orderer.example.com:7050",
    "ConnectionTimeout":0,
    "TLSEnabled":true,
    "TLSRootCertFile":"/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem",
    "ServerNameOverride":""
  },
  "CommitterPeer":{
    "Address":"peer0.org1.example.com:7051",
    "ConnectionTimeout":0,
    "TLSEnabled":true,
    "TLSRootCertFile":"/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt",
    "ServerNameOverride":""
  },
  "ProverPeer":{
    "Address":"peer0.org1.example.com:7051",
    "ConnectionTimeout":0,
    "TLSEnabled":true,
    "TLSRootCertFile":"/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt",
    "ServerNameOverride":""
  }
}
```
</details>

Paste the file above in a text editor and save it as ``configorg1.json``.
Once you have saved ``configorg1.json``, create a new file in your text editor,
and paste the JSON file below. Save the file as ``configorg2.json`` in the same
location:

<details>
  <summary>
    **Org2 Configuration file**
  </summary>
```
{
  "ChannelID":"",
  "MSPInfo":{
    "MSPConfigPath":"",
    "MSPID":"Org2MSP",
    "MSPType":"bccsp"
  },
  "Orderer":{
    "Address":"orderer.example.com:7050",
    "ConnectionTimeout":0,
    "TLSEnabled":true,
    "TLSRootCertFile":"/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem",
    "ServerNameOverride":""
  },
  "CommitterPeer":{
    "Address":"peer0.org2.example.com:9051",
    "ConnectionTimeout":0,
    "TLSEnabled":true,
    "TLSRootCertFile":"/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt",
    "ServerNameOverride":""
  },
  "ProverPeer":{
    "Address":"peer0.org2.example.com:9051",
    "ConnectionTimeout":0,
    "TLSEnabled":true,
    "TLSRootCertFile":"/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt",
    "ServerNameOverride":""
  }
}
```
</details>

We now need to save one additional file that we will use when we transfer our
tokens. Create a new file in your text editor, and save the file below as
``shares.json``:

<details>
  <summary>
    **shares.json**
  </summary>
```
[
    {
    "recipient":"Org2MSP:/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/User1@org2.example.com/msp",
    "quantity":"50"
    }
]
```
</details>

You are now ready to navigate back to the `fabric-samples` directory and bring
up the sample network:
```
cd ..
/byfn.sh up
```

The command will create the organizations, peers, ordering service, and channel
we will use to issue and transfer the tokens. When the command completes
successfully, you should see the following result:

```
========= All GOOD, BYFN execution completed ===========

 _____   _   _   ____
| ____| | \ | | |  _ \
|  _|   |  \| | | | | |
| |___  | |\  | | |_| |
|_____| |_| \_| |____/

```

### Issue tokens

We are going to tokenize 100 **BYFNcoins**, which can only be issued and traded by
our trusted friends on our sample network. Navigate into the CLI container using
the following command:

```
docker exec -it cli bash
```

Use the command below to issue a token worth 100 BYFNcoins as the Org1 admin.
The command uses the `configorg1.json` to find the endpoint of org1's prover
peer, which it will use to assemble the transaction. Note that the Org1
administrator submits the transaction, but the User1 of Org1 will be the token
owner.

```
# Issue the token as Org1

token issue --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp --channel mychannel --type BYFNcoins --quantity 100 --recipient Org1MSP:/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp
```

A successful command will generate a response similar to the following:

```
2019-03-12 00:49:43.864 UTC [token.client] BroadcastReceive -> INFO 001 calling OrdererClient.broadcastReceive
Orderer Status [SUCCESS]
Committed [true]
```

You can use the list command to view the token that was created. This command is
issued by User1, which is the owner of new token.

```
# List the tokens belonging to User1 of Org1

token list --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp --channel mychannel
```

A successful command will generate a response similar to the following:
```
{"tx_id":"4e2664225d6a67508cfa539108383e682f3d03debb768aa7920851fdeea6f5b7"}
[BYFNcoins,100]
```

In the command output, you can find the tokenID, the token `type`, and the
`quantity`. The tokenID is the transactionID of the transaction that created the
token.

### Transferring tokens

Now that the tokens have been created, User1 of Org1 can now spend the token
by transferring BYFNcoins to another user. User1 of Org1 will give User1 of
Org2 50 BYFNcoins, while keeping 50 for himself.

Use the command below to initiate the transfer. Use the ``tokenIDs`` flag to
transfer the tokenID returned by the list flag. Notice how the ``-- Shares``
flag passes the Token CLI a JSON file that allocates 50 BYFNcoins to User1 in
Org2. This is the file that you created in the `crypto-config` folder before
you started the network. Because the input token represents 100 BYFNcoins, the
transfer transaction will automatically create a new token belonging to User1 of
Org1 that represents the 50 BYFNcoins that were not transferred to Org2.

```
# Transfer 50 BYFNcoins to User1 of Org2
# The split of coins tranfered to Org1 and Org2 is in shares.json

token transfer --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp --channel mychannel --tokenIDs '[{"tx_id":"4e2664225d6a67508cfa539108383e682f3d03debb768aa7920851fdeea6f5b7"}]' --shares /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/shares.json
```
Once you have submitted the command above, you can run the `list` command again to
verify that User1 of Org1 now only has 50 BYFNcoins:
```
# List the tokens belonging to User1 of Org1

token list --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp --channel mychannel
```
Note that the BYFNcoins have a different tokenID than the previous coins. The
transfer destroyed the previous token and created a new token worth 50 BYFNcoins.
```
{"tx_id":"4eaf466884586106f480dd0bb4f675ddaa54d1290ea53e9c24a2c1344fb71d2c"}
[BYFNcoins,50]
```

You can run the command below to verify that User1 of Org2 received the
50 BYFNcoins:

```
# List the tokens belonging to User1 of Org2

token list --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg2.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/User1@org2.example.com/msp --channel mychannel
```

The tokenID of the coins owned by Org2 uses the same transaction ID as the coins
owned by Org1 since it was created by the same transaction. However, because it
was the second output of the transaction, it is also given an index to
distinguish it from the token owned by Org1.

```
{"tx_id":"4eaf466884586106f480dd0bb4f675ddaa54d1290ea53e9c24a2c1344fb71d2c","index":1}
[BYFNcoins,50]
```

## Redeeming tokens

Tokens can only be redeemed by their owners. Once an asset represented by a
token is redeemed, the token can no longer be transferred to any other owners.

Use the command below to redeem 25 BYFNcoins belonging to Org2.

```
# Redeem tokens belonging to User1 of Org2

token redeem --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg2.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/User1@org2.example.com/msp --channel mychannel  --tokenIDs '[{"tx_id":"4eaf466884586106f480dd0bb4f675ddaa54d1290ea53e9c24a2c1344fb71d2c","index":1}]' --quantity 25
```

Org2 now only has one token worth 25 BYFNcoins. Use the list command to verify
the number of BYFNcoins owned by User1 of Org2.

```
# List the tokens belonging to User1 of Org2

token list --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg2.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/User1@org2.example.com/msp --channel mychannel
```

Note that the new TokenID created as output of the redeem transaction.

Let's try to redeem tokens that belong to another user. Use the command below to
attempt as Org2 to redeem the token worth 50 BYFNcoins that belongs to Org1:

```
# Redeem tokens as Org1 belonging to Org2

token redeem --config /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg2.json --mspPath /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/User1@org2.example.com/msp --channel mychannel  --tokenIDs '[{"tx_id":"4eaf466884586106f480dd0bb4f675ddaa54d1290ea53e9c24a2c1344fb71d2c"}]' --quantity 50
```

The result will be the following error:
```
error from prover: the requestor does not own inputs
```

## Future features

The FabToken Alpha only supports limited issuing and trading functionality.
Future releases will provide users a greater ability to integrate tokens into
business logic by supporting **non-fungible tokens** and
**chaincode interoperability**,

Non fungible tokens cannot be merged or divided. Once they are created, they can
only be transferred to a new owner or redeemed. You can use non-fungible tokens
to represent unique assets such a concert ticket that is mapped to a particular
seat.

Chaincode interoperability allows tokens to be issued, transferred, and redeemed
by chaincode. This would allow the channel to issue and define tokens using
business logic agreed to by members of the channel. For example, you can use
chaincode to set the attributes of a chaincode, and associate certain attributes
with different transactions.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
