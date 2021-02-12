## Example Usage

### osnadmin channel join examples

Here's an example of the `osnadmin channel join` command.

* Create and join a sample channel `mychannel` defined by the application channel genesis
  block contained in file `mychannel-genesis-block.pb`. Use the orderer admin endpoint
  at `orderer.example.com:9443`.

  ```

  osnadmin channel join -o orderer.example.com:9443 --ca-file $CA_FILE --client-cert $CLIENT_CERT --client-key $CLIENT_KEY --channelID mychannel --config-block mychannel-genesis-block.pb

  Status: 201
  {
    "name": "mychannel",
    "url": "/participation/v1/channels/mychannel",
    "consensusRelation": "consenter",
    "status": "active",
    "height": 1
  }

  ```

  Status 201 and the channel details are returned indicating that the channel has been
  successfully created and joined.

### osnadmin channel list example

Here are some examples of the `osnadmin channel list` command.

* Listing all the channels that the orderer has joined. This includes the
  system channel (if one exists) and all of the application channels.

  ```
  osnadmin channel list -o orderer.example.com:9443 --ca-file $CA_FILE --client-cert $CLIENT_CERT --client-key $CLIENT_KEY

  Status: 200
  {
    "systemChannel": null,
    "channels": [
        {
            "name": "mychannel",
            "url": "/participation/v1/channels/mychannel"
        }
    ]
  }

  ```

  Status 200 and the list of channels are returned.

* Using the `--channelID` flag to list more details for `mychannel`.

  ```
  osnadmin channel list -o orderer.example.com:9443 --ca-file $CA_FILE --client-cert $CLIENT_CERT --client-key $CLIENT_KEY --channelID mychannel

  Status: 200
  {
	"name": "mychannel",
	"url": "/participation/v1/channels/mychannel",
	"consensusRelation": "consenter",
	"status": "active",
	"height": 3
  }

  ```

  Status 200 and the details of the channels are returned.

### osnadmin channel remove example

Here's an example of the `osnadmin channel remove` command.

* Removing channel `mychannel` from the orderer at `orderer.example.com:9443`.

  ```
  osnadmin channel remove -o orderer.example.com:9443 --ca-file $CA_FILE --client-cert $CLIENT_CERT --client-key $CLIENT_KEY --channelID mychannel

  Status: 204
  ```

  Status 204 is returned upon successful removal of a channel.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
