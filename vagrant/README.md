# Vagrant Development Environment

This directory contains the scripts necessary to bring up a simple development
environment with Vagrant. This environment is focused on the requirements for
building and testing the Fabric code and is not intended to be used for
application development or chaincode.

Most people will not need to use vagrant. Fabric can be easily built and
tested on Linux and macOS after installing the prerequisite tools and
libraries that are described in the documentation.

## Starting the Machine

Once the fabric code has been cloned, `vagrant up` can be run from the
`vagrant` folder. This will start an Ubuntu based virtual machine and
provision the tools necessary to build and test Fabric.

## Building and Testing in Vagrant

After starting the virtual machine with `vagrant up`, use `vagrant ssh` to
access the development shell. Once you're in the shell, `make clean && make`
can be used to build and test fabric.

The source code from the host machine is mounted inside the Vagrant virtual
machine so changes to the code can be made on the host or inside the VM.

```console
vagrant@ubuntu-xenial:~/fabric$ make clean && make
```
