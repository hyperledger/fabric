# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
PEERS += $(patsubst %,peer%,$(shell seq 1 4))
NODES += $(PEERS)
NODES += orderer
NODES += cli
NODES += ca
NODES += tlsca

DAEMONS = $(filter-out cli,$(NODES))

CHANNEL_NAME ?= mychannel

CRYPTOOUTPUT = build/cryptogen
ORDERER_ORG = $(CRYPTOOUTPUT)/ordererOrganizations/orderer.net
PEER_ORG= $(CRYPTOOUTPUT)/peerOrganizations/org1.net

CA_PATH = $(PEER_ORG)/ca
TLSCA_PATH= $(PEER_ORG)/tlsca
ORDERER_PATH = $(ORDERER_ORG)/orderers
PEER_PATH = $(PEER_ORG)/peers
USERS_PATH = $(PEER_ORG)/users

CHANNEL_TXNS=build/channel.tx build/anchor.tx

mspmap.orderer    := $(ORDERER_PATH)/orderer.orderer.net
mspmap.peer1      := $(PEER_PATH)/peer1.org1.net
mspmap.peer2      := $(PEER_PATH)/peer2.org1.net
mspmap.peer3      := $(PEER_PATH)/peer3.org1.net
mspmap.peer4      := $(PEER_PATH)/peer4.org1.net
mspmap.cli        := $(USERS_PATH)/Admin@org1.net

COMPOSE=docker-compose -f compose/docker-compose.yaml
DRUN=$(COMPOSE) run --rm cli

TLS ?= true
export TLS_ENABLED=$(TLS)

CRYPTOGEN=build/bin/cryptogen
CONFIGTXGEN=build/bin/configtxgen

FABRICPKG=github.com/hyperledger/fabric
pkgmap.cryptogen      := $(FABRICPKG)/common/tools/cryptogen
pkgmap.configtxgen    := $(FABRICPKG)/common/configtx/tool/configtxgen

help:
	@cat usage.txt

compose-up: nodes
	@echo "Booting docker-compose environment"
	$(COMPOSE) up -d $(DAEMONS)
	$(DRUN) ./configure.sh $(CHANNEL_NAME) "$(CHANNEL_TXNS)" "$(PEERS)" $(TLS)
	@./compose/report-env.sh "$(DAEMONS)" build/client.config $(TLS)

compose-down:
	$(COMPOSE) down
	rm -rf build/nodes $(CRYPTOOUTPUT)

nodes: $(patsubst %,build/nodes/%,$(NODES))

$(CRYPTOOUTPUT): config/cryptogen.yaml $(CRYPTOGEN)
	@mkdir -p ${@D}
	$(CRYPTOGEN) generate --config $< --output $@

.PRECIOUS: %.yaml
%.yaml:
	@mkdir -p ${@D}
	cp config/${@F} $@

%/genesis.block: build/configtx.yaml build/core.yaml $(CONFIGTXGEN)
	@mkdir -p ${@D}
	FABRIC_CFG_PATH=build $(CONFIGTXGEN) -profile SampleOrg -outputBlock $@

%.tx: build/configtx.yaml build/core.yaml $(CONFIGTXGEN)

%/channel.tx:
	@mkdir -p ${@D}
	FABRIC_CFG_PATH=build $(CONFIGTXGEN) -profile SampleChannel \
		-channelID ${CHANNEL_NAME} \
		-outputCreateChannelTx $@

%/anchor.tx:
	@mkdir -p ${@D}
	FABRIC_CFG_PATH=build $(CONFIGTXGEN) -profile SampleChannel \
		-channelID ${CHANNEL_NAME} \
		-outputAnchorPeersUpdate $@ \
		-asOrg Org1MSP

.PRECIOUS: %/msp
%/msp: $(CRYPTOOUTPUT)
	$(eval NODE = ${patsubst build/nodes/%/msp,%,${@}})
	@mkdir -p ${@D}
	cp -R $(mspmap.${NODE})/* ${@D}

.PRECIOUS: build/bin/%
build/bin/%:
	@mkdir -p ${@D}
	GOBIN=$(abspath ${@D}) go install $(pkgmap.${@F})

build/nodes/orderer: build/nodes/orderer/orderer.yaml
build/nodes/orderer: build/nodes/orderer/genesis.block
build/nodes/cli: $(CHANNEL_TXNS)

build/nodes/ca: build/nodes/ca/fabric-ca-server-config.yaml
	@mkdir -p $@
	cp $(CA_PATH)/*_sk $@/ca.key
	cp $(CA_PATH)/*.pem $@/ca.crt

build/nodes/tlsca: build/nodes/tlsca/fabric-tlsca-server-config.yaml
	@mkdir -p $@
	cp $(TLSCA_PATH)/*_sk $@/ca.key
	cp $(TLSCA_PATH)/*.pem $@/ca.crt
	mv $@/fabric-tlsca-server-config.yaml $@/fabric-ca-server-config.yaml

build/nodes/%: build/nodes/%/msp build/nodes/%/configtx.yaml build/nodes/%/core.yaml
	@echo "Built $@"

clean: compose-down
	rm -rf build
	rm $(CHANNEL_NAME).block
