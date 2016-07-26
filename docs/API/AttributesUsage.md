# Attributes usage

## Overview

The Attributes feature allows chaincode to make use of extended data in a transaction certificate. These attributes are certified by the Attributes Certificate Authority (ACA) so the chaincode can trust in the authenticity of the attributes' values.

To view complete documentation about attributes design please read ['Attributes support'](../tech/attributes.md).

## Use case: Authorizable counter

A common use case for the Attributes feature is Attributes Based Access Control (ABAC) which allows specific permissions to be granted to a chaincode invoker based on attribute values carried in the invoker's certificate.

['Authorizable counter'](../../examples/chaincode/go/authorizable_counter/authorizable_counter.go) is a simple example of ABAC, in this case only invokers whose "position" attribute has the value 'Software Engineer' will be able to increment the counter. On the other hand any invoker will be able to read the counter value.

In order to implement this example we used ['VerifyAttribyte' ](https://godoc.org/github.com/hyperledger/fabric/core/chaincode/shim#ChaincodeStub.VerifyAttribute) function to check the attribute value from the chaincode code.

```
isOk, _ := stub.VerifyAttribute("position", []byte("Software Engineer")) // Here the ABAC API is called to verify the attribute, just if the value is verified the counter will be incremented.
if isOk {
    // Increment counter code
}
```

The same behavior can be achieved by making use of ['Attribute support'](https://godoc.org/github.com/hyperledger/fabric/core/chaincode/shim/crypto/attr) API, in this case an attribute handler must be instantiated.

```
attributesHandler, _ := attr.NewAttributesHandlerImpl(stub)
isOk, _ := attributesHandler.VerifyAttribute("position", []byte("Software Engineer"))
if isOk {
    // Increment counter code
}
```
If attributes are accessed more than once, using `attributeHandler` is more efficient since the handler makes use of a cache to store values and keys.

In order to get the attribute value, in place of just verifying it, the following code can be used:

```
attributesHandler, _ := attr.NewAttributesHandlerImpl(stub)
value, _ := attributesHandler.GetValue("position")
```

## Enabling attributes

To make use of this feature the following property has to be set in the membersrvc.yaml file:


- aca.enabled = true

Another way is using environment variables:

```
MEMBERSRVC_CA_ACA_ENABLED=true ./membersrvc
```

## Enabling attributes encryption*

In order to make use of attributes encryption the following property has to be set in the membersrvc.yaml file:

- tca.attribute-encryption.enabled = true

Or using environment variables:

```
MEMBERSRVC_CA_ACA_ENABLED=true MEMBERSRVC_CA_TCA_ATTRIBUTE-ENCRYPTION_ENABLED=true ./membersrvc
```

### Deploy API making use of attributes

#### CLI
```
$ ./peer chaincode deploy --help
Deploy the specified chaincode to the network.

Usage:
  peer chaincode deploy [flags]

Global Flags:
  -a, --attributes="[]": User attributes for the chaincode in JSON format
  -c, --ctor="{}": Constructor message for the chaincode in JSON format
  -l, --lang="golang": Language the chaincode is written in
      --logging-level="": Default logging level and overrides, see core.yaml for full syntax
  -n, --name="": Name of the chaincode returned by the deploy transaction
  -p, --path="": Path to chaincode
      --test.coverprofile="coverage.cov": Done
  -t, --tid="": Name of a custom ID generation algorithm (hashing and decoding) e.g. sha256base64
  -u, --username="": Username for chaincode operations when security is enabled
  -v, --version[=false]: Display current version of fabric peer server
```
To deploy a chaincode with attributes "company" and "position" it should be written in the following way:

```
./peer chaincode deploy -u userName -n mycc -c '{"Function":"init", "Args": []}' -a '["position", "company"]'

```

#### REST

```
POST host:port/chaincode

{
  "jsonrpc": "2.0",
  "method": "deploy",
  "params": {
    "type": 1,
    "chaincodeID":{
        "name": "mycc"
    },
    "ctorMsg": {
        "function":"init",
        "args":[]
    }
    "attributes": ["position", "company"]
  },
  "id": 1
}
```

### Invoke API making use of attributes

#### CLI
```
$ ./peer chaincode invoke --help
Invoke the specified chaincode.

Usage:
  peer chaincode invoke [flags]

Global Flags:
  -a, --attributes="[]": User attributes for the chaincode in JSON format
  -c, --ctor="{}": Constructor message for the chaincode in JSON format
  -l, --lang="golang": Language the chaincode is written in
      --logging-level="": Default logging level and overrides, see core.yaml for full syntax
  -n, --name="": Name of the chaincode returned by the deploy transaction
  -p, --path="": Path to chaincode
      --test.coverprofile="coverage.cov": Done
  -t, --tid="": Name of a custom ID generation algorithm (hashing and decoding) e.g. sha256base64
  -u, --username="": Username for chaincode operations when security is enabled
  -v, --version[=false]: Display current version of fabric peer server
```
To invoke "autorizable counter" with attributes "company" and "position" it should be written as follows:

```
./peer chaincode invoke -u userName -n mycc -c '{"Function":"increment", "Args": []}' -a '["position", "company"]'

```

#### REST

```
POST host:port/chaincode

{
  "jsonrpc": "2.0",
  "method": "invoke",
  "params": {
    "type": 1,
    "chaincodeID":{
        "name": "mycc"
    },
    "ctorMsg": {
        "function":"increment",
        "args":[]
    }
    "attributes": ["position", "company"]
  },
  "id": 1
}
```

### Query API making use of attributes

#### CLI
```
$ ./peer chaincode query --help
Query using the specified chaincode.

Usage:
  peer chaincode query [flags]

Flags:
  -x, --hex[=false]: If true, output the query value byte array in hexadecimal. Incompatible with --raw
  -r, --raw[=false]: If true, output the query value as raw bytes, otherwise format as a printable string


Global Flags:
  -a, --attributes="[]": User attributes for the chaincode in JSON format
  -c, --ctor="{}": Constructor message for the chaincode in JSON format
  -l, --lang="golang": Language the chaincode is written in
      --logging-level="": Default logging level and overrides, see core.yaml for full syntax
  -n, --name="": Name of the chaincode returned by the deploy transaction
  -p, --path="": Path to chaincode
      --test.coverprofile="coverage.cov": Done
  -t, --tid="": Name of a custom ID generation algorithm (hashing and decoding) e.g. sha256base64
  -u, --username="": Username for chaincode operations when security is enabled
  -v, --version[=false]: Display current version of fabric peer server
```
To query "autorizable counter" with attributes "company" and "position" it should be written in this way:

```
./peer chaincode query -u userName -n mycc -c '{"Function":"read", "Args": []}' -a '["position", "company"]'

```

#### REST

```
POST host:port/chaincode

{
  "jsonrpc": "2.0",
  "method": "query",
  "params": {
    "type": 1,
    "chaincodeID":{
        "name": "mycc"
    },
    "ctorMsg": {
        "function":"read",
        "args":[]
    }
    "attributes": ["position", "company"]
  },
  "id": 1
}
```
* Attributes encryption is not yet available.
