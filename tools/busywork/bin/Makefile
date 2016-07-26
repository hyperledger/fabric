# Makefile for busywork/scripts

# Preliminaries: It is assumed that the GOPATH is set up correctly. It is
# assumed by many targets that the 'peer' and 'membersrvc' Docker images have
# been previosuly built. All Docker-related targets require that the user has
# unencumbered permission to run Docker (e.g., is root or a member of the
# 'docker' group). The user will also need password-less sudo access to run
# certain targets.

# The environment variable BUSYWORK_HOME names a directory that is used for
# log files and user-mode configurations. If not set, BUSYWORK_HOME defaults
# to $(HOME)/.busywork

# Note: You can easily maintain your own set of make targets by creating a
# file called 'private.mk' in this directory, and then tracking that file in
# your private fork. If 'private.mk' exists in this directory it will be
# included by this Makefile.

# TODO: Make image (re-)building contingent on all GO code being up-to-date.

############################################################################
# Documentation
############################################################################

# The following are the supported make targets:

ifeq (make, targets)

############################################################################
# Compilation and Docker image building

build build_peer build_membersrvc
rebuild new

    The 'build*' targets do a 'go build' of the peer and membersrvc servers,
    or both.  Use the ARGS variable with these targets to pass args to 'go
    build'. E.g., to do 'go build -a', execute

        make build ARGS=-a

    The 'rebuild' target does the above (-a) for both the peer and membersrvc
    servers. The 'new' can be run after every 'git pull' to 'rebuild' both
    servers and the 'images'.

images peer_image membersrvc_image

    The 'images' target builds the peer and membersrvc images from
    a clean slate. The 'peer_image' and 'membersrvc_image' targets rebuild
    individual images. NB: If you are using use-mode peer networks
    (userModeNetwork) you _do not_ need to build these images.


############################################################################
# Testing

test
go_tests
behave_tests behave_tests_with_logs

    You can run the tests that are part of the Go source code ('go_tests'),
    the test suite coded in the Behave environment ('behave_tests') or both
    test suites ('test'). Use 'behave_tests_with_logs' if you want logs from
    the Docker containers.

busyTest

    Run a simple test of the 'busy' script.


############################################################################
# 'Clean' routines. Note: Docker does not understand that "rm -f" usually
# means "don't complain and exit normally if the thing asked to be deleted is
# not found". You will likely see lots of messages from Docker when running
# these targets, and because of this we typically continue even in the evente
# of "errors".

docker_chaincode_clean

    First do docker_ps_clean, then force-remove all container images created for
    chaincodes.

docker_fabric_clean

    First do docker_chaincode_clean, then force-remove all remaining container
    images related to the fabric.

docker_ps_clean

    Force remove _all_ running and stopped Docker containers.

docker_reset

    Stop Docker, remove /var/lib/docker, restart Docker.

fabric_clean
fabric_reset

    'fabric_clean' kills all 'fabric' and 'membersrvc' images
    running on the system. 'fabric_reset' goes further and also deletes the
    fabric and memberservices databases, and does a docker_chaincode_clean.


############################################################################
# Miscellaneous targets

docker_ips

    List IP addresses of all containers

cclogs

    Create log directories from all 'dev-vp*' Docker chaincode
    containers. Ideally this feature will eventually move to fabric options
    that allow chaincode logs to be written directly to disk.  Note that all
    previous chaincode container logging directories are first removed from
    BUSYWORK_HOME.

endif


############################################################################
# Make targets
############################################################################

ifeq ($(BUSYWORK_HOME),)
BUSYWORK_HOME = $(HOME)/.busywork
ignore = $(shell mkdir -p $(BUSYWORK_HOME))
endif

FABRIC = $(GOPATH)/src/github.com/hyperledger/fabric

.PHONY: null
null:
	@echo "**********************************************************************"
	@echo "* There is no default make target                                    *"
	@echo "* The defined make targets are documented at the top of the Makefile *"
	@echo "**********************************************************************"
	@exit 1



############################################################################
# Compilation and Docker image building

.PHONY: build build_peer build_membersrvc new rebuild

rebuild:
	$(MAKE) build ARGS=-a

new: rebuild images

build: build_peer build_membersrvc

build_peer:
	cd $(FABRIC)/peer; go build $(ARGS)

build_membersrvc:
	cd $(FABRIC)/membersrvc; go build $(ARGS)


.PHONY: images peer_image membersrvc_image

images: docker_fabric_clean
	$(MAKE) peer_image
	$(MAKE) membersrvc_image

membersrvc_image:
	cd $(FABRIC); $(MAKE) membersrvc-image-clean membersrvc-image

peer_image:
	cd $(FABRIC); $(MAKE) peer-image-clean peer-image


############################################################################
# Testing

.PHONY: test go_tests behave_tests behave_tests_with_logs busyTest

go_tests: nosec_noops_1
	cd $(GOPATH)/src/github.com/hyperledger/fabric; \
	go test -timeout=20m $$(go list github.com/hyperledger/fabric/... | \
		grep -v /vendor/ | grep -v /examples/)

behave_tests: fabric_reset
	cd $(GOPATH)/src/github.com/hyperledger/fabric/bddtests; \
	behave

behave_tests_with_logs: fabric_reset
	cd $(GOPATH)/src/github.com/hyperledger/fabric/bddtests; \
	behave -D logs=Y

test: go_tests behave_tests

busyTest:
	./busyTest

############################################################################
# "Clean" routines

.PHONY: docker_chaincode_clean \
	docker_fabric_clean docker_ps_clean docker_reset \
	fabric_clean fabric_reset


docker_chaincode_clean: docker_ps_clean
	-docker images | egrep '^dev-|^<none>' | \
		awk '{print $$3}' | xargs docker rmi -f

docker_fabric_clean: docker_chaincode_clean
	cd $(FABRIC); $(MAKE) peer-image-clean membersrvc-image-clean

docker_ps_clean:
	-docker ps -a -q | xargs docker rm -f

docker_reset:
	sudo service docker stop
	sudo rm -rf /var/lib/docker
	sudo service docker start

fabric_clean:
	-pkill peer
	-pkill membersrvc

fabric_reset: fabric_clean docker_chaincode_clean
	rm -rf /var/hyperledger/*


############################################################################
# Miscellaneous targets

.PHONY: docker_ips cclogs

docker_ips:
	@docker ps -a -q | xargs -n 1 ./docker_ip

cclogs:
	@rm -rf $(BUSYWORK_HOME)/dev-vp*
	@for container in `docker ps -a | grep dev-vp | awk '{print \$$2}'`; do \
		dir=$(BUSYWORK_HOME)/$$container\_logs; \
		mkdir -p $$dir; \
		docker logs $$container 1> $$dir/stdout 2> $$dir/stderr; \
	done
	@ls -al $(BUSYWORK_HOME)/dev-vp*


# Load the user's private makefile, if it exists.

ifneq (,$(wildcard private.mk))
include private.mk
endif
