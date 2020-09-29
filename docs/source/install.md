# Install Fabric and Fabric Samples

Please install the [Prerequisites](./prereqs.html) before following these install instructions.
<br>
We think the best way to understand something is to use it yourself.  To help you use Fabric, we have created a simple Fabric Network called `test-network` using docker compose, and a set of sample applications that demonstrate its core capabilities. We have also precompiled `Fabric CLI tool binaries` and `Fabric Docker Images` which will be downloaded to your environment, to get you going.


#### Mac
<br>
> 
>> **Install**  Install Fabric-Samples in `$HOME/go/src/github.com/<your_github_userid>` directory.  This is a Golang Community recommendation for Go projects and this location will also enable local directories to be shared with Docker containers
>> ```shell
>> $ mkdir -p $HOME/go/src/github.com/<your_github_userid>
>> $ cd $HOME/go/src/github.com/<your_github_userid>
>>```
>> Get the latest production release of Fabric Samples
>>```shell
>> $ curl -sSL https://bit.ly/2ysbOFE | bash -s
>>```
> You have completed installing Fabric samples, binaries and docker images to your system.
<br>


#### LINUX
<br>
> 
>> **Install**  Install Fabric-Samples in `$HOME/go/src/github.com/<your_github_userid>` directory.  This is a Golang Community recommendation for Go projects and this location will also enable local directories to be shared with Docker containers
>> ```shell
>> $ mkdir -p $HOME/go/src/github.com/<your_github_userid>
>> $ cd $HOME/go/src/github.com/<your_github_userid>
>>```
>> Get the latest production release of Fabric Samples
>>```shell
>> $ curl -sSL https://bit.ly/2ysbOFE | bash -s
>>```
> You have completed installing Fabric samples, binaries and docker images to your system.
<br>


#### Windows
<br>
> Enter the directory where you want to install Fabric in a terminal window and enter the following cURL command. Please consult the Docker documentation for [file sharing](https://docs.docker.com/docker-for-mac/#file-sharing) and The [GOPATH environment](https://golang.org/doc/gopath_code.html#GOPATH) documentation to see the best location under one of the shared drives. 

>> ```shell
>> $ curl -sSL https://bit.ly/2ysbOFE | bash -s
>>```
> You have completed installing Fabric samples, binaries and docker images to your system.
<br>


### Note:

* The cURL command performs the following steps
  * If needed, clone the [hyperledger/fabric-samples](https://github.com/hyperledger/fabric-samples) repository
  * Download the following platform-specific Hyperledger Fabric CLI tool binaries and config files you will need to set up your network into the `/bin` and `/config` directories
    * `configtxgen`,<br>
    * `configtxlator`,<br>
    * `cryptogen`,<br>
    * `discover`,<br>
    * `idemixgen`,<br>
    * `orderer`,<br>
    * `peer`,<br>
    * `fabric-ca-client`,<br>
    * `fabric-ca-server`<br>
  * Download the Hyperledger Fabric docker images for the version specified


* To view the help and available commands for the Fabric-Samples bootstrap script, please use the -h flag with the cURL command:
```shell
curl -sSL https://bit.ly/2ysbOFE | bash -s -- -h
```
* To curl specific releases of Fabric-Samples, please pass a version identifier for Fabric and Fabric-CA docker images. The command below demonstrates how to download the latest production releases - `Fabric v2.2.0` and `Fabric CA v1.4.8` 
```shell
curl -sSL https://bit.ly/2ysbOFE | bash -s -- <fabric_version> <fabric-ca_version>
curl -sSL https://bit.ly/2ysbOFE | bash -s -- 2.2.0 1.4.8
```


* If you get an error running the curl command
  * You may have too old a version of curl that does not handle redirects or an unsupported environment. Please make sure you use a newer version from the [cURL downloads page](https://curl.haxx.se/download.html)
  * Alternately, there might be an issue with the bit.ly, please retry the command with the un-shortened URL: 
  ```shell
  curl -sSL https://raw.githubusercontent.com/hyperledger/fabric/master/scripts/bootstrap.sh| bash -s 
  ```


* GOPATH: By default, you do not need to set the GOPATH environment variable, it is assumed to be $HOME/go on Unix systems and %USERPROFILE%\go on Windows. If however, you would like to use a different location for fabric-samples install, you may set GOPATH to point to your specific go workspace. For example on macOS 
  ```shell
  $ export GOPATH:$Home/<user-defined-workspace>/go
  ```
  Please consult the Docker documentation for [file sharing](https://docs.docker.com/docker-for-mac/#file-sharing) and the [GOPATH environment](https://golang.org/doc/gopath_code.html#GOPATH) documentation to understand any challenges therein. 


* If you are looking to set up your envrionment to start contributing to Fabric, please refer to the instructions for [Setting up the contributor development environment](https://hyperledger-fabric.readthedocs.io/en/latest/dev-setup/devenv.html).


If you continue to see errors, share your logs on the **fabric-questions** channel on [Hyperledger Rocket Chat](https://chat.hyperledger.org/home) or on [StackOverflow](https://stackoverflow.com/questions/tagged/hyperledger-fabric).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->

