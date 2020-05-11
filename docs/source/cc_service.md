# Chaincode as an external service

Fabric v2.0 supports chaincode deployment and execution outside of Fabric that enables users to manage a chaincode runtime independently of the peer. This facilitates deployment of chaincode on Fabric cloud deployments such as Kubernetes. Instead of building and launching the chaincode on every peer, chaincode can now run as a service whose lifecycle is managed outside of Fabric. This capability leverages the Fabric v2.0 external builder and launcher functionality which enables operators to extend a peer with programs to build, launch, and discover chaincode. Before reading this topic you should become familiar with the [External Builder and Launcher](./cc_launcher.html) content.

Prior to the availability of the external builders, the chaincode package content was required to be a set of source code files for a particular language which could be built and launched as a chaincode binary. The new external build and launcher functionality now allows users to optionally customize the build process. With respect to running the chaincode as an external service, the build process allows you to specify the endpoint information of the server where the chaincode is running. Hence the package simply consists of the externally running chaincode server endpoint information and TLS artifacts for secure connection. TLS is optional but highly recommended for all environments except a simple test environment.

The rest of this topic describes how to configure chaincode as an external service:

* [Packaging chaincode](#packaging-chaincode)
* [Configuring a peer to process external chaincode](#configuring-a-peer-to-process-external-chaincode)
* [External builder and launcher sample scripts](#external-builder-and-launcher-sample-scripts)
* [Writing chaincode to run as an external service](#writing-chaincode-to-run-as-an-external-service)
* [Deploying the chaincode](#deploying-the-chaincode)
* [Running the chaincode as an external service](#running-the-chaincode-as-an-external-service)

**Note:** This is an advanced feature that will likely require custom packaging of the peer image. For example, the following samples use `jq` and `bash`, which are not included in the current official `fabric-peer` image.

## Packaging chaincode

With the Fabric v2.0 chaincode lifecycle, chaincode is [packaged](./cc_launcher.html#chaincode-packages) and installed in a `.tar.gz` format. The following `myccpackage.tgz` archive  demonstrates the required structure:

```sh
$ tar xvfz myccpackage.tgz
metadata.json
code.tar.gz
```

The chaincode package should be used to provide two pieces of information to the external builder and launcher process
* identify if the chaincode is an external service. The `bin/detect` section describes an approach using the `metadata.json` file
* provide chaincode endpoint information in a `connection.json` file placed in the release directory. The `bin/run` section describes the `connection.json` file

There is plenty of flexibility to gathering the above information. The sample scripts in the [External builder and launcher sample scripts](#external-builder-and-launcher-sample-scripts) illustrate a simple approach to providing the information.
As an example of flexibility, consider packaging couchdb index files (see [Add the index to your chaincode folder](couchdb_tutorial.html#add-the-index-to-your-chaincode-folder)). Sample scripts below describe an approach to packaging the files into code.tar.gz.

```
tar cfz code.tar.gz connection.json metadata
tar cfz $1-pkg.tgz metadata.json code.tar.gz
```

## Configuring a peer to process external chaincode

In this section we go over the configuration needed
* to detect if the chaincode package identifies an external chaincode service
* to create the `connection.json` file in the release directory

### Modify the peer core.yaml to include the externalBuilder

Assume the scripts are on the peer in the `bin` directory as follows
```
    <fully qualified path on the peer's env>
    └── bin
        ├── build
        ├── detect
        └── release
```

Modify the `chaincode` stanza of the peer `core.yaml` file to include the `externalBuilders` configuration element:

```yaml
externalBuilders:
     - name: myexternal
       path: <fully qualified path on the peer's env>   
```

### External builder and launcher sample scripts

To help understand what each script needs to contain to work with the chaincode as an external service, this section contains samples of  `bin/detect` `bin/build`, `bin/release`, and `bin/run` scripts.

**Note:** These samples use the `jq` command to parse json. You can run `jq --version` to check if you have it installed. Otherwise, install `jq` or suitably modify the scripts.

#### bin/detect

The `bin/detect script` is responsible for determining whether or not a buildpack should be used to build a chaincode package and launch it.  For chaincode as an external service, the sample script looks for a `type` property set to `external` in the `metadata.json` file:

```json
{"path":"","type":"external","label":"mycc"}
```

The peer invokes detect with two arguments:

```
bin/detect CHAINCODE_SOURCE_DIR CHAINCODE_METADATA_DIR
```

A sample `bin/detect` script could contain:

```sh

#!/bin/bash

set -euo pipefail

METADIR=$2
#check if the "type" field is set to "external"
if [ "$(jq -r .type "$METADIR/metadata.json")" == "external" ]; then
    exit 0
fi

exit 1

```

#### bin/build

For chaincode as an external service, the sample build script assumes the chaincode package's `code.tar.gz` file contains `connection.json` which it simply copies to the `BUILD_OUTPUT_DIR`. The peer invokes the build script with three arguments:

```
bin/build CHAINCODE_SOURCE_DIR CHAINCODE_METADATA_DIR BUILD_OUTPUT_DIR
```

A sample `bin/build` script could contain:

```sh

#!/bin/bash

set -euo pipefail

SOURCE=$1
OUTPUT=$3

#external chaincodes expect connection.json file in the chaincode package
if [ ! -f "$SOURCE/connection.json" ]; then
    >&2 echo "$SOURCE/connection.json not found"
    exit 1
fi

#simply copy the endpoint information to specified output location
cp $SOURCE/connection.json $OUTPUT/connection.json

if [ -d "$SOURCE/metadata" ]; then
    cp -a $SOURCE/metadata $OUTPUT/metadata
fi

exit 0

```

#### bin/release

For chaincode as an external service, the `bin/release` script is responsible for providing the `connection.json` to the peer by placing it in the `RELEASE_OUTPUT_DIR`.  The `connection.json` file has the following JSON structure

* **address** - chaincode server endpoint accessible from peer. Must be specified in “<host>:<port>” format.
* **dial_timeout** - interval to wait for connection to complete. Specified as a string qualified with time units (e.g, "10s", "500ms", "1m"). Default is “3s” if not specified.
* **tls_required** - true or false. If false, "client_auth_required", "client_key", "client_cert", and "root_cert" are not required. Default is “true”.
* **client_auth_required** - if true, "client_key" and "client_cert" are required. Default is false. It is ignored if tls_required is false.
* **client_key** - PEM encoded string of the client private key.
* **client_cert**  - PEM encoded string of the client certificate.
* **root_cert**  - PEM encoded string of the server (peer) root certificate.

For example:

```json
{
  "address": "your.chaincode.host.com:9999",
  "dial_timeout": "10s",
  "tls_required": "true",
  "client_auth_required": "true",
  "client_key": "-----BEGIN EC PRIVATE KEY----- ... -----END EC PRIVATE KEY-----",
  "client_cert": "-----BEGIN CERTIFICATE----- ... -----END CERTIFICATE-----",
  "root_cert": "-----BEGIN CERTIFICATE---- ... -----END CERTIFICATE-----"
}
```

As noted in the `bin/build` section, this sample assumes the chaincode package directly contains the `connection.json` file which the build script copies to the `BUILD_OUTPUT_DIR`. The peer invokes the release script with two arguments:

```
bin/release BUILD_OUTPUT_DIR RELEASE_OUTPUT_DIR
```

A sample `bin/release` script could contain:


```sh

#!/bin/bash

set -euo pipefail

BLD="$1"
RELEASE="$2"

if [ -d "$BLD/metadata" ]; then
   cp -a "$BLD/metadata/"* "$RELEASE/"
fi

#external chaincodes expect artifacts to be placed under "$RELEASE"/chaincode/server
if [ -f $BLD/connection.json ]; then
   mkdir -p "$RELEASE"/chaincode/server
   cp $BLD/connection.json "$RELEASE"/chaincode/server

   #if tls_required is true, copy TLS files (using above example, the fully qualified path for these fils would be "$RELEASE"/chaincode/server/tls)

   exit 0
fi

exit 1
```    

## Writing chaincode to run as an external service

Currently, the chaincode as an external service model is only supported by GO chaincode shim. In Fabric v2.0, the GO shim API adds a `ChaincodeServer` type that developers should use to create a chaincode server.  The `Invoke` and `Query` APIs are unaffected. Developers should write to the `shim.ChaincodeServer` API, then build the chaincode and run it in the external environment of choice. Here is a simple sample chaincode program to illustrate the pattern:

```go

package main

import (
        "fmt"

        "github.com/hyperledger/fabric-chaincode-go/shim"
        pb "github.com/hyperledger/fabric-protos-go/peer"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

func (s *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
        // init code
}

func (s *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
        // invoke code
}

//NOTE - parameters such as ccid and endpoint information are hard coded here for illustration. This can be passed in in a variety of standard ways
func main() {
       //The ccid is assigned to the chaincode on install (using the “peer lifecycle chaincode install <package>” command) for instance
        ccid := "mycc:fcbf8724572d42e859a7dd9a7cd8e2efb84058292017df6e3d89178b64e6c831"

        server := &shim.ChaincodeServer{
                        CCID: ccid,
                        Address: "myhost:9999"
                        CC: new(SimpleChaincode),
                        TLSProps: shim.TLSProperties{
                                Disabled: true,
                        },
                }
        err := server.Start()
        if err != nil {
                fmt.Printf("Error starting Simple chaincode: %s", err)
        }
}
```
The key to running the chaincode as an external service is the use of `shim.ChaincodeServer`. This uses the new shim API `shim.ChaincodeServer` with the chaincode service properties described below:

* **CCID** (string)- CCID should match chaincode's package name on peer. This is the `CCID` associated with the installed chaincode as returned by the `peer lifecycle chaincode install <package>` CLI command. This can be obtained post-install using the "peer lifecycle chaincode queryinstalled" command.
* **Address** (string) - Address is the listen address of the chaincode server
* **CC** (Chaincode) -  CC is the chaincode that handles Init and Invoke
* **TLSProps** (TLSProperties) - TLSProps is the TLS properties passed to chaincode server
* **KaOpts** (keepalive.ServerParameters) -  KaOpts keepalive options, sensible defaults provided if nil

Then build the chaincode as suitable to your GO environment.

## Deploying the chaincode

When the GO chaincode is ready for deployment, you can package the chaincode as explained in the [Packaging chaincode](#packaging-chaincode) section and deploy the chaincode as explained in the [Fabric chaincode lifecycle](./chaincode_lifecycle.html) concept topic.

## Running the chaincode as an external service

Create the chaincode as specified in the [Writing chaincode to run as an external service](#writing-chaincode-to-run-as-an-external-service) section. Run the built executable in your environment of choice, such as Kubernetes or directly as a process on the peer machine.

Using this chaincode as an external service model, installing the chaincode on each peer is no longer required. With the chaincode endpoint deployed to the peer instead and the chaincode running, you can continue the normal process of committing the
chaincode definition to the channel and invoking the chaincode.

<!---
Licensed under Creative Commons Attribution 4.0 International License https://creativecommons.org/licenses/by/4.0/
-->
