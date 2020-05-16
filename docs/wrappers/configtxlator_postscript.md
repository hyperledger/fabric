## Examples

### Decoding

Decode a block named `fabric_block.pb` to JSON and print to stdout.

```
configtxlator proto_decode --input fabric_block.pb --type common.Block
```

Alternatively, after starting the REST server, the following curl command
performs the same operation through the REST API.

```
curl -X POST --data-binary @fabric_block.pb "${CONFIGTXLATOR_URL}/protolator/decode/common.Block"
```

### Encoding

Convert a JSON document for a policy from stdin to a file named `policy.pb`.

```
configtxlator proto_encode --type common.Policy --output policy.pb
```

Alternatively, after starting the REST server, the following curl command
performs the same operation through the REST API.

```
curl -X POST --data-binary /dev/stdin "${CONFIGTXLATOR_URL}/protolator/encode/common.Policy" > policy.pb
```

### Pipelines

Compute a config update from `original_config.pb` and `modified_config.pb` and decode it to JSON to stdout.

```
configtxlator compute_update --channel_id testchan --original original_config.pb --updated modified_config.pb | configtxlator proto_decode --type common.ConfigUpdate
```

Alternatively, after starting the REST server, the following curl commands
perform the same operations through the REST API.

```
curl -X POST -F channel=testchan -F "original=@original_config.pb" -F "updated=@modified_config.pb" "${CONFIGTXLATOR_URL}/configtxlator/compute/update-from-configs" | curl -X POST --data-binary /dev/stdin "${CONFIGTXLATOR_URL}/protolator/decode/common.ConfigUpdate"
```

## Additional Notes

The tool name is a portmanteau of *configtx* and *translator* and is intended to
convey that the tool simply converts between different equivalent data
representations. It does not generate configuration. It does not submit or
retrieve configuration. It does not modify configuration itself, it simply
provides some bijective operations between different views of the configtx
format.

There is no configuration file `configtxlator` nor any authentication or
authorization facilities included for the REST server.  Because `configtxlator`
does not have any access to data, key material, or other information which
might be considered sensitive, there is no risk to the owner of the server in
exposing it to other clients.  However, because the data sent by a user to
the REST server might be confidential, the user should either trust the
administrator of the server, run a local instance, or operate via the CLI.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
