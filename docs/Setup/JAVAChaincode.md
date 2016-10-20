## Java chaincode

Note: This guide generally assumes you have followed the Chaincode development environment setup tutorial [here](https://github.com/hyperledger/fabric/blob/master/docs/Setup/Chaincode-setup.md).

### To get started developing Java chaincode

1. Ensure you have gradle
  * Download the binary distribution from [http://gradle.org/gradle-download/](http://gradle.org/gradle-download/)
  * Unpack, move to the desired location, and add gradle's bin directory to your system path
  * Ensure `gradle -v` works from the command-line, and shows version 2.12 or greater
  * Optionally, enable the [gradle daemon](https://docs.gradle.org/current/userguide/gradle_daemon.html) for faster builds
2. Ensure you have the Java 1.8 **JDK** installed. Also ensure Java's directory is on your path with `java -version`
  * Additionally, you will need to have the [`JAVA HOME`](https://docs.oracle.com/cd/E19182-01/821-0917/6nluh6gq9/index.html) variable set to your **JDK** installation in your system path
3. From your command line terminal, move to the `devenv` subdirectory of your workspace environment. Log into a Vagrant terminal by executing the following command:

    ```
    vagrant ssh
    ```

4. Build and run the peer process.

    ```
    cd $GOPATH/src/github.com/hyperledger/fabric
    make peer
    peer node start
    ```

5. The following steps is for deploying chaincode in non-dev mode.

	* Deploy the chaincode,

```
	peer chaincode deploy -l java -p /opt/gopath/src/github.com/hyperledger/fabric/examples/chaincode/java/SimpleSample -c '{"Args": ["init", "a","100", "b", "200"]}'
```

`6d9a704d95284593fe802a5de89f84e86fb975f00830bc6488713f9441b835cf32d9cd07b087b90e5cb57a88360f90a4de39521a5595545ad689cd64791679e9`

		* This command will give the 'name' for this chaincode, and use this value in all the further commands with the -n (name) parameter


		* PS. This may take a few minutes depending on the environment as it deploys the chaincode in the container,

* Invoke a transfer transaction,

```
	peer chaincode invoke -l java \
	-n 6d9a704d95284593fe802a5de89f84e86fb975f00830bc6488713f9441b835cf32d9cd07b087b90e5cb57a88360f90a4de39521a5595545ad689cd64791679e9 \
	-c '{"Args": ["transfer", "a", "b", "10"]}'
```
`c7dde1d7-fae5-4b68-9ab1-928d61d1e346`

* Query the values of a and b after the transfer

```
	peer chaincode query -l java \
	-n 6d9a704d95284593fe802a5de89f84e86fb975f00830bc6488713f9441b835cf32d9cd07b087b90e5cb57a88360f90a4de39521a5595545ad689cd64791679e9 \
	-c '{ "Args": ["query", "a"]}'
	{"Name":"a","Amount":"80"}


	peer chaincode query -l java \
	-n 6d9a704d95284593fe802a5de89f84e86fb975f00830bc6488713f9441b835cf32d9cd07b087b90e5cb57a88360f90a4de39521a5595545ad689cd64791679e9 \
	-c '{ "Args": ["query", "b"]}'
	{"Name":"b","Amount":"220"}
```


### Java chaincode deployment in DEV Mode

1. Follow the step 1 to 3 as above,
2. Build and run the peer process
```    
    cd $GOPATH/src/github.com/hyperledger/fabric
    make peer
    peer node start --peer-chaincodedev
```
3. Open the second Vagrant terminal and build the Java shim layer and publish it to Local Maven Repo
```
cd $GOPATH/src/github.com/hyperledger/fabric/core/chaincode/shim/java
gradle -b build.gradle clean
gradle -b build.gradle build
```
4. Change to examples folder to build and run,
```
    cd $GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/java/SimpleSample
    gradle -b build.gradle build
```
5. Run the SimpleSample chaincode using the `gradle -b build.gradle run`

6. Open the third Vagrant terminal to run init and invoke on the chaincode

    peer chaincode deploy -l java -n SimpleSample -c '{"Args": ["init", "a","100", "b", "200"]}'
```
2016/06/28 19:10:15 Load docker HostConfig: %+v &{[] [] []  [] false map[] [] false [] [] [] [] host    { 0} [] { map[]} false []  0 0 0 false 0    0 0 0 []}
19:10:15.461 [crypto] main -> INFO 002 Log level recognized 'info', set to INFO
SimpleSample
```
    peer chaincode invoke -l java -n SimpleSample -c '{"Args": ["transfer", "a", "b", "10"]}'

```
2016/06/28 19:11:13 Load docker HostConfig: %+v &{[] [] []  [] false map[] [] false [] [] [] [] host    { 0} [] { map[]} false []  0 0 0 false 0    0 0 0 []}
19:11:13.553 [crypto] main -> INFO 002 Log level recognized 'info', set to INFO
978ff89e-e4ef-43da-a9f8-625f2f6f04e5
```
    peer chaincode query -l java -n SimpleSample -c '{ "Args": ["query", "a"]}'
```
2016/06/28 19:12:19 Load docker HostConfig: %+v &{[] [] []  [] false map[] [] false [] [] [] [] host    { 0} [] { map[]} false []  0 0 0 false 0    0 0 0 []}
19:12:19.289 [crypto] main -> INFO 002 Log level recognized 'info', set to INFO
{"Name":"a","Amount":"90"}
```
    peer chaincode query -l java -n SimpleSample -c '{"Args": ["query", "b"]}'
```
2016/06/28 19:12:25 Load docker HostConfig: %+v &{[] [] []  [] false map[] [] false [] [] [] [] host    { 0} [] { map[]} false []  0 0 0 false 0    0 0 0 []}
19:12:25.667 [crypto] main -> INFO 002 Log level recognized 'info', set to INFO
{"Name":"b","Amount":"210"}
```

### Developing new JAVA chaincode
1. Create a new Java project structure.
2. Use existing `build.grade` from  any example JAVA Chaincode project like `examples/chaincode/java/SimpleSample`.
3. Make your main class extend ChaincodeBase class and implement the following methods from base class.
  1. `public String run(ChaincodeStub stub, String function, String[] args)   `
  2. `public String query(ChaincodeStub stub, String function, String[] args)`
  3. `public String getChaincodeID()`
4. Modify the `mainClassName` in `build.gradle` to point to your new class.
5. Build this project using `gradle -b build.gradle build`
6. Run this chaincode after starting a peer in dev-mode as above using `gradle -b build.gradle run`


