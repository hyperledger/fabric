# Install Fabric and Fabric Samples

Please install the [Prerequisites](./prereqs.html) before following these install instructions.

We think the best way to understand something is to use it yourself.  To help you use Fabric, we have created a simple Fabric test network using Docker compose, and a set of sample applications that demonstrate its core capabilities.

We also have precompiled `Fabric CLI tool binaries` and `Fabric Docker Images` which will be downloaded to your environment, to get you going.

The cURL command in the instructions below sets up your environment so that you can run the Fabric test network. Specifically, it performs the following steps:

* Clones the [hyperledger/fabric-samples](https://github.com/hyperledger/fabric-samples) repository.
* Downloads the latest Hyperledger Fabric Docker images and tags them as `latest`
* Downloads the following platform-specific Hyperledger Fabric CLI tool binaries and config files into the `fabric-samples` `/bin` and `/config` directories. These binaries will help you interact with the test network.
  * `configtxgen`,
  * `configtxlator`,
  * `cryptogen`,
  * `discover`,
  * `idemixgen`,
  * `orderer`,
  * `osnadmin`,
  * `peer`,
  * `fabric-ca-client`,
  * `fabric-ca-server`

## Download Fabric samples, Docker images, and binaries

A working directory is required - for example, Go Developers use the `$HOME/go/src/github.com/<your_github_userid>` directory.  This is a Golang Community recommendation for Go projects.

```shell
mkdir -p $HOME/go/src/github.com/<your_github_userid>
cd $HOME/go/src/github.com/<your_github_userid>
```

To get the install script:

```bash
curl -sSLO https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/install-fabric.sh && chmod +x install-fabric.sh
```

Run the script with the `-h` option to see the options:

```bash
./install-fabric.sh -h
Usage: ./install-fabric.sh [-f|--fabric-version <arg>] [-c|--ca-version <arg>] <comp-1> [<comp-2>] ... [<comp-n>] ...
        <comp>: Component to install one or more of  d[ocker]|b[inary]|s[amples]. If none specified, all will be installed
        -f, --fabric-version: FabricVersion (default: '2.5.0')
        -c, --ca-version: Fabric CA Version (default: '1.5.6')
```

## Choosing which components

To specify the components to download add one or more of the following arguments. Each argument can be shortened to its first letter.

* `docker` to use Docker to download the Fabric Container Images
* `podman` to use podman to download the Fabric Container Images
* `binary` to download the Fabric binaries
* `samples` to clone the fabric-samples github repo to the current directory

To pull the Docker containers and clone the samples repo, run one of these commands for example

```bash
./install-fabric.sh docker samples binary
or
./install-fabric.sh d s b
```

If no arguments are supplied, then the arguments `docker binary samples` are assumed.

## Choosing which version

By default the latest version of the components are used; these can be altered by using the options `--fabric-version` and `-ca-version`.  `-f` and `-c` are the respective short forms.

For example, to download the v2.5.0 binaries, run this command

```bash
./install-fabric.sh --fabric-version 2.5.0 binary
```

You have completed installing Fabric samples, Docker images, and binaries to your system.

* If you are looking to set up your environment to start contributing to Fabric, please refer to the instructions for [Setting up the contributor development environment](https://hyperledger-fabric.readthedocs.io/en/latest/dev-setup/devenv.html).

> Note: this is an updated install script with the same end-result as the existing script, but with an improved syntax. This script adopts the postitive opt-in approach to selecting the components to install.  The original script is still present at the same location `curl -sSL https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/bootstrap.sh| bash -s`

* If you need help, post your questions and share your logs on the **fabric-questions** channel on [Hyperledger Discord Chat](https://discord.com/invite/hyperledger) or on [StackOverflow](https://stackoverflow.com/questions/tagged/hyperledger-fabric).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
