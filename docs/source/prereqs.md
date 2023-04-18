# Prerequisites

The following prerequisites are required to run a Docker-based Fabric test network on your local machine.

## Mac

<!--- Indent entire section -->
<div style="margin-left: 1.5em;">

### Homebrew

For macOS, we recommend using [Homebrew](https://brew.sh) to manage the prereqs.

```shell
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install.sh)"
brew --version # => Homebrew 2.5.2
```

The Xcode command line tools will be installed as part of the Homebrew installation.
Once Homebrew is ready, installing the necessary prerequisites is very easy:

### Git

Install the latest version of [git](https://git-scm.com/downloads) if it is not already installed.

```shell
brew install git
git --version # => git version 2.23.0
```

### cURL

Install the latest version of [cURL](https://curl.haxx.se/download.html) if it is not already installed.

```shell
brew install curl
curl --version # => curl 7.64.1 (...)
```

### Docker

Install the latest version of [Docker Desktop](https://docs.docker.com/get-docker/) if it is not already installed.
Since Docker Desktop is a UI application on Mac, use `cask` to install it.

Homebrew v2.x:

```shell
brew cask install --appdir="/Applications" docker
```

Homebrew v3.x:

```shell
brew install --cask --appdir="/Applications" docker
```

Docker Desktop must be launched to complete the installation so be sure to open the application after installing it:

```shell
open /Applications/Docker.app
```

Once installed, confirm the latest versions of both `docker` and `docker-compose` executables were installed.

```shell
docker --version # => Docker version 19.03.12, build 48a66213fe
docker-compose --version # => docker-compose version 1.27.2, build 18f557f9
```

> **Note:** Some users have reported errors while running Fabric-Samples with the Docker Desktop `gRPC FUSE for file sharing` option checked.
> Please uncheck this option in your Docker Preferences to continue using `osxfs for file sharing`.

### Go

Optional: Install the latest Fabric supported version of [Go](https://golang.org/doc/install) if it is not already
installed (only required if you will be writing Go chaincode or SDK applications).

```shell
brew install go@1.20.3
go version # => go1.20.3 darwin/amd64
```

### JQ

Optional: Install the latest version of [jq](https://stedolan.github.io/jq/download/) if it is not already installed
(only required for the tutorials related to channel configuration transactions).

```shell
brew install jq
jq --version # => jq-1.6
```
</div>

## **Linux (Ubuntu/Debian based distro)**

<!--- Indent entire section -->
<div style="margin-left: 1.5em;">

Prerequisites: [git](https://git-scm.com/downloads), [cURL](https://curl.haxx.se/download.html), [Docker](https://docs.docker.com/get-docker/)

```shell
sudo apt-get install git curl docker-compose -y

# Make sure the Docker daemon is running.
sudo systemctl start docker

# Add your user to the Docker group.
sudo usermod -a -G docker <username>

# Check version numbers  
docker --version
docker-compose --version

# Optional: If you want the Docker daemon to start when the system starts, use the following:
sudo systemctl enable docker
```

### Go

Optional: Install the latest version of [Go](https://golang.org/doc/install) (only required if you will be writing Go chaincode or SDK applications).

### JQ

Optional: Install the latest version of [jq](https://stedolan.github.io/jq/download/) (only required for the tutorials related to channel configuration transactions).

</div>

## **Windows**

<!--- Indent entire section -->
<div style="margin-left: 1.5em;">

### Docker

Install the latest version of [Docker Desktop](https://docs.docker.com/get-docker/) if it is not already installed.

### WSL2

Both the Fabric documentation and Fabric samples rely heavily on a `bash` environment. The recommended
path is to use WSL2 (Windows Subsystem for Linux version 2) to provide a native Linux environment and then you can follow the Linux prerequisites section (excluding the Linux Docker prerequisite as you already have Docker Desktop) and install them into your WSL2 linux distribution.

WSL2 may not be installed by default; you can check and install WSL2 by going into "Programs and Features", clicking on "Turn Windows features on or off" and ensuring that both "Windows Subsystem For Linux" and "Virtual Machine Platform" are selected.

Next you will need to install a Linux distribution such as Ubuntu-20.04 and make sure it's set to using version 2 of WSL. Refer to [Install WSL](https://docs.microsoft.com/en-us/windows/wsl/install) for more information.

Finally, you need to ensure Docker Desktop has integration enabled for your distribution so it can interact with Docker elements, such as a bash command window. To do this, open the Docker Desktop gui and go into settings, select `Resources` and them `WSL Integration` and ensure the checkbox for enable integration is checked. You should then see your WSL2 linux distribution listed (if you don't then it is probably because it is still a WSL1 distribution and needs to be converted to WSL2) and you can then toggle the switch to enable integration for that distro. Refer to [Docker Desktop WSL2 backend](https://docs.docker.com/desktop/windows/wsl/) for more information

### Microsoft VS Code (Optional)

Microsoft VS Code provides an IDE that has tight integration with WSL2 Linux Distibutions. Search the Microsoft Marketplace in VS Code for the Remote Development extension pack for more information. This pack includes, among other things, the `Remote - WSL extension` and the `Remote - Containers` extension.

### Git For Windows (Optional)

Although not required, if you do decide to install Git on Windows and manage the Fabric repositories natively (as opposed to within WSL2 and its Git installation), then make sure you configure Git as follows:

Update the following `git` configurations:
```shell
git config --global core.autocrlf false
git config --global core.longpaths true
```

You can check the setting of these parameters with the following commands:
```shell
git config --get core.autocrlf
git config --get core.longpaths
```
These output from these commands should be false and true respectively.

</div>

## **Notes**

- These prerequisites are recommended for Fabric users. If you are a Fabric developer, please refer to the instructions for [Setting up the development environment](https://hyperledger-fabric.readthedocs.io/en/latest/dev-setup/devenv.html).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
