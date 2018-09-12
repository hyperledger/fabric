/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

const DefaultOrdererTemplate = `---
{{ with $w := . -}}
General:
  LedgerType: file
  ListenAddress: 127.0.0.1
  ListenPort: {{ .OrdererPort Orderer "Listen" }}
  TLS:
    Enabled: true
    PrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    Certificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    RootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
    ClientAuthRequired: false
    ClientRootCAs:
  Keepalive:
    ServerMinInterval: 60s
    ServerInterval: 7200s
    ServerTimeout: 20s
  LogLevel: info
  LogFormat: '%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'
  GenesisMethod: file
  GenesisProfile: {{ .SystemChannel.Profile }}
  GenesisFile: {{ .RootDir }}/{{ .SystemChannel.Name }}_block.pb
  SystemChannel: {{ .SystemChannel.Name }}
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
  Prefix: hyperledger-fabric-ordererledger
RAMLedger:
  HistorySize: 1000
{{ if eq .Consensus.Type "kafka" -}}
Kafka:
  Retry:
    ShortInterval: 5s
    ShortTotal: 10m
    LongInterval: 5m
    LongTotal: 12h
    NetworkTimeouts:
      DialTimeout: 10s
      ReadTimeout: 10s
      WriteTimeout: 10s
    Metadata:
      RetryBackoff: 250ms
      RetryMax: 3
    Producer:
      RetryBackoff: 100ms
      RetryMax: 3
    Consumer:
      RetryBackoff: 2s
  Topic:
    ReplicationFactor: 1
  Verbose: false
  TLS:
    Enabled: false
    PrivateKey:
    Certificate:
    RootCAs:
  SASLPlain:
    Enabled: false
    User:
    Password:
  Version:{{ end }}
Debug:
    BroadcastTraceDir:
    DeliverTraceDir:
{{- end }}
`
