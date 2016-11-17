## Setting up the development environment

### Overview
The current development environment utilizes Vagrant running an Ubuntu image, which in turn launches Docker containers. Conceptually, the Host launches a VM, which in turn launches Docker containers.

**Host -> VM -> Docker**

This model allows developers to leverage their favorite OS/editors and execute the system in a controlled environment that is consistent amongst the development team.

- Note that your Host should not run within a VM. If you attempt this, the VM within your Host may fail to boot with a message indicating that VT-x is not available.

### Prerequisites
* [Git client](https://git-scm.com/downloads)
* [Go](https://golang.org/) - 1.6 or later
* [Vagrant](https://www.vagrantup.com/) - 1.7.4 or later
* [VirtualBox](https://www.virtualbox.org/) - 5.0 or later
* BIOS Enabled Virtualization - Varies based on hardware

- Note: The BIOS Enabled Virtualization may be within the CPU or Security settings of the BIOS

### Steps

#### Set your GOPATH
Make sure you have properly setup your Host's [GOPATH environment variable](https://github.com/golang/go/wiki/GOPATH). This allows for both building within the Host and the VM.

#### Note to Windows users

If you are running Windows, before running any `git clone` commands, run the following command.
```
git config --get core.autocrlf
```
If `core.autocrlf` is set to `true`, you must set it to `false` by running
```
git config --global core.autocrlf false
```
If you continue with `core.autocrlf` set to `true`, the `vagrant up` command will fail with the error `./setup.sh: /bin/bash^M: bad interpreter: No such file or directory`

#### Cloning the Fabric project

Since the Fabric project is a `Go` project, you'll need to clone the Fabric repo to your $GOPATH/src directory. If your $GOPATH has multiple path components, then you will want to use the first one. There's a little bit of setup needed:

```
cd $GOPATH/src
mkdir -p github.com/hyperledger
cd github.com/hyperledger
```

Recall that we are using `Gerrit` for source control, which has its own internal git repositories. Hence, we will need to [clone from Gerrit](../Gerrit/gerrit.md#Working-with-a-local-clone-of-the-repository). For brevity, the command is as follows:
```
git clone ssh://LFID@gerrit.hyperledger.org:29418/fabric && scp -p -P 29418 LFID@gerrit.hyperledger.org:hooks/commit-msg fabric/.git/hooks/
```
**Note:** of course, you would want to replace `LFID` with your [Linux Foundation ID](../Gerrit/lf-account.md).

#### Boostrapping the VM using Vagrant

Now you're ready to launch Vagrant.

```
cd $GOPATH/src/github.com/hyperledger/fabric/devenv
vagrant up
```

Go get coffee... this will take a few minutes. Once complete, you should be able to `ssh` into the Vagrant VM just created.
```
vagrant ssh
```

### Building the fabric

Once you have your vagrant development environment established, you can proceed to [build and test](build.md) the fabric. Once inside the VM, you can find the peer project under `$GOPATH/src/github.com/hyperledger/fabric`. It is also mounted as  `/hyperledger`.

### Notes

**NOTE:** any time you change any of the files in your local fabric directory (under `$GOPATH/src/github.com/hyperledger/fabric`), the update will be instantly available within the VM fabric directory.

**NOTE:** If you intend to run the development environment behind an HTTP Proxy, you need to configure the guest so that the provisioning process may complete. You can achieve this via the *vagrant-proxyconf* plugin. Install with `vagrant plugin install vagrant-proxyconf` and then set the VAGRANT_HTTP_PROXY and VAGRANT_HTTPS_PROXY environment variables *before* you execute `vagrant up`. More details are available here: https://github.com/tmatilai/vagrant-proxyconf/

**NOTE:** The first time you run this command it may take quite a while to complete (it could take 30 minutes or more depending on your environment) and at times it may look like it's not doing anything. As long you don't get any error messages just leave it alone, it's all good, it's just cranking.

**NOTE to Windows 10 Users:** There is a known problem with vagrant on Windows 10 (see [mitchellh/vagrant#6754](https://github.com/mitchellh/vagrant/issues/6754)). If the `vagrant up` command fails it may be because you do not have Microsoft Visual C++ Redistributable installed. You can download the missing package at the following address: http://www.microsoft.com/en-us/download/details.aspx?id=8328
