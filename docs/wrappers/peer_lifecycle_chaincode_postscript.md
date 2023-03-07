
## Example Usage

### peer lifecycle chaincode package example

A chaincode needs to be packaged before it can be installed on your peers.
This example uses the `peer lifecycle chaincode package` command to package
a Go chaincode.

  * Use the `--path` flag to indicate the location of the chaincode.
    The path must be a fully qualified path or a path relative to your present working directory.
  * Use the `--label` flag to provide a chaincode package label of `myccv1`
    that your organization will use to identify the package.

    ```
    peer lifecycle chaincode package mycc.tar.gz --path $CHAINCODE_DIR --lang golang --label myccv1
    ```

### peer lifecycle chaincode install example

After the chaincode is packaged, you can use the `peer chaincode install` command
to install the chaincode on your peers.

  * Install the `mycc.tar.gz ` package on `peer0.org1.example.com:7051` (the
    peer defined by `--peerAddresses`).

    ```
    peer lifecycle chaincode install mycc.tar.gz --peerAddresses peer0.org1.example.com:7051
    ```
    If successful, the command will return the package identifier. The
    package ID is the package label combined with a hash of the chaincode
    package taken by the peer.
    ```
    2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nEmycc:ebd89878c2bbccf62f68c36072626359376aa83c36435a058d453e8dbfd894cc" >
    2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: mycc:a7ca45a7cc85f1d89c905b775920361ed089a364e12a9b6d55ba75c965ddd6a9
    ```

### peer lifecycle chaincode queryinstalled example

You need to use the chaincode package identifier to approve a chaincode
definition for your organization. You can find the package ID for the
chaincodes you have installed by using the
`peer lifecycle chaincode queryinstalled` command:

```
peer lifecycle chaincode queryinstalled --peerAddresses peer0.org1.example.com:7051
```

A successful command will return the package ID associated with the
package label.

```
Get installed chaincodes on peer:
Package ID: myccv1:a7ca45a7cc85f1d89c905b775920361ed089a364e12a9b6d55ba75c965ddd6a9, Label: myccv1
```

  * You can also use the `--output` flag to have the CLI format the output as
    JSON.

    ```
    peer lifecycle chaincode queryinstalled --peerAddresses peer0.org1.example.com:7051 --output json
    ```

    If successful, the command will return the chaincodes you have installed as JSON.

    ```
    {
      "installed_chaincodes": [
        {
          "package_id": "mycc_1:aab9981fa5649cfe25369fce7bb5086a69672a631e4f95c4af1b5198fe9f845b",
          "label": "mycc_1",
          "references": {
            "mychannel": {
              "chaincodes": [
                {
                  "name": "mycc",
                  "version": "1"
                }
              ]
            }
          }
        }
      ]
    }
    ```

### peer lifecycle chaincode getinstalledpackage example

You can retrieve an installed chaincode package from a peer using the
`peer lifecycle chaincode getinstalledpackage` command. Use the package
identifier returned by `queryinstalled`.

  * Use the `--package-id` flag to pass in the chaincode package identifier. Use
  the `--output-directory` flag to specify where to write the chaincode package.
  If the output directory is not specified, the chaincode package will be written
  in the current directory.

  ```
  peer lifecycle chaincode getinstalledpackage --package-id myccv1:a7ca45a7cc85f1d89c905b775920361ed089a364e12a9b6d55ba75c965ddd6a9 --output-directory /tmp --peerAddresses peer0.org1.example.com:7051
  ```

### peer lifecycle chaincode calculatepackageid example

You can calculate the package ID from a packaged chaincode without installing the chaincode on peers
using the `peer lifecycle chaincode calculatepackageid` command.
This command will be useful, for example, in the following cases:

  * When multiple chaincode packages with the same label name are installed,
  it is possible to identify which ID corresponds to which package later.
  * To check whether a particular chaincode package is installed or not without
  installing that package.

Calcuate the package ID for the `mycc.tar.gz` package:

```
peer lifecycle chaincode calculatepackageid mycc.tar.gz
```

A successful command will return the package ID for the packaged chaincode.

