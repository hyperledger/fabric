#!/usr/bin/env bash
# Full Hyperledger Fabric network ordered by BiniBFT (no SmartBFT), on localhost.
#
#   N orderers (odd, default 5)  - every one a BiniBFT consenter
#   2 peer organisations         - peer0.org1, peer0.org2 (endorse + validate + commit)
#   1 chaincode ("simple")       - chaincode-as-a-service, one server per peer
#
#   bash net.sh build       build every binary needed (orderer with BiniBFT, peer, tools,
#                           ccaas builder, chaincode server, test client, e2e client) into $BIN
#   N=5 bash net.sh gen     crypto material, channel config, genesis block
#   bash net.sh up          start orderers + peers, join everybody to the channel
#   bash net.sh deploy      lifecycle: package, install, approve (both orgs), commit, init
#   bash net.sh stop-orderer K / start-orderer K
#   bash net.sh down        stop every process
#   bash net.sh clean       down + delete ledgers, logs and generated material
set -euo pipefail

NET="$(cd "$(dirname "$0")" && pwd)"
FABRIC="$(cd "$NET/../../../.." && pwd)"
BIN="${BIN:-$HOME/bin}"
export BIN
ORDERER_BIN="${ORDERER_BIN:-$BIN/orderer-binibft}"
CHANNEL="${CHANNEL:-mychannel}"
CC_NAME=simple
export PATH="$HOME/opt/go/bin:$BIN:$PATH"

if [ -f "$NET/.n" ]; then N=$(cat "$NET/.n"); else N="${N:-5}"; fi

O="$NET/crypto-config/ordererOrganizations/example.com"
ORDERER_CA="$O/orderers/orderer1.example.com/tls/ca.crt"

o_port()   { echo $((7150 + $1)); }   # orderer client + cluster
o_admin()  { echo $((9150 + $1)); }   # osnadmin
o_ops()    { echo $((9250 + $1)); }
p_port()   { echo $((7301 + 10 * $1)); }  # org1 -> 7311, org2 -> 7321
p_ccport() { echo $((7302 + 10 * $1)); }
p_ops()    { echo $((9301 + 10 * $1)); }
cc_port()  { echo $((9901 + 10 * $1)); }  # chaincode server: org1 9911, org2 9921

pem_json() { awk '{printf "%s\\n", $0}' "$1"; }   # PEM file -> one JSON string body
peerdir()  { echo "$NET/crypto-config/peerOrganizations/org$1.example.com"; }

as_org() { # $1 = org number, $2 = Admin | User1
  local k=$1 u=${2:-Admin} d; d=$(peerdir "$k")
  export FABRIC_CFG_PATH="$NET/config"
  export CORE_PEER_LOCALMSPID="Org${k}MSP"
  export CORE_PEER_MSPCONFIGPATH="$d/users/$u@org$k.example.com/msp"
  export CORE_PEER_ADDRESS="127.0.0.1:$(p_port "$k")"
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_TLS_ROOTCERT_FILE="$d/peers/peer0.org$k.example.com/tls/ca.crt"
}
peer_tls_ca() { echo "$(peerdir "$1")/peers/peer0.org$1.example.com/tls/ca.crt"; }

gen_crypto_config() {
  {
    echo "OrdererOrgs:"
    echo "  - Name: Orderer"
    echo "    Domain: example.com"
    echo "    EnableNodeOUs: false"
    echo "    Specs:"
    for i in $(seq 1 "$N"); do
      echo "      - Hostname: orderer$i"
      echo "        SANS: [localhost, 127.0.0.1]"
    done
    echo "PeerOrgs:"
    for k in 1 2; do
      echo "  - Name: Org$k"
      echo "    Domain: org$k.example.com"
      echo "    EnableNodeOUs: true"
      echo "    Template: {Count: 1, SANS: [localhost, 127.0.0.1]}"
      echo "    Users: {Count: 1}"
    done
  } > "$NET/crypto-config.yaml"
}

