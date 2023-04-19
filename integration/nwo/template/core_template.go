/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package template

const DefaultCore = `---
logging:
  format: '%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'

peer:
  id: {{ Peer.ID }}
  networkId: {{ .NetworkID }}
  address: 127.0.0.1:{{ .PeerPort Peer "Listen" }}
  addressAutoDetect: true
  listenAddress: 127.0.0.1:{{ .PeerPort Peer "Listen" }}
  chaincodeListenAddress: 0.0.0.0:{{ .PeerPort Peer "Chaincode" }}
  keepalive:
    minInterval: 60s
    client:
      interval: 60s
      timeout: 20s
    deliveryClient:
      interval: 60s
      timeout: 20s
  gossip:
    bootstrap: 127.0.0.1:{{ .PeerPort Peer "Listen" }}
    endpoint: 127.0.0.1:{{ .PeerPort Peer "Listen" }}
    externalEndpoint: 127.0.0.1:{{ .PeerPort Peer "Listen" }}
    useLeaderElection: false
    orgLeader: true
    membershipTrackerInterval: 5s
    maxBlockCountToStore: 10
    maxPropagationBurstLatency: 10ms
    maxPropagationBurstSize: 10
    propagateIterations: 1
    propagatePeerNum: 3
    pullInterval: 4s
    pullPeerNum: 3
    requestStateInfoInterval: 4s
    publishStateInfoInterval: 4s
    stateInfoRetentionInterval:
    publishCertPeriod: 10s
    dialTimeout: 3s
    connTimeout: 2s
    recvBuffSize: 20
    sendBuffSize: 200
    digestWaitTime: 1s
    requestWaitTime: 1500ms
    responseWaitTime: 2s
    aliveTimeInterval: 5s
    aliveExpirationTimeout: 25s
    reconnectInterval: 25s
    election:
      startupGracePeriod: 15s
      membershipSampleInterval: 1s
      leaderAliveThreshold: 10s
      leaderElectionDuration: 5s
    pvtData:
      pullRetryThreshold: 7s
      transientstoreMaxBlockRetention: 1000
      pushAckTimeout: 3s
      btlPullMargin: 10
      reconcileBatchSize: 10
      reconcileSleepInterval: 5s
      reconciliationEnabled: true
      skipPullingInvalidTransactionsDuringCommit: false
      implicitCollectionDisseminationPolicy:
        requiredPeerCount: 0
        maxPeerCount: 1
    state:
       enabled: false
       checkInterval: 10s
       responseTimeout: 3s
       batchSize: 10
       blockBufferSize: 20
       maxRetries: 3
  events:
    address: 127.0.0.1:{{ .PeerPort Peer "Events" }}
    buffersize: 100
    timeout: 10ms
    timewindow: 15m
    keepalive:
      minInterval: 60s
  tls:
    enabled: {{ .TLSEnabled }}
    clientAuthRequired: {{ .ClientAuthRequired }}
    cert:
      file: {{ .PeerLocalTLSDir Peer }}/server.crt
    key:
      file: {{ .PeerLocalTLSDir Peer }}/server.key
    clientCert:
      file: {{ .PeerLocalTLSDir Peer }}/server.crt
    clientKey:
      file: {{ .PeerLocalTLSDir Peer }}/server.key
    rootcert:
      file: {{ .PeerLocalTLSDir Peer }}/ca.crt
    clientRootCAs:
      files:
      - {{ .PeerLocalTLSDir Peer }}/ca.crt
  authentication:
    timewindow: 15m
  fileSystemPath: filesystem
  BCCSP:
    Default: SW
    SW:
      Hash: SHA2
      Security: 256
      FileKeyStore:
        KeyStore:
  mspConfigPath: {{ .PeerLocalMSPDir Peer }}
  localMspId: {{ (.Organization Peer.Organization).MSPID }}
  deliveryclient:
    reconnectTotalTimeThreshold: 3600s
  localMspType: bccsp
  profile:
    enabled:     false
    listenAddress: 127.0.0.1:{{ .PeerPort Peer "ProfilePort" }}
  handlers:
    authFilters:
    - name: DefaultAuth
    - name: ExpirationCheck
    decorators:
    - name: DefaultDecorator
    endorsers:
      escc:
        name: DefaultEndorsement
    validators:
      vscc:
        name: DefaultValidation
  validatorPoolSize:
  discovery:
    enabled: true
    authCacheEnabled: true
    authCacheMaxSize: 1000
    authCachePurgeRetentionRatio: 0.75
    orgMembersAllowedAccess: false
  limits:
    concurrency:
      endorserService: 100
      deliverService: 100
  gateway:
    enabled: {{ .GatewayEnabled }}

vm:
  endpoint: unix:///var/run/docker.sock
  docker:
    tls:
      enabled: false
      ca:
        file: docker/ca.crt
      cert:
        file: docker/tls.crt
      key:
        file: docker/tls.key
    attachStdout: true
    hostConfig:
      NetworkMode: host
      LogConfig:
        Type: json-file
        Config:
          max-size: "50m"
          max-file: "5"
      Memory: 2147483648

chaincode:
  builder: $(DOCKER_NS)/fabric-ccenv:$(PROJECT_VERSION)
  pull: false
  golang:
    runtime: $(DOCKER_NS)/fabric-baseos:$(PROJECT_VERSION)
    dynamicLink: false
  java:
    runtime: $(DOCKER_NS)/fabric-javaenv:latest
  node:
    runtime: $(DOCKER_NS)/fabric-nodeenv:latest
  installTimeout: 300s
  startuptimeout: 300s
  executetimeout: 30s
  mode: net
  keepalive: 0
  system:
    _lifecycle: enable
    cscc:       enable
    lscc:       enable
    qscc:       enable
  logging:
    level:  info
    shim:   warning
    format: '%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'
  externalBuilders: {{ range .ExternalBuilders }}
    - path: {{ .Path }}
      name: {{ .Name }}
      propagateEnvironment: {{ range .PropagateEnvironment }}
         - {{ . }}
      {{- end }}
  {{- end }}

ledger:
  blockchain:
  state:
    stateDatabase: goleveldb
    couchDBConfig:
      couchDBAddress: 127.0.0.1:5984
      username:
      password:
      maxRetries: 3
      maxRetriesOnStartup: 10
      requestTimeout: 35s
      queryLimit: 10000
      maxBatchUpdateSize: 1000
  history:
    enableHistoryDatabase: true
  pvtdataStore:
    deprioritizedDataReconcilerInterval: 60m
    purgeInterval: 1

operations:
  listenAddress: 127.0.0.1:{{ .PeerPort Peer "Operations" }}
  tls:
    enabled: {{ .TLSEnabled }}
    cert:
      file: {{ .PeerLocalTLSDir Peer }}/server.crt
    key:
      file: {{ .PeerLocalTLSDir Peer }}/server.key
    clientAuthRequired: {{ .ClientAuthRequired }}
    clientRootCAs:
      files:
      - {{ .PeerLocalTLSDir Peer }}/ca.crt
metrics:
  provider: {{ .MetricsProvider }}
  statsd:
    {{- if .StatsdEndpoint }}
    network: tcp
    address: {{ .StatsdEndpoint }}
    {{- else }}
    network: udp
    address: 127.0.0.1:8125
    {{- end }}
    writeInterval: 5s
    prefix: {{ ReplaceAll (ToLower Peer.ID) "." "_" }}
`