```
myccv1:cc7bb5f50a53c207f68d37e9423c32f968083282e5ffac00d41ffc5768dc1873
```

  * You can also use the `--output` flag to have the CLI format the output as JSON.

    ```
    peer lifecycle chaincode calculatepackageid mycc.tar.gz --output json
    ```

    If successful, the command will return the chaincode package ID as JSON.

    ```
    {
      "package_id": "myccv1:cc7bb5f50a53c207f68d37e9423c32f968083282e5ffac00d41ffc5768dc1873"
    }
    ```

### peer lifecycle chaincode approveformyorg example

Once the chaincode package has been installed on your peers, you can approve
a chaincode definition for your organization. The chaincode definition includes
the important parameters of chaincode governance, including the chaincode name,
version and the endorsement policy.

Here is an example of the `peer lifecycle chaincode approveformyorg` command,
which approves the definition of a chaincode  named `mycc` at version `1.0` on
channel `mychannel`.

  * Use the `--package-id` flag to pass in the chaincode package identifier. Use
    the `--signature-policy` flag to define an endorsement policy for the chaincode.
    Use the `init-required` flag to require the execution of an initialization function
    before other chaincode functions can be called.

    ```
    export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

    peer lifecycle chaincode approveformyorg  -o orderer.example.com:7050 --tls --cafile $ORDERER_CA --channelID mychannel --name mycc --version 1.0 --init-required --package-id myccv1:a7ca45a7cc85f1d89c905b775920361ed089a364e12a9b6d55ba75c965ddd6a9 --sequence 1 --signature-policy "AND ('Org1MSP.peer','Org2MSP.peer')"

    2019-03-18 16:04:09.046 UTC [cli.lifecycle.chaincode] InitCmdFactory -> INFO 001 Retrieved channel (mychannel) orderer endpoint: orderer.example.com:7050
    2019-03-18 16:04:11.253 UTC [chaincodeCmd] ClientWait -> INFO 002 txid [efba188ca77889cc1c328fc98e0bb12d3ad0abcda3f84da3714471c7c1e6c13c] committed with status (VALID) at peer0.org1.example.com:7051
    ```

  * You can also use the `--channel-config-policy` flag use a policy inside
    the channel configuration as the chaincode endorsement policy. The default
    endorsement policy is `Channel/Application/Endorsement`

    ```
    export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

    peer lifecycle chaincode approveformyorg -o orderer.example.com:7050 --tls --cafile $ORDERER_CA --channelID mychannel --name mycc --version 1.0 --init-required --package-id myccv1:a7ca45a7cc85f1d89c905b775920361ed089a364e12a9b6d55ba75c965ddd6a9 --sequence 1 --channel-config-policy Channel/Application/Admins

    2019-03-18 16:04:09.046 UTC [cli.lifecycle.chaincode] InitCmdFactory -> INFO 001 Retrieved channel (mychannel) orderer endpoint: orderer.example.com:7050
    2019-03-18 16:04:11.253 UTC [chaincodeCmd] ClientWait -> INFO 002 txid [efba188ca77889cc1c328fc98e0bb12d3ad0abcda3f84da3714471c7c1e6c13c] committed with status (VALID) at peer0.org1.example.com:7051
    ```

### peer lifecycle chaincode queryapproved example

