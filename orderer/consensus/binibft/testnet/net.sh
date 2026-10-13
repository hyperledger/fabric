#!/usr/bin/env bash
# 4-orderer BiniBFT test network on localhost.
#   ./net.sh gen            generate crypto material and the channel genesis block
#   ./net.sh up [binary]    start the 4 orderers and join them to the channel
#   ./net.sh stop N         stop orderer N (simulate a crash)
#   ./net.sh start N        restart orderer N (it must re-join: state is on disk)
#   ./net.sh down           stop everything
#   ./net.sh clean          remove ledgers, logs and generated material
set -euo pipefail

NET="$(cd "$(dirname "$0")" && pwd)"
BIN="$HOME/bin"
ORDERER_BIN="${ORDERER_BIN:-$BIN/orderer-binibft}"
CHANNEL="${CHANNEL:-mychannel}"
O="$NET/crypto-config/ordererOrganizations/example.com"
export PATH="$HOME/opt/go/bin:$BIN:$PATH"

orderer_env() { # $1 = node number
  local i=$1 d="$O/orderers/orderer$1.example.com"
  export FABRIC_CFG_PATH="$HOME/dev/fabric/sampleconfig"
  export ORDERER_GENERAL_LISTENADDRESS=127.0.0.1
  export ORDERER_GENERAL_LISTENPORT=$((7049 + i))
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
  export ORDERER_ADMIN_LISTENADDRESS="127.0.0.1:$((7949 + i))"
  export ORDERER_ADMIN_TLS_ENABLED=false
  export ORDERER_FILELEDGER_LOCATION="$NET/data/orderer$i/ledger"
  export ORDERER_CONSENSUS_WALDIR="$NET/data/orderer$i/wal"
  export ORDERER_CONSENSUS_SNAPDIR="$NET/data/orderer$i/snap"
  export ORDERER_OPERATIONS_LISTENADDRESS="127.0.0.1:$((8439 + i))"
  export ORDERER_METRICS_PROVIDER=disabled
  export FABRIC_LOGGING_SPEC="${FABRIC_LOGGING_SPEC:-info}"
}

start_orderer() {
  local i=$1
  mkdir -p "$NET/logs" "$NET/data/orderer$i"
  ( orderer_env "$i"
    nohup setsid "$ORDERER_BIN" >> "$NET/logs/orderer$i.log" 2>&1 < /dev/null &
    echo $! > "$NET/data/orderer$i/pid" )
  echo "orderer$i started (pid $(cat "$NET/data/orderer$i/pid"), port $((7049 + i)))"
}

join() {
  local i=$1
  "$BIN/osnadmin" channel join --channelID "$CHANNEL" --config-block "$NET/genesis.block" \
    -o "127.0.0.1:$((7949 + i))" 2>&1 | tr -d '\n'; echo
}

case "${1:-}" in
  gen)
    cd "$NET"
    rm -f genesis.block
    # Never regenerate the crypto material under running orderers: they would keep
    # the old certificates in memory and every TLS handshake would fail.
    if [ ! -d crypto-config ]; then
      cryptogen generate --config=crypto-config.yaml --output=crypto-config
    fi
    FABRIC_CFG_PATH="$NET" configtxgen -profile BiniBFTChannel -channelID "$CHANNEL" -outputBlock genesis.block
    echo "generated: $NET/genesis.block"
    ;;
  up)
    for i in 1 2 3 4; do start_orderer $i; done
    sleep 4
    for i in 1 2 3 4; do join $i; done
    ;;
  start)  start_orderer "$2" ;;
  stop)
    kill "$(cat "$NET/data/orderer$2/pid")" && echo "orderer$2 stopped"
    ;;
  down)
    for i in 1 2 3 4; do
      [ -f "$NET/data/orderer$i/pid" ] && kill "$(cat "$NET/data/orderer$i/pid")" 2>/dev/null || true
    done
    echo "all orderers stopped"
    ;;
  clean)
    bash "$0" down || true
    rm -rf "$NET/data" "$NET/logs"
    ;;
  *) echo "usage: $0 gen|up|start N|stop N|down|clean"; exit 1 ;;
esac
