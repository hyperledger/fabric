# Makefile for counters

############################################################################
# Documentation
############################################################################

# Note: You can easily maintain your own set of make targets by creating a
# file called 'private.mk' in this directory, and then tracking that file in
# your private fork. If 'private.mk' exists in this directory it will be
# included by this Makefile.

# The following are the supported make targets:

ifeq (make, targets)

############################################################################
# Stress tests

stress1

    A single validating peer with NOOPS consensus and no security. 64 clients
    each drive 1000 transactions into a single array without interlock.

stress2b stress2n

    A user-mode network of 4 validating peers running without security. Send
    10,000 transactions into a single array as a single burst, using only the
    first peer, with one interlock at the end of the burst. Consensus
    protocols are PBFT Batch (stress2b) or NOOPS (stress2n).

secure1

    A 4-node setup with security. Security-mode is so slow that this is not a
    very stressful test. This test uses PBFT batch mode.

sweep1b sweep1n

    *** Note: The 'sweep1b' target currently appears to work in large server ***
    *** environments but not in Vagrant/laptop environments                  ***

    A user-mode network of 4 validating peers running without security. One
    chaincode with one array. Sweep the number of clients from 1 to 64 by
    powers-of-2, sending a constant 32K transactions into the network
    targeting all peers in short bursts, each client interlocking every 64
    transactions. Consensus protocols are PBFT Batch (sweep1b) or NOOPS
    (sweep1n).

sweep2b sweep2n

    *** Note: The 'sweep2b' target does not work at present. These targets ***
    *** may be modified or removed in the future.                          ***

    A user-mode network of 4 validating peers running without security. Four
    chaincodes, each managing 128 arrays. The median array size here is 64
    counters or 512 bytes. Sweep the number of clients from 1 to 64 by
    powers-of-2, sending a constant 32K transactions into the network
    targeting all peers in moderate bursts, each client interlocking every 256
    transactions. Consensus protocols are PBFT Batch (sweep2b) or NOOPS
    (sweep2n).


############################################################################
# Miscellaneous

build

    Build the chaincode

countersTest

    Test the 'counters' chaincode

endif

############################################################################
# Make targets
############################################################################

############################################################################
# Stress tests

NETWORK = ../bin/userModeNetwork

.PHONY: stress1
stress1: 
	@$(NETWORK) -noops 1
	@./driver \
		-clients 64 \
		-transactions 1000 \
		-peerBurst 1000 \
		-keepLog


.PHONY: stress2b stress2n
STRESS2 = \
	echo "makefile: Sleeping 10 seconds to allow peers to interlock;" \
	sleep 10; \
	./driver \
		-transactions 10000 \
		-peerBurst 10000 \
		-targetPeers 1 \
		-interlock \
		-interlockTimeout 5m \
		-keepLog \
		-checkAgreement -dupCheck

STRESS2_CONFIG = \
	@CORE_PBFT_GENERAL_TIMEOUT_NULLREQUEST=3s

stress2b: 
	@$(STRESS2_CONFIG) $(NETWORK) -batch 4
	@$(STRESS2)

stress2n: 
	@$(STRESS2_CONFIG) $(NETWORK) -noops 4
	@$(STRESS2)

.PHONY: secure1
secure1:
	@CORE_PBFT_GENERAL_TIMEOUT_NULLREQUEST=3s \
	$(NETWORK) -batch -security 4
	@echo "makefile: Sleeping 10 seconds to allow peers to interlock"
	@sleep 10	
	@./driver \
		-transactions 100 \
		-peerBurst 10 \
		-interlock \
		-keepLog \
		-checkAgreement -dupCheck

.PHONY: sweep1b sweep1n
SWEEP1 = \
	echo "makefile: Sleeping 10 seconds to allow peers to interlock"; \
	sleep 10; \
	set -e; \
	./driver \
		-clients $$clients \
		-transactions $$((32768 / $$clients)) \
		-peerBurst 8 \
		-netBurst 8 \
		-interlock -interlockTimeout 5m \
		-keepLog \
		-convergeWait 5m -convergePoll 5s \
		-checkAgreement -dupCheck

SWEEP1_CONFIG = \
	CORE_PBFT_GENERAL_TIMEOUT_NULLREQUEST=3s \
	CORE_PEER_VALIDATOR_CONSENSUS_BUFFERSIZE=32768

sweep1b: 
	@for clients in 1 2 4 8 16 32 64; do \
		$(SWEEP1_CONFIG) $(NETWORK) -batch 4; \
		$(SWEEP1); \
	done

sweep1n: 
	@for clients in 1 2 4 8 16 32 64; do \
		$(SWEEP1_CONFIG) $(NETWORK) -noops 4; \
		$(SWEEP1); \
	done


.PHONY: sweep2b sweep2n
SWEEP2 = \
	echo "makefile: Sleeping 10 seconds to allow peers to interlock"; \
	sleep 10; \
	set -e; \
	./driver \
		-clients $$clients \
		-chaincodes 4 \
		-arrays 128 \
		-transactions $$((64 / $$clients)) \
		-peerBurst 16 \
		-netBurst 16 \
		-interlock -interlockTimeout 5m \
		-keepLog \
		-convergeWait 5m -convergePoll 5s \
		-checkAgreement -dupCheck

SWEEP2_CONFIG = \
	CORE_PBFT_GENERAL_TIMEOUT_NULLREQUEST=3s \
	CORE_PEER_VALIDATOR_CONSENSUS_BUFFERSIZE=32768

sweep2b: 
	@for clients in 1 2 4 8 16 32 64; do \
		$(SWEEP2_CONFIG) $(NETWORK) -batch 4; \
		$(SWEEP2); \
	done

sweep2n: 
	@for clients in 1 2 4 8 16 32 64; do \
		$(SWEEP2_CONFIG) $(NETWORK) -noops 4; \
		$(SWEEP2); \
	done


############################################################################
# Miscellaneous

.PHONY: build
build:
	go build -a

.PHONY: countersTest
countersTest:
	./countersTest


# Load the user's private makefile, if it exists.

ifneq (,$(wildcard private.mk))
include private.mk
endif
