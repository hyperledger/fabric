# Copyright IBM Corp All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
# This vagrantfile creates a VM that is capable of building and testing
# the fabric core.

Vagrant.require_version ">= 1.7.4"
Vagrant.configure('2') do |config|
  config.vm.box = "bento/ubuntu-20.04"
  config.vm.synced_folder "..", "/home/vagrant/fabric"
  config.ssh.forward_agent = true

  config.vm.provider :virtualbox do |vb|
    vb.name = "hyperledger"
    vb.cpus = 2
    vb.memory = 4096
  end

  config.vm.provision :shell, name: "essentials", path: "essentials.sh"
  config.vm.provision :shell, name: "docker", path: "docker.sh"
  config.vm.provision :shell, name: "golang", path: "golang.sh"
  config.vm.provision :shell, name: "limits", path: "limits.sh"
  config.vm.provision :shell, name: "softhsm", path: "softhsm.sh"
  config.vm.provision :shell, name: "user", privileged: false, path: "user.sh"
end

# -*- mode: ruby -*-
# vi: set ft=ruby :
