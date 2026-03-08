/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package template

const DefaultOrderer = `---
{{ with $w := . -}}
General:
  ListenAddress: 127.0.0.1
  ListenPort: {{ .OrdererPort Orderer "Listen" }}
  Throttling:
   # Rate is the maximum rate for all clients combined.
    Rate: 0
   # InactivityTimeout defines the time frame after which
   # inactive clients are pruned from memory and are not considered
   # when allocating the budget for throttling per client.
    InactivityTimeout: 5s
  TLS:
    Enabled: {{ .TLSEnabled }}
    PrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    Certificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    RootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
    ClientAuthRequired: {{ $w.ClientAuthRequired }}
    ClientRootCAs:
  Cluster:
    ClientCertificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    ClientPrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    ServerCertificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    ServerPrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    ReplicationPolicy: {{ .OrdererReplicationPolicy }}
    DialTimeout: 5s
    RPCTimeout: 7s
    ReplicationBufferSize: 20971520
    ReplicationPullTimeout: 5s
    ReplicationRetryTimeout: 5s
    ListenAddress: 127.0.0.1
    ListenPort: {{ .OrdererPort Orderer "Cluster" }}
  Keepalive:
    ServerMinInterval: 60s
    ServerInterval: 7200s
    ServerTimeout: 20s
  Backoff:
    BaseDelay: 1s
    Multiplier: 1.6
    MaxDelay: 2m
  BootstrapMethod: "none"
  LocalMSPDir: {{ $w.OrdererLocalMSPDir Orderer }}
  LocalMSPID: {{ ($w.Organization Orderer.Organization).MSPID }}
  Profile:
    Enabled: false
    Address: 127.0.0.1:{{ .OrdererPort Orderer "Profile" }}
  BCCSP:
    Default: SW
    SW:
      Hash: SHA2
      Security: 256
      FileKeyStore:
        KeyStore:
  Authentication:
    TimeWindow: 15m
FileLedger:
  Location: {{ .OrdererDir Orderer }}/system
Debug:
  BroadcastTraceDir:
  DeliverTraceDir:
Consensus:
  WALDir: {{ .OrdererDir Orderer }}/etcdraft/wal
  SnapDir: {{ .OrdererDir Orderer }}/etcdraft/snapshot
  EvictionSuspicion: 5s
Operations:
  ListenAddress: 127.0.0.1:{{ .OrdererPort Orderer "Operations" }}
  TLS:
    Enabled: {{ .TLSEnabled }}
    PrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    Certificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    RootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
    ClientAuthRequired: {{ $w.ClientAuthRequired }}
    ClientRootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
Metrics:
  Provider: {{ .MetricsProvider }}
  Statsd:
    {{- if .StatsdEndpoint }}
    Network: tcp
    Address: {{ .StatsdEndpoint }}
    {{- else }}
    Network: udp
    Address: 127.0.0.1:8125
    {{- end }}
    WriteInterval: 5s
    Prefix: {{ ReplaceAll (ToLower Orderer.ID) "." "_" }}
Admin:
  ListenAddress: 127.0.0.1:{{ .OrdererPort Orderer "Admin" }}
  TLS:
    Enabled: {{ .TLSEnabled }}
    PrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    Certificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    RootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
    ClientAuthRequired: true
    ClientRootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
{{- end }}
ChannelParticipation:
  Enabled: true
  MaxRequestBodySize: 1 MB
`