gen_configtx() {
  local c="crypto-config/ordererOrganizations/example.com/orderers"
  {
    cat <<EOF
Organizations:
  - &OrdererOrg
    Name: OrdererOrg
    ID: OrdererMSP
    MSPDir: crypto-config/ordererOrganizations/example.com/msp
    Policies:
      Readers: {Type: Signature, Rule: "OR('OrdererMSP.member')"}
      Writers: {Type: Signature, Rule: "OR('OrdererMSP.member')"}
      Admins:  {Type: Signature, Rule: "OR('OrdererMSP.admin')"}
    OrdererEndpoints:
EOF
    for i in $(seq 1 "$N"); do echo "      - 127.0.0.1:$(o_port "$i")"; done
    for k in 1 2; do
      cat <<EOF
  - &Org$k
    Name: Org${k}MSP
    ID: Org${k}MSP
    MSPDir: crypto-config/peerOrganizations/org$k.example.com/msp
    Policies:
      Readers:     {Type: Signature, Rule: "OR('Org${k}MSP.admin', 'Org${k}MSP.peer', 'Org${k}MSP.client')"}
      Writers:     {Type: Signature, Rule: "OR('Org${k}MSP.admin', 'Org${k}MSP.client')"}
      Admins:      {Type: Signature, Rule: "OR('Org${k}MSP.admin')"}
      Endorsement: {Type: Signature, Rule: "OR('Org${k}MSP.peer')"}
    AnchorPeers:
      - {Host: 127.0.0.1, Port: $(p_port "$k")}
EOF
    done
    cat <<EOF

Capabilities:
  Channel:     &ChannelCapabilities     {V3_0: true}
  Orderer:     &OrdererCapabilities     {V2_0: true}
  # peers of Fabric v3.1 support Application capabilities up to V2_5
  Application: &ApplicationCapabilities {V2_0: true, V2_5: true}

Application: &ApplicationDefaults
  Organizations:
  Policies:
    LifecycleEndorsement: {Type: ImplicitMeta, Rule: "MAJORITY Endorsement"}
    Endorsement:          {Type: ImplicitMeta, Rule: "MAJORITY Endorsement"}
    Readers:              {Type: ImplicitMeta, Rule: "ANY Readers"}
    Writers:              {Type: ImplicitMeta, Rule: "ANY Writers"}
    Admins:               {Type: ImplicitMeta, Rule: "MAJORITY Admins"}
  Capabilities:
    <<: *ApplicationCapabilities

Orderer: &OrdererDefaults
  # "BFT" is served by BiniBFT in this orderer build (it replaces SmartBFT).
  OrdererType: BFT
  BatchTimeout: 500ms
  BatchSize: {MaxMessageCount: 20, AbsoluteMaxBytes: 10 MB, PreferredMaxBytes: 2 MB}
  # Required by Fabric's config schema for the BFT type; BiniBFT ignores them.
  SmartBFT:
    RequestBatchMaxInterval: 200ms
    RequestForwardTimeout: 5s
    RequestComplainTimeout: 20s
    RequestAutoRemoveTimeout: 3m0s
    ViewChangeResendInterval: 5s
    ViewChangeTimeout: 20s
    LeaderHeartbeatTimeout: 1m0s
    CollectTimeout: 1s
    IncomingMessageBufferSize: 200
    RequestPoolSize: 100000
    LeaderHeartbeatCount: 10
  ConsenterMapping:
EOF
    for i in $(seq 1 "$N"); do
      cat <<EOF
    - ID: $i
      Host: 127.0.0.1
      Port: $(o_port "$i")
      MSPID: OrdererMSP
      Identity: $c/orderer$i.example.com/msp/signcerts/orderer$i.example.com-cert.pem
      ClientTLSCert: $c/orderer$i.example.com/tls/server.crt
      ServerTLSCert: $c/orderer$i.example.com/tls/server.crt
EOF
    done
    cat <<EOF
  Organizations:
  Policies:
    Readers:         {Type: ImplicitMeta, Rule: "ANY Readers"}
    Writers:         {Type: ImplicitMeta, Rule: "ANY Writers"}
    Admins:          {Type: ImplicitMeta, Rule: "MAJORITY Admins"}
    BlockValidation: {Type: ImplicitMeta, Rule: "ANY Writers"}
  Capabilities:
    <<: *OrdererCapabilities

Channel: &ChannelDefaults
  Policies:
    Readers: {Type: ImplicitMeta, Rule: "ANY Readers"}
    Writers: {Type: ImplicitMeta, Rule: "ANY Writers"}
    Admins:  {Type: ImplicitMeta, Rule: "MAJORITY Admins"}
  Capabilities:
    <<: *ChannelCapabilities

Profiles:
  BiniBFTFullChannel:
    <<: *ChannelDefaults
    Orderer:
      <<: *OrdererDefaults
      Organizations: [*OrdererOrg]
    Application:
      <<: *ApplicationDefaults
      Organizations: [*Org1, *Org2]
EOF
  } > "$NET/configtx.yaml"
}

