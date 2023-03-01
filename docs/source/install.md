# Install Fabric and Fabric Samples

Please install the [Prerequisites](./prereqs.html) before following these install instructions.

We think the best way to understand something is to use it yourself.  To help you use Fabric, we have created a simple Fabric test network using docker compose, and a set of sample applications that demonstrate its core capabilities.
We have also precompiled `Fabric CLI tool binaries` and `Fabric Docker Images` which will be downloaded to your environment, to get you going.

The cURL command in the instructions below sets up your environment so that you can run the Fabric test network. Specifically, it performs the following steps:
* Clones the [hyperledger/fabric-samples](https://github.com/hyperledger/fabric-samples) repository.
* Downloads the latest Hyperledger Fabric Docker images and tags them as `latest`
* Downloads the following platform-specific Hyperledger Fabric CLI tool binaries and config files into the `fabric-samples` `/bin` and `/config` directories. These binaries will help you interact with the test network.
  * `configtxgen`,<br>
  * `configtxlator`,<br>
  * `cryptogen`,<br>
  * `discover`,<br>
  * `idemixgen`,<br>
  * `orderer`,<br>
  * `osnadmin`,<br>
  * `peer`,<br>
  * `fabric-ca-client`,<br>
  * `fabric-ca-server`<br>


## Download Fabric samples, docker images, and binaries.

Download `fabric-samples` to the `$HOME/go/src/github.com/<your_github_userid>` directory.  This is a Golang Community recommendation for Go projects. If you are using a different directory or Windows, see the [Notes](https://hyperledger-fabric.readthedocs.io/en/latest/install.html#notes) below.

```shell
$ mkdir -p $HOME/go/src/github.com/<your_github_userid>
$ cd $HOME/go/src/github.com/<your_github_userid>
```

Download the latest release of Fabric samples, docker images, and binaries.

```shell
$ curl -sSL https://bit.ly/2ysbOFE | bash -s
```

You have completed installing Fabric samples, docker images, and binaries to your system.

## Advanced download options

To view the help and available commands for the download script, please use the `-h` flag with the cURL command:

```shell
curl -sSL https://bit.ly/2ysbOFE | bash -s -- -h
```

To download a specific release, pass a version identifier for Fabric and Fabric CA Docker images. The command below demonstrates how to download the latest production releases - `Fabric v2.4.9` and `Fabric CA v1.5.5` 

```shell
curl -sSL https://bit.ly/2ysbOFE | bash -s -- <fabric_version> <fabric-ca_version>
curl -sSL https://bit.ly/2ysbOFE | bash -s -- 2.4.9 1.5.5
```

## Notes

### Other considerations

* Setting GOPATH is not required when using Go modules in your projects, or when using the recommended directory. If you would like to use a different location for fabric-samples, you may set GOPATH to point to your specific Go workspace. For example on macOS:

  ```shell
  $ export GOPATH:$Home/<user-defined-workspace>/go
  ```

* If you are looking to set up your environment to start contributing to Fabric, please refer to the instructions for [Setting up the contributor development environment](https://hyperledger-fabric.readthedocs.io/en/latest/dev-setup/devenv.html).

## Troubleshooting

* If you get an error running the cURL command
  * You may have too old a version of cURL that does not handle redirects or an unsupported environment. Please make sure you use a newer version from the [cURL downloads page](https://curl.haxx.se/download.html)
  * Alternately, there might be an issue with the bit.ly, please retry the command with the un-shortened URL:
  ```shell
  curl -sSL https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/bootstrap.sh| bash -s 
  ```

* If you need help, post your questions and share your logs on the **fabric-questions** channel on [Hyperledger Rocket Chat](https://chat.hyperledger.org/home) or on [StackOverflow](https://stackoverflow.com/questions/tagged/hyperledger-fabric).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
