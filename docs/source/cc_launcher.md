# External Builders and Launchers

Prior to Hyperledger Fabric 2.0, the process used to build and launch chaincode was part of the peer implementation and could not be easily customized. All chaincode installed on the peer would be "built" using language specific logic hard coded in the peer. This build process would generate a Docker container image that would be launched to execute chaincode that connected as a client to the peer.

This approach limited chaincode implementations to a handful of languages, required Docker to be part of the deployment environment, and prevented running chaincode as a long running server process.

Starting with Fabric 2.0, External Builders and Launchers address these limitations by enabling operators to extend the peer with programs that can build, launch, and discover chaincode. To leverage this capability you will need to create your own buildpack and then modify the peer core.yaml to include a new `externalBuilder` configuration element which lets the peer know an external builder is available. The following sections describe the details of this process.

Note that if no configured external builder claims a chaincode package, the peer will attempt to process the package as if it were created with the standard Fabric packaging tools such as the peer CLI or node SDK.

## External builder model

Hyperledger Fabric External Builders and Launchers are loosely based on Heroku [Buildpacks](https://devcenter.heroku.com/articles/buildpack-api). A buildpack implementation is simply a collection of programs or scripts that transform application artifacts into something that can run. The buildpack model has been adapted for chaincode packages and extended to support chaincode execution and discovery.

### External builder and launcher API

An external builder and launcher consists of four programs or scripts:

- `bin/detect`: Determine whether or not this buildpack should be used to build the chaincode package and launch it.
- `bin/build`: Transform the chaincode package into executable chaincode.
- `bin/release` (optional): Provide metadata to the peer about the chaincode.
- `bin/run` (optional): Run the chaincode.

#### `bin/detect`

The `bin/detect` script is responsible for determining whether or not a buildpack should be used to build a chaincode package and launch it. The peer invokes `detect` with two arguments:

```sh
bin/detect CHAINCODE_SOURCE_DIR CHAINCODE_METADATA_DIR
```

When `detect` is invoked, `CHAINCODE_SOURCE_DIR` contains the chaincode source and `CHAINCODE_METADATA_DIR` contains the `metadata.json` file from the chaincode package installed to the peer. The `CHAINCODE_SOURCE_DIR` and `CHAINCODE_METADATA_DIR` should be treated as read only inputs. If the buildpack should be applied to the chaincode source package, `detect` must return an exit code of `0`; any other exit code will indicate that the buildpack should not be applied.

The following is an example of a simple `detect` script for go chaincode:
```sh
#!/bin/bash

CHAINCODE_METADATA_DIR="$2"

# use jq to extract the chaincode type from metadata.json and exit with
# success if the chaincode type is golang
if [ "$(jq -r .type "$CHAINCODE_METADATA_DIR/metadata.json" | tr '[:upper:]' '[:lower:]')" = "golang" ]; then
    exit 0
fi

exit 1
```

#### `bin/build`

The `bin/build` script is responsible for building, compiling, or transforming the contents of a chaincode package into artifacts that can be used by `release` and `run`. The peer invokes `build` with three arguments:

```sh
bin/build CHAINCODE_SOURCE_DIR CHAINCODE_METADATA_DIR BUILD_OUTPUT_DIR
```

When `build` is invoked, `CHAINCODE_SOURCE_DIR` contains the chaincode source and `CHAINCODE_METADATA_DIR` contains the `metadata.json` file from the chaincode package installed to the peer. `BUILD_OUTPUT_DIR` is the directory where `build` must place artifacts needed by `release` and `run`. The build script should treat the input directories `CHAINCODE_SOURCE_DIR` and `CHAINCODE_METADATA_DIR` as read only, but the `BUILD_OUTPUT_DIR` is writeable.

When `build` completes with an exit code of `0`, the contents of `BUILD_OUTPUT_DIR` will be copied to the persistent storage maintained by the peer; any other exit code will be considered a failure.

The following is an example of a simple `build` script for go chaincode:

```sh
#!/bin/bash

CHAINCODE_SOURCE_DIR="$1"
CHAINCODE_METADATA_DIR="$2"
BUILD_OUTPUT_DIR="$3"

# extract package path from metadata.json
GO_PACKAGE_PATH="$(jq -r .path "$CHAINCODE_METADATA_DIR/metadata.json")"
if [ -f "$CHAINCODE_SOURCE_DIR/src/go.mod" ]; then
    cd "$CHAINCODE_SOURCE_DIR/src"
    go build -v -mod=readonly -o "$BUILD_OUTPUT_DIR/chaincode" "$GO_PACKAGE_PATH"
else
    GO111MODULE=off go build -v  -o "$BUILD_OUTPUT_DIR/chaincode" "$GO_PACKAGE_PATH"
fi

# save statedb index metadata to provide at release
if [ -d "$CHAINCODE_SOURCE_DIR/META-INF" ]; then
    cp -a "$CHAINCODE_SOURCE_DIR/META-INF" "$BUILD_OUTPUT_DIR/"
fi
```

#### `bin/release`

The `bin/release` script is responsible for providing chaincode metadata to the peer. `bin/release` is optional. If it is not provided, this step is skipped. The peer invokes `release` with two arguments:

```sh
bin/release BUILD_OUTPUT_DIR RELEASE_OUTPUT_DIR
```

When `release` is invoked, `BUILD_OUTPUT_DIR` contains the artifacts populated by the `build` program and should be treated as read only input. `RELEASE_OUTPUT_DIR` is the directory where `release` must place artifacts to be consumed by the peer.

When `release` completes, the peer will consume two types of metadata from `RELEASE_OUTPUT_DIR`:

- state database index definitions for CouchDB
- external chaincode server connection information (`chaincode/server/connection.json`)

If CouchDB index definitions are required for the chaincode, `release` is responsible for placing the indexes into the `statedb/couchdb/indexes` directory under `RELEASE_OUTPUT_DIR`. The indexes must have a `.json` extension.  See the [CouchDB indexes](couchdb_as_state_database.html#couchdb-indexes) documentation for details.

In cases where a chaincode server implementation is used, `release` is responsible for populating `chaincode/server/connection.json` with the address of the chaincode server and any TLS assets required to communicate with the chaincode. When server connection information is provided to the peer, `run` will not be called. See the [Chaincode Server](https://jira.hyperledger.org/browse/FAB-14086) documentation for details.

The following is an example of a simple `release` script for go chaincode:

```sh
#!/bin/bash

BUILD_OUTPUT_DIR="$1"
RELEASE_OUTPUT_DIR="$2"

# copy indexes from META-INF/* to the output directory
if [ -d "$BUILD_OUTPUT_DIR/META-INF" ] ; then
   cp -a "$BUILD_OUTPUT_DIR/META-INF/"* "$RELEASE_OUTPUT_DIR/"
fi
```

#### `bin/run`

The `bin/run` script is responsible for running chaincode. The peer invokes `run` with two arguments:

```sh
bin/run BUILD_OUTPUT_DIR RUN_METADATA_DIR
```

When `run` is called, `BUILD_OUTPUT_DIR` contains the artifacts populated by the `build` program and `RUN_METADATA_DIR` is populated with a file called `chaincode.json` that contains the information necessary for chaincode to connect and register with the peer. Note that the `bin/run` script should treat these `BUILD_OUTPUT_DIR` and `RUN_METADATA_DIR` directories as read only input.  The keys included in `chaincode.json` are:

- `chaincode_id`: The unique ID associated with the chaincode package.
- `peer_address`: The address in `host:port` format of the `ChaincodeSupport` gRPC server endpoint hosted by the peer.
- `client_cert`: The PEM encoded TLS client certificate generated by the peer that must be used when the chaincode establishes its connection to the peer.
- `client_key`: The PEM encoded client key generated by the peer that must be used when the chaincode establishes its connection to the peer.
- `root_cert`: The PEM encoded TLS root certificate for the `ChaincodeSupport` gRPC server endpoint hosted by the peer.
- `mspid`: The local mspid of the peer.

When `run` terminates, the peer considers the chaincode terminated. If another request arrives for the chaincode, the peer will attempt to start another instance of the chaincode by invoking `run` again. The contents of `chaincode.json` must not be cached across invocations.

The following is an example of a simple `run` script for go chaincode:
```sh
#!/bin/bash

BUILD_OUTPUT_DIR="$1"
RUN_METADATA_DIR="$2"

# setup the environment expected by the go chaincode shim
export CORE_CHAINCODE_ID_NAME="$(jq -r .chaincode_id "$RUN_METADATA_DIR/chaincode.json")"
export CORE_PEER_TLS_ENABLED="true"
export CORE_TLS_CLIENT_CERT_FILE="$RUN_METADATA_DIR/client.crt"
export CORE_TLS_CLIENT_KEY_FILE="$RUN_METADATA_DIR/client.key"
export CORE_PEER_TLS_ROOTCERT_FILE="$RUN_METADATA_DIR/root.crt"
export CORE_PEER_LOCALMSPID="$(jq -r .mspid "$RUN_METADATA_DIR/chaincode.json")"

# populate the key and certificate material used by the go chaincode shim
jq -r .client_cert "$RUN_METADATA_DIR/chaincode.json" > "$CORE_TLS_CLIENT_CERT_FILE"
jq -r .client_key  "$RUN_METADATA_DIR/chaincode.json" > "$CORE_TLS_CLIENT_KEY_FILE"
jq -r .root_cert   "$RUN_METADATA_DIR/chaincode.json" > "$CORE_PEER_TLS_ROOTCERT_FILE"
if [ -z "$(jq -r .client_cert "$RUN_METADATA_DIR/chaincode.json")" ]; then
    export CORE_PEER_TLS_ENABLED="false"
fi

# exec the chaincode to replace the script with the chaincode process
exec "$BUILD_OUTPUT_DIR/chaincode" -peer.address="$(jq -r .peer_address "$ARTIFACTS/chaincode.json")"
```

## Configuring external builders and launchers

Configuring the peer to use external builders involves adding an externalBuilder element under the chaincode configuration block in the  `core.yaml` that defines external builders. Each external builder definition must include a name (used for logging) and the path to parent of the `bin` directory containing the builder scripts.

An optional list of environment variable names to propagate from the peer when invoking the external builder scripts can also be provided.

The following example defines two external builders:

```yaml
chaincode:
  externalBuilders:
  - name: my-golang-builder
    path: /builders/golang
    environmentWhitelist:
    - GOPROXY
    - GONOPROXY
    - GOSUMDB
    - GONOSUMDB
  - name: noop-builder
    path: /builders/binary
```

In this example, the implementation of "my-golang-builder" is contained within the `/builders/golang` directory and its build scripts are located in `/builders/golang/bin`. When the peer invokes any of the build scripts associated with "my-golang-builder", it will propagate only the values of the environment variables in the whitelist.

Note: The following environment variables are always propagated to external builders:

- LD_LIBRARY_PATH
- LIBPATH
- PATH
- TMPDIR

When an `externalBuilder` configuration is present, the peer will iterate over the list of builders in the order provided, invoking `bin/detect` until one completes successfully. If no builder completes `detect` successfully, the peer will fallback to using the legacy Docker build process implemented within the peer. This means that external builders are completely optional.

In the example above, the peer will attempt to use "my-golang-builder", followed by "noop-builder", and finally the peer internal build process.

## Chaincode packages

As part of the new lifecycle introduced with Fabric 2.0, the chaincode package format changed from serialized protocol buffer messages to a gzip compressed POSIX tape archive. Chaincode packages created with `peer lifecycle chaincode package` use this new format.

### Lifecycle chaincode package contents

A lifecycle chaincode package contains two files. The first file, `code.tar.gz` is a gzip compressed POSIX tape archive. This file includes the source artifacts for the chaincode. Packages created by the peer CLI will place the chaincode implementation source under the `src` directory and chaincode metadata (like CouchDB indexes) under the `META-INF` directory.

The second file, `metadata.json` is a JSON document with three keys:
- `type`: the chaincode type (e.g. GOLANG, JAVA, NODE)
- `path`: for go chaincode, the GOPATH or GOMOD relative path to the main chaincode package; undefined for other types
- `label`: the chaincode label that is used to generate the package-id by which the package is identified within the new chaincode lifecycle process.

Note that the `type` and `path` fields are only utilized by docker platform builds.

### Chaincode packages and external builders

When a chaincode package is installed to a peer, the contents of `code.tar.gz` and `metadata.json` are not processed prior to calling external builders, except for the `label` field that is used by the new lifecycle process to compute the package id. This affords users a great deal of flexibility in how they package source and metadata that will be processed by external builders and launchers.

For example, a custom chaincode package could be constructed that contains a pre-compiled, implementation of chaincode in `code.tar.gz` with a `metadata.json` that allows a _binary buildpack_ to detect the custom package, validate the hash of the binary, and run the program as chaincode.

Another example would be a chaincode package that only contains state database index definitions and the data necessary for an external launcher to connect to a running chaincode server. In this case, the `build` process would simply extract the metadata from the process and `release` would present it to the peer.

The only requirements are that `code.tar.gz` can only contain regular file and directory entries, and that the entries cannot contain paths that would result in files being written outside of the logical root of the chaincode package.

<!---
Licensed under Creative Commons Attribution 4.0 International License https://creativecommons.org/licenses/by/4.0/
-->