gen_core_yaml() { # stock core.yaml with the ccaas builder path and Docker disabled
  mkdir -p "$NET/config"
  sed -e "s#/opt/hyperledger/ccaas_builder#$BIN/ccaas_builder#" \
      -e "s#endpoint: unix:///var/run/docker.sock#endpoint: \"\"#" \
      "$FABRIC/sampleconfig/core.yaml" > "$NET/config/core.yaml"
}

spawn() { # $1 = name, rest = command (env already exported in a subshell)
  local name=$1; shift
  mkdir -p "$NET/logs" "$NET/data/pids"
  nohup setsid "$@" >> "$NET/logs/$name.log" 2>&1 < /dev/null &
  echo $! > "$NET/data/pids/$name"
}
stop_proc() { local f="$NET/data/pids/$1"; [ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null && rm -f "$f"; return 0; }

start_orderer() {
  local i=$1 d="$O/orderers/orderer$1.example.com"
  ( export FABRIC_CFG_PATH="$FABRIC/sampleconfig"
    export ORDERER_GENERAL_LISTENADDRESS=127.0.0.1
    export ORDERER_GENERAL_LISTENPORT="$(o_port "$i")"
    export ORDERER_GENERAL_LOCALMSPID=OrdererMSP
    export ORDERER_GENERAL_LOCALMSPDIR="$d/msp"
    export ORDERER_GENERAL_TLS_ENABLED=true
    export ORDERER_GENERAL_TLS_PRIVATEKEY="$d/tls/server.key"
    export ORDERER_GENERAL_TLS_CERTIFICATE="$d/tls/server.crt"
    export ORDERER_GENERAL_TLS_ROOTCAS="[$d/tls/ca.crt]"
    export ORDERER_GENERAL_CLUSTER_CLIENTCERTIFICATE="$d/tls/server.crt"
    export ORDERER_GENERAL_CLUSTER_CLIENTPRIVATEKEY="$d/tls/server.key"
    export ORDERER_GENERAL_CLUSTER_ROOTCAS="[$d/tls/ca.crt]"
    export ORDERER_GENERAL_BOOTSTRAPMETHOD=none
    export ORDERER_CHANNELPARTICIPATION_ENABLED=true
    export ORDERER_ADMIN_LISTENADDRESS="127.0.0.1:$(o_admin "$i")"
    export ORDERER_ADMIN_TLS_ENABLED=false
    export ORDERER_FILELEDGER_LOCATION="$NET/data/orderer$i/ledger"
    export ORDERER_CONSENSUS_WALDIR="$NET/data/orderer$i/wal"
    export ORDERER_CONSENSUS_SNAPDIR="$NET/data/orderer$i/snap"
    export ORDERER_OPERATIONS_LISTENADDRESS="127.0.0.1:$(o_ops "$i")"
    export ORDERER_METRICS_PROVIDER=disabled
    export FABRIC_LOGGING_SPEC="${FABRIC_LOGGING_SPEC:-info}"
    spawn "orderer$i" "$ORDERER_BIN" )
  echo "orderer$i started on 127.0.0.1:$(o_port "$i")"
}

start_peer() {
  local k=$1 d; d="$(peerdir "$1")/peers/peer0.org$1.example.com"
  ( export FABRIC_CFG_PATH="$NET/config"
    export CORE_PEER_ID="peer0.org$k.example.com"
    export CORE_PEER_ADDRESS="127.0.0.1:$(p_port "$k")"
    export CORE_PEER_LISTENADDRESS="127.0.0.1:$(p_port "$k")"
    export CORE_PEER_CHAINCODEADDRESS="127.0.0.1:$(p_ccport "$k")"
    export CORE_PEER_CHAINCODELISTENADDRESS="127.0.0.1:$(p_ccport "$k")"
    export CORE_PEER_GOSSIP_BOOTSTRAP="127.0.0.1:$(p_port "$k")"
    export CORE_PEER_GOSSIP_EXTERNALENDPOINT="127.0.0.1:$(p_port "$k")"
    export CORE_PEER_LOCALMSPID="Org${k}MSP"
    export CORE_PEER_MSPCONFIGPATH="$d/msp"
    export CORE_PEER_TLS_ENABLED=true
    export CORE_PEER_TLS_CERT_FILE="$d/tls/server.crt"
    export CORE_PEER_TLS_KEY_FILE="$d/tls/server.key"
    export CORE_PEER_TLS_ROOTCERT_FILE="$d/tls/ca.crt"
    export CORE_PEER_FILESYSTEMPATH="$NET/data/peer0.org$k"
    export CORE_LEDGER_SNAPSHOTS_ROOTDIR="$NET/data/peer0.org$k/snapshots"
    export CORE_OPERATIONS_LISTENADDRESS="127.0.0.1:$(p_ops "$k")"
    export CORE_METRICS_PROVIDER=disabled
    # the ccaas builder fills {{.ccport}} in connection.json: each peer talks to its own server
    export CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG="{\"ccport\":\"$(cc_port "$k")\"}"
    export FABRIC_LOGGING_SPEC="${FABRIC_LOGGING_SPEC:-info}"
    spawn "peer0.org$k" "$BIN/peer" node start )
  echo "peer0.org$k started on 127.0.0.1:$(p_port "$k")"
}

start_cc_server() { # $1 = org, $2 = package id
  local k=$1 dir="$NET/data/cc-org$1"
  mkdir -p "$dir"
  # the chaincode shim serves mutual TLS only; demo: one self-signed cert for both ends
  echo "{\"listen_address\":\"127.0.0.1:$(cc_port "$k")\",\"key\":\"$(pem_json "$NET/data/cc-tls/key.pem")\",\"cert\":\"$(pem_json "$NET/data/cc-tls/cert.pem")\",\"ca\":\"$(pem_json "$NET/data/cc-tls/cert.pem")\"}" > "$dir/config.json"
  ( cd "$dir" && spawn "cc-org$k" "$BIN/simple-cc-server" "$2" )
  echo "chaincode server for org$k started on 127.0.0.1:$(cc_port "$k")"
}

both_peers() {
  echo "--peerAddresses 127.0.0.1:$(p_port 1) --tlsRootCertFiles $(peer_tls_ca 1)" \
       "--peerAddresses 127.0.0.1:$(p_port 2) --tlsRootCertFiles $(peer_tls_ca 2)"
}

deploy() {
  local pkg="$NET/data/pkg" ord="127.0.0.1:$(o_port 1)"
  rm -rf "$pkg" "$NET/data/cc-tls"; mkdir -p "$pkg/src" "$NET/data/cc-tls"
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 365 \
    -keyout "$NET/data/cc-tls/key.pem" -out "$NET/data/cc-tls/cert.pem" \
    -subj "/CN=chaincode" -addext "subjectAltName=IP:127.0.0.1" 2>/dev/null
  # chaincode-as-a-service package: only the address, the code runs as its own process
  echo "{\"address\":\"127.0.0.1:{{.ccport}}\",\"dial_timeout\":\"10s\",\"tls_required\":true,\"client_auth_required\":true,\"root_cert\":\"$(pem_json "$NET/data/cc-tls/cert.pem")\",\"client_cert\":\"$(pem_json "$NET/data/cc-tls/cert.pem")\",\"client_key\":\"$(pem_json "$NET/data/cc-tls/key.pem")\"}" > "$pkg/src/connection.json"
  echo "{\"type\":\"ccaas\",\"label\":\"${CC_NAME}_1.0\"}" > "$pkg/metadata.json"
  tar -C "$pkg/src" -czf "$pkg/code.tar.gz" connection.json
  tar -C "$pkg" -czf "$pkg/$CC_NAME.tar.gz" metadata.json code.tar.gz

  local id; id=$(as_org 1; peer lifecycle chaincode calculatepackageid "$pkg/$CC_NAME.tar.gz")
  echo "package id: $id"

  for k in 1 2; do
    echo "== org$k: install"
    ( as_org "$k"; peer lifecycle chaincode install "$pkg/$CC_NAME.tar.gz" 2>&1 | tail -1 )
    start_cc_server "$k" "$id"
  done
  sleep 2
  for k in 1 2; do
    echo "== org$k: approve (an ordered transaction)"
    ( as_org "$k"; peer lifecycle chaincode approveformyorg -o "$ord" --tls --cafile "$ORDERER_CA" \
        --channelID "$CHANNEL" --name "$CC_NAME" --version 1.0 --sequence 1 --init-required \
        --package-id "$id" 2>&1 | tail -1 )
  done
  ( as_org 1; peer lifecycle chaincode checkcommitreadiness --channelID "$CHANNEL" --name "$CC_NAME" \
      --version 1.0 --sequence 1 --init-required --output json )
  echo "== commit chaincode definition (an ordered transaction)"
  # shellcheck disable=SC2046
  ( as_org 1; peer lifecycle chaincode commit -o "$ord" --tls --cafile "$ORDERER_CA" \
      --channelID "$CHANNEL" --name "$CC_NAME" --version 1.0 --sequence 1 --init-required \
      $(both_peers) 2>&1 | tail -2 )
  echo "== init ledger: a=100, b=200"
  # shellcheck disable=SC2046
  ( as_org 1; peer chaincode invoke -o "$ord" --tls --cafile "$ORDERER_CA" -C "$CHANNEL" -n "$CC_NAME" \
      --isInit -c '{"Args":["init","a","100","b","200"]}' $(both_peers) --waitForEvent 2>&1 | tail -2 )
}

build_all() {
  set -e
  mkdir -p "$BIN/ccaas_builder/bin"
  echo "building into $BIN (first build takes a few minutes)"
  ( cd "$FABRIC"
    go build -o "$BIN/orderer-binibft" ./cmd/orderer
    for t in peer cryptogen configtxgen osnadmin; do go build -o "$BIN/$t" "./cmd/$t"; done
    go build -o "$BIN/binibft-testclient" ./orderer/consensus/binibft/testclient
    go build -o "$BIN/simple-cc-server" ./integration/chaincode/server )
  ( cd "$FABRIC/ccaas_builder" && for t in detect build release; do go build -o "$BIN/ccaas_builder/bin/$t" "./cmd/$t"; done )
  ( cd "$NET/client" && go build -o "$BIN/binibft-e2e-client" . )
  echo "done: $(ls "$BIN" | tr '
' ' ')"
}

case "${1:-}" in
  build) build_all ;;
  gen)
    if [ $((N % 2)) -eq 0 ]; then echo "N must be odd (got $N)"; exit 1; fi
    cd "$NET"; rm -rf genesis.block crypto-config data logs
    echo "$N" > .n
    gen_crypto_config; gen_configtx; gen_core_yaml
    cryptogen generate --config=crypto-config.yaml --output=crypto-config >/dev/null
    FABRIC_CFG_PATH="$NET" configtxgen -profile BiniBFTFullChannel -channelID "$CHANNEL" \
      -outputBlock genesis.block 2>&1 | tail -1
    echo "generated a network with $N BiniBFT orderers and 2 peer orgs"
    ;;
  up)
    for i in $(seq 1 "$N"); do start_orderer "$i"; done
    sleep 4
    for i in $(seq 1 "$N"); do
      echo -n "orderer$i join: "
      osnadmin channel join --channelID "$CHANNEL" --config-block "$NET/genesis.block" \
        -o "127.0.0.1:$(o_admin "$i")" 2>&1 | tr -d '\n\t' | grep -o '"consensusRelation": *"[a-z]*", *"status": *"[a-z]*"' || echo "FAILED"
    done
    for k in 1 2; do start_peer "$k"; done
    sleep 4
    for k in 1 2; do
      echo -n "peer0.org$k join: "
      ( as_org "$k"; peer channel join -b "$NET/genesis.block" 2>&1 | tail -1 )
    done
    ;;
  deploy) deploy ;;
  stop-orderer)  stop_proc "orderer$2"; echo "orderer$2 stopped" ;;
  start-orderer) start_orderer "$2" ;;
  down)
    for f in "$NET"/data/pids/*; do [ -f "$f" ] && stop_proc "$(basename "$f")"; done
    echo "all processes stopped"
    ;;
  clean)
    bash "$0" down || true
    rm -rf "$NET/data" "$NET/logs" "$NET/crypto-config" "$NET/genesis.block" "$NET/config" "$NET/.n"
    ;;
  env) # usage: source <(bash net.sh env 1 User1)
    declare -f as_org peerdir p_port o_port peer_tls_ca both_peers
    echo "NET='$NET' N=$N CHANNEL=$CHANNEL ORDERER_CA='$ORDERER_CA'"
    ;;
  *) echo "usage: $0 build | N=5 $0 gen | up | deploy | stop-orderer K | start-orderer K | down | clean"; exit 1 ;;
esac