You can query an organization's approved chaincode definition by using the `peer lifecycle chaincode queryapproved` command.
You can use this command to see the details (including package ID) of approved chaincode definitions.

  * Here is an example of the `peer lifecycle chaincode queryapproved` command,
    which queries the approved definition of a chaincode named `mycc` at sequence number `1` on
    channel `mychannel`.

    ```
    peer lifecycle chaincode queryapproved -C mychannel -n mycc --sequence 1

    Approved chaincode definition for chaincode 'mycc' on channel 'mychannel':
    sequence: 1, version: 1, init-required: true, package-id: mycc_1:d02f72000e7c0f715840f51cb8d72d70bc1ba230552f8445dded0ec8b6e0b830, endorsement plugin: escc, validation plugin: vscc
    ```

    If NO package is specified for the approved definition, this command will display an empty package ID.

  * You can also use this command without specifying the sequence number in order to query the latest approved definition (latest: the newer of the currently defined sequence number and the next sequence number).

    ```
    peer lifecycle chaincode queryapproved -C mychannel -n mycc

    Approved chaincode definition for chaincode 'mycc' on channel 'mychannel':
    sequence: 3, version: 3, init-required: false, package-id: mycc_1:d02f72000e7c0f715840f51cb8d72d70bc1ba230552f8445dded0ec8b6e0b830, endorsement plugin: escc, validation plugin: vscc
    ```

  * You can also use the `--output` flag to have the CLI format the output as
    JSON.

    - When querying an approved chaincode definition for which package is specified

      ```
      peer lifecycle chaincode queryapproved -C mychannel -n mycc --sequence 1 --output json
      ```

      If successful, the command will return a JSON that has the approved chaincode definition for chaincode `mycc` at sequence number `1` on channel `mychannel`.

      ```
      {
        "sequence": 1,
        "version": "1",
        "endorsement_plugin": "escc",
        "validation_plugin": "vscc",
        "validation_parameter": "EiAvQ2hhbm5lbC9BcHBsaWNhdGlvbi9FbmRvcnNlbWVudA==",
        "collections": {},
        "init_required": true,
        "source": {
          "Type": {
            "LocalPackage": {
              "package_id": "mycc_1:d02f72000e7c0f715840f51cb8d72d70bc1ba230552f8445dded0ec8b6e0b830"
            }
          }
        }
      }
      ```

    - When querying an approved chaincode definition for which package is NOT specified

      ```
      peer lifecycle chaincode queryapproved -C mychannel -n mycc --sequence 2 --output json
      ```

      If successful, the command will return a JSON that has the approved chaincode definition for chaincode `mycc` at sequence number `2` on channel `mychannel`.

      ```
      {
        "sequence": 2,
        "version": "2",
        "endorsement_plugin": "escc",
        "validation_plugin": "vscc",
        "validation_parameter": "EiAvQ2hhbm5lbC9BcHBsaWNhdGlvbi9FbmRvcnNlbWVudA==",
        "collections": {},
        "source": {
          "Type": {
            "Unavailable": {}
          }
        }
      }
      ```

### peer lifecycle chaincode checkcommitreadiness example

You can check whether a chaincode definition is ready to be committed using the
`peer lifecycle chaincode checkcommitreadiness` command, which will return
successfully if a subsequent commit of the definition is expected to succeed. It
also outputs which organizations have approved the chaincode definition. If an
organization has approved the chaincode definition specified in the command, the
command will return a value of true. You can use this command to learn whether enough
channel members have approved a chaincode definition to meet the
`/Channel/Application/Endorsement` policy (a majority by default) before the
definition can be committed to a channel.

  * Here is an example of the `peer lifecycle chaincode checkcommitreadiness` command,
    which checks a chaincode named `mycc` at version `1.0` on channel `mychannel`.

    ```
    peer lifecycle chaincode checkcommitreadiness --channelID mychannel --name mycc --version 1.0 --init-required --sequence 1
    ```

    If successful, the command will return the organizations that have approved
    the chaincode definition.

    ```
    Chaincode definition for chaincode 'mycc', version '1.0', sequence '1' on channel
    'mychannel' approval status by org:
    Org1MSP: true
    Org2MSP: true
    ```

  * You can also use the `--output` flag to have the CLI format the output as
    JSON.

    ```
    peer lifecycle chaincode checkcommitreadiness --channelID mychannel --name mycc --version 1.0 --init-required --sequence 1 --output json
    ```

    If successful, the command will return a JSON map that shows if an organization
    has approved the chaincode definition.

    ```
    {
       "Approvals": {
          "Org1MSP": true,
          "Org2MSP": true
       }
    }
    ```

### peer lifecycle chaincode commit example

