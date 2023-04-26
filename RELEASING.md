# Releasing

Hyperledger Fabric uses a Github Actions [Release workflow](https://github.com/hyperledger/fabric/actions/workflows/release.yml).

The following artifacts are published when creating a new release:

- [Github release](https://github.com/hyperledger/fabric/releases) with release notes and attached binary tars for the following architectures:
  - darwin-amd64
  - darwin-arm64 (new since v2.5.0)
  - linux-amd64
  - linux-arm64 (new since v2.5.0)
  - windows-amd64

- DockerHub images for [peer](https://hub.docker.com/r/hyperledger/fabric-peer/tags), orderer, ccenv, baseos, tools (CLI) for the following architectures:
  - linux-amd64
  - linux-arm64 (new since v2.5.0)

## Before Releasing

Verify [fabric-test](https://dev.azure.com/Hyperledger/Fabric-Test/_build) and [fabric-samples](https://github.com/hyperledger/fabric-samples/actions) CI is healthy.
fabric-samples can be updated to utilize locally built fabric images. Alternatively fabric-samples CI can be updated to utilize an official fabric Github pre-release (e.g. alpha, beta, rc).

Releases are typically cut from a stable release branch, e.g. `release-2.5`, but can be cut from `main` branch (preview, alpha, and beta releases are often cut from `main` branch before a release stabilizes in its own release branch).

Create a release commit that updates version, documentation, release notes, install scripts. Example [pull request](https://github.com/hyperledger/fabric/pull/4133).

## Create Release

Draft a GitHub release on the [releases page](https://github.com/hyperledger/fabric/releases) to trigger the release workflow to publish the new release.
- Specify a tag name and matching release title, e.g. `v2.5.0`.
- Choose the branch to release from

Technically, this will simply create a tagged release with no artifacts, and then the tagging action will actually trigger the [release workflow](https://github.com/hyperledger/fabric/blob/main/.github/workflows/release.yml) that publishes the release artifacts.

The release workflow performs 3 jobs:
- Build Fabric Binaries
- Build and Push docker images
- Create Github Release based on release_notes/vX.X.X.md and binaries built in first job

Navigate to the [release workflow](https://github.com/hyperledger/fabric/actions/workflows/release.yml) to monitor progress.

[NOTE FOR release-2.2 ONLY] - `release-2.2` still uses a manually dispatched workflow instead of the tag trigger described above. Follow these steps instead:
- Navigate to [Release workflow](https://github.com/hyperledger/fabric/actions/workflows/release.yml)
- Select to run workflow for branch `release-2.2`
- Set the version, two digit version, and commit hash, click Run Workflow

## Test release artifacts

Verify Github release and DockerHub images got created.

Pull down the artifacts and run test-network as a user reading the documentation would:

```
curl -sSLO https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/install-fabric.sh && chmod +x install-fabric.sh

./install-fabric.sh --fabric-version 2.5.0 --ca-version 1.5.6

cd fabric-samples/test-network
./network.sh up createChannel -ca -c mychannel -s couchdb
./network.sh deployCC -ccn basicgo -ccp ../asset-transfer-basic/chaincode-go/ -ccl go

export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=$PWD/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051

peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C mychannel -n basicgo --peerAddresses localhost:7051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses localhost:9051 --tlsRootCertFiles ${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"function":"InitLedger","Args":[]}'

peer chaincode query -C mychannel -n basicgo -c '{"Args":["GetAllAssets"]}'

docker logs peer0.org1.example.com

./network.sh down
```

## After releasing

In the fabric release branch, update the version to next expected version (if known).

In the fabric `main` branch, update the docs and scripts to mention the new release. Example [pull request](https://github.com/hyperledger/fabric/pull/4138).

Update fabric-samples CI to utilize the new release.

fabric-samples main branch is typically set up to work with latest fabric release branch and main branch releases, therefore no tag is typically required in fabric-samples.
However, prior fabric-samples release branches such as `release-2.2` require the latest `release-2.2` commit to have tags for the fabric v2.2.x releases, so that when users clone the samples via the install script the corresponding samples release will be checked out.
Example tagging procedure for a fabric-samples administrator:

```
git tag v2.2.11 c3a0e81
git push origin v2.2.11
```
