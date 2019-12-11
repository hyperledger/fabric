# Chaincode as an external service

Fabric v2.0 supports chaincode deployment and execution outside of Fabric that enables users to manage a chaincode runtime independently of the peer. This facilitates deployment of chaincode on Fabric cloud deployments such as Kubernetes. Instead of building and launching the chaincode on every peer, chaincode can now run as a service whose lifecycle is managed outside of Fabric. This capability leverages the Fabric v2.0 external builder and launcher functionality which enables operators to extend a peer with programs to build, launch, and discover chaincode. Before reading this topic you should become familiar with the [External Builder and Launcher](./cc_launcher.html) content.

Prior to the availability of the external builders, the chaincode package content was required to be a set of source code files for a particular language which could be built and launched as a chaincode binary. The new external build and launcher functionality now allows users to optionally customize the build process. And with respect to running the chaincode as an external service, the build process allows you to specify the endpoint information of the server where the chaincode is running. Hence the package simply consists of the externally running chaincode server endpoint information and TLS artifacts for secure connection. TLS is optional but highly recommended for all environments except a simple test environment.

The rest of this topic describes how to configure chaincode as an external service:

* [Packaging chaincode](#packaging-chaincode)
* [Configuring a peer to process external chaincode](#configuring-a-peer-to-process-external-chaincode)
* [External builder and launcher sample scripts](#external-builder-and-launcher-sample-scripts)
* [Writing chaincode to run as an external service](#writing-chaincode-to-run-as-an-external-service)
* [Deploying the chaincode](#deploying-the-chaincode)
* [Running the chaincode as an external service](#running-the-chaincode-as-an-external-service)

## Packaging chaincode

With the Fabric v2.0 chaincode lifecycle, chaincode is [packaged](./cc_launcher.html#chaincode-packages) and installed in a `.tar.gz` format. The following `myccpackage.tgz` archive  demonstrates the required structure:

```sh
$ tar xvfz myccpackage.tgz
code.tar.gz
metadata.json
```

### code.tar.gz archive requirements

The `code.tar.gz` archive must include connection information for the chaincode endpoint. This information is packaged into `connection.json` in the `/bin/release` step (see below). In this example we package `connection.json` directly in `code.tar.gz` so the `release` step can just copy it over.

* **address** - chaincode server endpoint accessible from peer. Must be specified in “<host>:<port>” format.
* **dial_timeout** - interval to wait for connection to complete. Specified as a string qualified with units (examples : "10s", "500ms", "1m"). Default is “3s” if not specified.
* **tls_required** - true or false. If false "client_auth_required", "key_path", "cert_path", "root_cert_path" are not required. Default is “true”.
* **client_auth_required** - if true then you need to specify "key_path" and "cert_path" for client authentication. Default is false. Ignored if tls_required is false.
* **key_path** - path to the key file. This path is relative to the “release directory” (see “release” below). Required if client_auth_required is true. Ignored if tls_required is false.
* **cert_path**  - path to the certificate file. This path is relative to the “release directory” (see “release” below). Required if client_auth_required is true. Ignored if tls_required is false.
* **root_cert_path**  - path to the root cert for authenticating the server. Required when tls_required is set to true.

For example:
```json
{
  "address": "your.chaincode.host.com:9999",
  "dial_timeout": "10s",
  "tls_required": true,
  "client_auth_required": "true",
  "key_path": "path/rooted/in/release/directory/key.pem",
  "cert_path": "path/rooted/in/release/directory/cert.pem",
  "root_cert_path": "path/rooted/in/release/directory/rootcert.pem"
}
```

**Note:** The TLS files can be placed anywhere in the `code.tar.gz` archive because the directory of the contents of the `.tar.gz` will be provided to the external chaincode builder scripts. The `/bin/release` script, described later in this document, should move the files to the appropriate paths relative to the release directory. In the future, the PEM may be included in the JSON itself.

### metadata.json requirements

When using chaincode as an external service, you can set the `type` field in `metadata.json` to `external` in order to indicate that the external service is being used. For example:

```json
{"path":"","type":"external","label":"mycc"}
```

## Configuring a peer to process external chaincode

If you have reviewed the external build and chaincode launcher documentation then these steps will be familiar to you. We leverage those scripts to define the external chaincode service information. These scripts are located on the peer file system that is accessible to the peer process from the `core.yaml` file in the `externalBuilders` element under the `chaincode` stanza. An example is included in the steps which follow.

### Create the set of external builder and launcher scripts on the peer

In order to configure chaincode as an external service, you must use the scripts as follows:
* **detect** - examines the chaincode package and accept if metadata.json `type` is set to `external`.
* **build** - builds the chaincode and places the build artifacts in the `BUILD_OUTPUT_DIR`. For chaincode as an external service, the peer does not build the chaincode. Instead, the script extracts the chaincode endpoint information in the `connection.json` file and other artifacts from the `code.tar.gz` and places them in the specified location.
* **release** - copies the built artifacts (in our case the `connection.json` file) to a specified release location.

You may notice that the external builder and launcher `bin/run` script is not required for the chaincode as an external service.

The scripts that are required to exist in the peer `/bin` directory:
```
    <fully qualified path on the peer's env>
    └── bin
        ├── build
        ├── detect
        └── release
```

### Modify the peer core.yaml to include the externalBuilder

Finally, in order for the peer to use the external builder and launcher scripts, you need to modify the `chaincode` stanza of the peer `core.yaml` file to include the `externalBuilders` configuration element.

```yaml
externalBuilders:
     - name: myexternal
       path: <fully qualified path on the peer's env>   
```

## External builder and launcher sample scripts

To help understand what each script needs to contain to work with the chaincode as an external service, this section contains samples of  `bin/detect` `bin/build`, `/bin/release`, and `bin/run` scripts.

**Note:** These samples use the `jq` command to parse json. You can run `jq --version` to check if you have it installed. Otherwise, install `jq` or suitably modify the scripts yourself.

### /bin/detect

The `bin/detect script` is responsible for determining whether or not a buildpack should be used to build a chaincode package and launch it. For chaincode as an external service, the script should detect that metadata.json `type` is set to `external`.  The peer invokes detect with two arguments:

```
bin/detect CHAINCODE_SOURCE_DIR CHAINCODE_METADATA_DIR
```

A typical `detect` script could contain:

```sh

#!/bin/bash

set -euo pipefail

if [ "$#" -ne 2 ]; then
    >&2 echo "Expected 2 directories got $#"
    exit 2
fi

#check if the "type" field is set to "external"
if [ "$(jq -r .type "$2/metadata.json")" == "external" ]; then
    exit 0
fi

exit 1
```
Recall that the metadata.json file should contain the following keys:

```json
{"path":"","type":"external","label":"mycc"}
```

### /bin/build

The `bin/build` script is responsible for building, compiling, or transforming the contents of a chaincode package into artifacts that can be used by release and run. For chaincode as an external service, the build script copies the `connection.json` to the `BUILD_OUTPUT_DIR`. The peer invokes the build script with three arguments:

```
bin/build CHAINCODE_SOURCE_DIR CHAINCODE_METADATA_DIR BUILD_OUTPUT_DIR
```

A typical `build` script could contain:

```sh

#!/bin/bash

set -euo pipefail

if [ "$#" -ne 3 ]; then
    >&2 echo "Expected 3 directories got $#"
    exit 1
fi

SOURCE=$1
OUTPUT=$3

#external chaincodes expect connection.json file in the chaincode package
if [ ! -f "$SOURCE/connection.json" ]  ; then
    >&2 echo "$SOURCE/connection.json not found"
    exit 1
fi

#do more validation here if needed

#simply copy the endpoint information to specified output location
cp $SOURCE/connection.json $OUTPUT/connection.json

exit 0

```

### /bin/release

The `bin/release script` is responsible for providing metadata chaincode to the peer. For chaincode as an external service, the `bin/release` script is responsible for providing the connection.json to the peer by placing it in the `RELEASE_OUTPUT_DIR`. The peer invokes the release script with two arguments:

```
bin/release BUILD_OUTPUT_DIR RELEASE_OUTPUT_DIR
```

A typical `release` script could contain:

```sh

#!/bin/bash

set -euo pipefail

set -x

if [ "$#" -ne 2 ]; then
    >&2 echo "Expected 2 directories got $#"
    exit 2
fi

BLD="$1"
RELEASE="$2"

#external chaincodes expect artifacts to be placed under "$RELEASE"/chaincode/server
if [ -f $BLD/connection.json ]; then
   mkdir -p "$RELEASE"/chaincode/server
   cp $BLD/connection.json "$RELEASE"/chaincode/server
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

When the GO chaincode is ready for deployment, you can package the chaincode as explained in the [Packaging chaincode](#packaging-chaincode) section and deploy the chaincode as explained in the [chaincode lifecycle](./chaincode4noah.html#chaincode-lifecycle) documentation.

## Running the chaincode as an external service

Create the chaincode as specified in the [Writing chaincode to run as an external service](writing-chaincode-to-run-as-an-external-service) section. Run the built executable in your environment of choice, such as Kubernetes or directly as a process on the peer machine.

Using this chaincode as an external service model, installing chaincode on each peer is no longer required. With the chaincode endpoint deployed to the peer instead and the chaincode running, you can continue to instantiate and invoke chaincode normally.

<!---
Licensed under Creative Commons Attribution 4.0 International License https://creativecommons.org/licenses/by/4.0/
-->