Once a sufficient number of organizations approve a chaincode definition for
their organizations (a majority by default), one organization can commit the
definition the channel using the `peer lifecycle chaincode commit` command:

  * This command needs to target the peers of other organizations on the channel
    to collect their organization endorsement for the definition.

    ```
    export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

    peer lifecycle chaincode commit -o orderer.example.com:7050 --channelID mychannel --name mycc --version 1.0 --sequence 1 --init-required --tls --cafile $ORDERER_CA --peerAddresses peer0.org1.example.com:7051 --peerAddresses peer0.org2.example.com:9051

    2019-03-18 16:14:27.258 UTC [chaincodeCmd] ClientWait -> INFO 001 txid [b6f657a14689b27d69a50f39590b3949906b5a426f9d7f0dcee557f775e17882] committed with status (VALID) at peer0.org2.example.com:9051
    2019-03-18 16:14:27.321 UTC [chaincodeCmd] ClientWait -> INFO 002 txid [b6f657a14689b27d69a50f39590b3949906b5a426f9d7f0dcee557f775e17882] committed with status (VALID) at peer0.org1.example.com:7051
    ```

### peer lifecycle chaincode querycommitted example

You can query the chaincode definitions that have been committed to a channel by
using the `peer lifecycle chaincode querycommitted` command. You can use this
command to query the current definition sequence number before upgrading a
chaincode.

  * You need to supply the chaincode name and channel name in order to query a
    specific chaincode definition and the organizations that have approved it.

    ```
    peer lifecycle chaincode querycommitted --channelID mychannel --name mycc --peerAddresses peer0.org1.example.com:7051

    Committed chaincode definition for chaincode 'mycc' on channel 'mychannel':
    Version: 1, Sequence: 1, Endorsement Plugin: escc, Validation Plugin: vscc
    Approvals: [Org1MSP: true, Org2MSP: true]
    ```

  * You can also specify just the channel name in order to query all chaincode
  definitions on that channel.

    ```
    peer lifecycle chaincode querycommitted --channelID mychannel --peerAddresses peer0.org1.example.com:7051

    Committed chaincode definitions on channel 'mychannel':
    Name: mycc, Version: 1, Sequence: 1, Endorsement Plugin: escc, Validation Plugin: vscc
    Name: yourcc, Version: 2, Sequence: 3, Endorsement Plugin: escc, Validation Plugin: vscc
    ```

  * You can also use the `--output` flag to have the CLI format the output as
    JSON.

    - For querying a specific chaincode definition

      ```
      peer lifecycle chaincode querycommitted --channelID mychannel --name mycc --peerAddresses peer0.org1.example.com:7051 --output json
      ```

      If successful, the command will return a JSON that has committed chaincode definition for chaincode 'mycc' on channel 'mychannel'.

      ```
      {
        "sequence": 1,
        "version": "1",
        "endorsement_plugin": "escc",
        "validation_plugin": "vscc",
        "validation_parameter": "EiAvQ2hhbm5lbC9BcHBsaWNhdGlvbi9FbmRvcnNlbWVudA==",
        "collections": {},
        "init_required": true,
        "approvals": {
          "Org1MSP": true,
          "Org2MSP": true
        }
      }
      ```

      The `validation_parameter` is base64 encoded. An example of the command to decode it is as follows.

      ```
      echo EiAvQ2hhbm5lbC9BcHBsaWNhdGlvbi9FbmRvcnNlbWVudA== | base64 -d

       /Channel/Application/Endorsement
      ```

    - For querying all chaincode definitions on that channel

      ```
      peer lifecycle chaincode querycommitted --channelID mychannel --peerAddresses peer0.org1.example.com:7051 --output json
      ```

      If successful, the command will return a JSON that has committed chaincode definitions on channel 'mychannel'.

      ```
      {
        "chaincode_definitions": [
          {
            "name": "mycc",
            "sequence": 1,
            "version": "1",
            "endorsement_plugin": "escc",
            "validation_plugin": "vscc",
            "validation_parameter": "EiAvQ2hhbm5lbC9BcHBsaWNhdGlvbi9FbmRvcnNlbWVudA==",
            "collections": {},
            "init_required": true
          },
          {
            "name": "yourcc",
            "sequence": 3,
            "version": "2",
            "endorsement_plugin": "escc",
            "validation_plugin": "vscc",
            "validation_parameter": "EiAvQ2hhbm5lbC9BcHBsaWNhdGlvbi9FbmRvcnNlbWVudA==",
            "collections": {}
          }
        ]
      }
      ```


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
