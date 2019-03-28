## Example Usage

### token issue example

You can use the following command to issue 100 `Fabcoins` belonging to
`User1@org1.example.com`. The tokens are issued by `Admin@org1.example.com`.

  * Use the `--config` flag to provide the path to a file that contains the
    connection information for your fabric network, including your
    **Prover peer**. You can find a sample configuration file [below](#configuration-file-example). Use the
    `--mspPath` flag to provide the path to the MSP of the token issuer.

    ```
    export CONFIG_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json
    export MSP_PATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    .
    token issue --config $CONFIG_FILE --mspPath $MSP_PATH --channel mychannel --type Fabcoins --quantity 100 --recipient Org1MSP:/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp
    .
    2019-03-28 18:19:29.438 UTC [token.client] BroadcastReceive -> INFO 001 calling OrdererClient.broadcastReceive
    Orderer Status [SUCCESS]
    Committed [true]
    ```

### token list example

You can use the `token list` command to discover the tokenIDs of the tokens that
you own.

  * Use the `--mspPath` flag to provide the path to the MSP of the token owner.

    ```
    export CONFIG_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json
    export MSP_PATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp
    .
    token list --config $CONFIG_FILE --mspPath $MSP_PATH --channel mychannel
    ```
    If successful, the command will return the tokenID, which is the ID of the
    transaction that created the token, as well as the type and quantity of
    assets represented by the token.
    ```
    {"tx_id":"23604056d205c656fa757f568a6a4f0105567ebc208303065aa7e5a11849c0c8"}
    [Fabcoins,100]
    ```

### token transfer example

You can transfer the tokens that you own to another member of the channel using
the `token transfer` command.

  * Use the ``--tokenIDs`` flag to select the tokens that you want to transfer.
    Use the ``--shares`` flag to provide a path to a JSON file that describes
    how the input token will be distributed to the recipients of the transaction.
    You can find a sample shares file [below](#shares-file-example).

    ```
    export CONFIG_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json
    export MSP_PATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp
    export SHARES=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/shares.json
    .
    token transfer --config $CONFIG_FILE --mspPath $MSP_PATH --channel mychannel --tokenIDs '[{"tx_id":"23604056d205c656fa757f568a6a4f0105567ebc208303065aa7e5a11849c0c8"}]' --shares $SHARES
    .
    2019-03-28 18:27:43.468 UTC [token.client] BroadcastReceive -> INFO 001 calling OrdererClient.broadcastReceive
    Orderer Status [SUCCESS]
    Committed [true]
    ```

### token redeem example

Redeemed tokens can no longer be transferred to other channel members. Tokens
can only be redeemed by their owner. You can use the following command to redeem
50 `Fabcoins`:

    ```
    export CONFIG_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/configorg1.json
    export MSP_PATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp
    .
    token redeem --config $CONFIG_FILE --mspPath $MSP_PATH --channel mychannel --tokenIDs '[{"tx_id":"30e6337fdc0d07a5c46f51d6b58c4958992e21fed0aed5c822b30f9f28366698"}]' --quantity 50
    .
    2019-03-28 18:29:29.656 UTC [token.client] BroadcastReceive -> INFO 001 calling OrdererClient.broadcastReceive
    Orderer Status [SUCCESS]
    Committed [true]
    ```

### Configuration file example

The configuration file provides the token CLI the endpoint information of your
network. The file include the **Prover Peer** that your organization will use to
assemble token transactions.

<details>
  <summary>
    **Sample configuration file**
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


### Shares file example

The shares file is used by the transfer transaction to allocate the assets
represented by the input token between the recipients of the transfer. Any
quantity of the input token that is not transferred to a recipient is
automatically provided to the original owner in a new token.

<details>
  <summary>
    **Sample shares file**
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



<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
