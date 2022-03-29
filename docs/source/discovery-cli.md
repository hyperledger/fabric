Service Discovery CLI
=====================

The discovery service has its own Command Line Interface (CLI) which
uses a YAML configuration file to persist properties such as certificate
and private key paths, as well as MSP ID.

The `discover` command has the following subcommands:
  * saveConfig
  * peers
  * config
  * endorsers

And the usage of the command is shown below:

```
usage: discover [<flags>] <command> [<args> ...]

Command line client for fabric discovery service

Flags:
  --help                   Show context-sensitive help (also try --help-long and --help-man).
  --configFile=CONFIGFILE  Specifies the config file to load the configuration from
  --peerTLSCA=PEERTLSCA    Sets the TLS CA certificate file path that verifies the TLS peer's certificate
  --tlsCert=TLSCERT        (Optional) Sets the client TLS certificate file path that is used when the peer enforces client authentication
  --tlsKey=TLSKEY          (Optional) Sets the client TLS key file path that is used when the peer enforces client authentication
  --userKey=USERKEY        Sets the user's key file path that is used to sign messages sent to the peer
  --userCert=USERCERT      Sets the user's certificate file path that is used to authenticate the messages sent to the peer
  --MSP=MSP                Sets the MSP ID of the user, which represents the CA(s) that issued its user certificate

Commands:
  help [<command>...]
    Show help.

  peers [<flags>]
    Discover peers

  config [<flags>]
    Discover channel config

  endorsers [<flags>]
    Discover chaincode endorsers

  saveConfig
    Save the config passed by flags into the file specified by --configFile
```

Configuring external endpoints
------------------------------

For a peer to be exposed to service discovery, they need to have `peer.gossip.externalEndpoint`
configured in `core.yaml`. Otherwise, Fabric assumes the peer should not be
disclosed.

The `core.yaml` value can also be overridden using an environment variable.

```
CORE_PEER_GOSSIP_EXTERNALENDPOINT=peer1.org1.example.com:8051
```

Persisting configuration
------------------------

To persist the configuration, a config file name should be supplied via
the flag `--configFile`, along with the command `saveConfig`:

```
discover --configFile conf.yaml --peerTLSCA tls/ca.crt --userKey msp/keystore/ea4f6a38ac7057b6fa9502c2f5f39f182e320f71f667749100fe7dd94c23ce43_sk --userCert msp/signcerts/User1\@org1.example.com-cert.pem  --MSP Org1MSP saveConfig
```

By executing the above command, configuration file would be created:

```yaml
$ cat conf.yaml
version: 0
tlsconfig:
  certpath: ""
  keypath: ""
  peercacertpath: /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/tls/ca.crt
  timeout: 0s
signerconfig:
  mspid: Org1MSP
  identitypath: /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/signcerts/User1@org1.example.com-cert.pem
  keypath: /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/keystore/ea4f6a38ac7057b6fa9502c2f5f39f182e320f71f667749100fe7dd94c23ce43_sk
```

When the peer runs with TLS enabled, the discovery service on the peer
requires the client to connect to it with mutual TLS, even if the
peer has not set `tls.clientAuthRequired` to `true`.

When `tls.clientAuthRequired` is set to `false`, the peer will still
request (and verify if given, but not require) client TLS certificates.
Therefore if the client does not pass a TLS certificate,
TLS connections can be established to the peer but will be rejected in the
peer's discovery layer. To that end, the discovery CLI provides a
TLS certificate on its own if the user doesn't explicitly set one.

More concretely, when the discovery CLI's config file has a certificate path for
`peercacertpath`, but the `certpath` and `keypath` aren't configured as
in the above - the discovery CLI generates a self-signed TLS certificate
and uses this to connect to the peer.

When the `peercacertpath` isn't configured, the discovery CLI connects
without TLS , and this is highly not recommended, as the information is
sent over plaintext, un-encrypted.

Querying the discovery service
------------------------------

The discoveryCLI acts as a discovery client, and it needs to be executed
against a peer. This is done via specifying the `--server` flag. In
addition, the queries are channel-scoped, so the `--channel` flag must
be used.

The only query that doesn't require a channel is the local membership
peer query, which by default can only be used by administrators of the
peer being queried.

The discover CLI supports all server-side queries:

-   Peer membership query
-   Configuration query
-   Endorsers query

Let's go over them and see how they should be invoked and parsed:

Peer membership query:
----------------------

```
$ discover --configFile conf.yaml peers --channel mychannel  --server peer0.org1.example.com:7051
[
	{
		"MSPID": "Org2MSP",
		"LedgerHeight": 5,
		"Endpoint": "peer0.org2.example.com:9051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICKTCCAc+gAwIBAgIRANK4WBck5gKuzTxVQIwhYMUwCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzIuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzIuZXhhbXBsZS5jb20wHhcNMTgwNjE3MTM0NTIxWhcNMjgwNjE0MTM0NTIx\nWjBqMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzENMAsGA1UECxMEcGVlcjEfMB0GA1UEAxMWcGVlcjAub3Jn\nMi5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABJa0gkMRqJCi\nzmx+L9xy/ecJNvdAV2zmSx5Sf2qospVAH1MYCHyudDEvkiRuBPgmCdOdwJsE0g+h\nz0nZdKq6/X+jTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1Ud\nIwQkMCKAIFZMuZfUtY6n2iyxaVr3rl+x5lU0CdG9x7KAeYydQGTMMAoGCCqGSM49\nBAMCA0gAMEUCIQC0M9/LJ7j3I9NEPQ/B1BpnJP+UNPnGO2peVrM/mJ1nVgIgS1ZA\nA1tsxuDyllaQuHx2P+P9NDFdjXx5T08lZhxuWYM=\n-----END CERTIFICATE-----\n",
		"Chaincodes": [
			"mycc"
		]
	},
	{
		"MSPID": "Org2MSP",
		"LedgerHeight": 5,
		"Endpoint": "peer1.org2.example.com:10051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICKDCCAc+gAwIBAgIRALnNJzplCrYy4Y8CjZtqL7AwCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzIuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzIuZXhhbXBsZS5jb20wHhcNMTgwNjE3MTM0NTIxWhcNMjgwNjE0MTM0NTIx\nWjBqMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzENMAsGA1UECxMEcGVlcjEfMB0GA1UEAxMWcGVlcjEub3Jn\nMi5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABNDopAkHlDdu\nq10HEkdxvdpkbs7EJyqv1clvCt/YMn1hS6sM+bFDgkJKalG7s9Hg3URF0aGpy51R\nU+4F9Muo+XajTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1Ud\nIwQkMCKAIFZMuZfUtY6n2iyxaVr3rl+x5lU0CdG9x7KAeYydQGTMMAoGCCqGSM49\nBAMCA0cAMEQCIAR4fBmIBKW2jp0HbbabVepNtl1c7+6++riIrEBnoyIVAiBBvWmI\nyG02c5hu4wPAuVQMB7AU6tGSeYaWSAAo/ExunQ==\n-----END CERTIFICATE-----\n",
		"Chaincodes": [
			"mycc"
		]
	},
	{
		"MSPID": "Org1MSP",
		"LedgerHeight": 5,
		"Endpoint": "peer0.org1.example.com:7051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICKDCCAc6gAwIBAgIQP18LeXtEXGoN8pTqzXTHZTAKBggqhkjOPQQDAjBzMQsw\nCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy\nYW5jaXNjbzEZMBcGA1UEChMQb3JnMS5leGFtcGxlLmNvbTEcMBoGA1UEAxMTY2Eu\nb3JnMS5leGFtcGxlLmNvbTAeFw0xODA2MTcxMzQ1MjFaFw0yODA2MTQxMzQ1MjFa\nMGoxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T\nYW4gRnJhbmNpc2NvMQ0wCwYDVQQLEwRwZWVyMR8wHQYDVQQDExZwZWVyMC5vcmcx\nLmV4YW1wbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEKeC/1Rg/ynSk\nNNItaMlaCDZOaQvxJEl6o3fqx1PVFlfXE4NarY3OO1N3YZI41hWWoXksSwJu/35S\nM7wMEzw+3KNNMEswDgYDVR0PAQH/BAQDAgeAMAwGA1UdEwEB/wQCMAAwKwYDVR0j\nBCQwIoAgcecTOxTes6rfgyxHH6KIW7hsRAw2bhP9ikCHkvtv/RcwCgYIKoZIzj0E\nAwIDSAAwRQIhAKiJEv79XBmr8gGY6kHrGL0L3sq95E7IsCYzYdAQHj+DAiBPcBTg\nRuA0//Kq+3aHJ2T0KpKHqD3FfhZZolKDkcrkwQ==\n-----END CERTIFICATE-----\n",
		"Chaincodes": [
			"mycc"
		]
	},
	{
		"MSPID": "Org1MSP",
		"LedgerHeight": 5,
		"Endpoint": "peer1.org1.example.com:8051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICJzCCAc6gAwIBAgIQO7zMEHlMfRhnP6Xt65jwtDAKBggqhkjOPQQDAjBzMQsw\nCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy\nYW5jaXNjbzEZMBcGA1UEChMQb3JnMS5leGFtcGxlLmNvbTEcMBoGA1UEAxMTY2Eu\nb3JnMS5leGFtcGxlLmNvbTAeFw0xODA2MTcxMzQ1MjFaFw0yODA2MTQxMzQ1MjFa\nMGoxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T\nYW4gRnJhbmNpc2NvMQ0wCwYDVQQLEwRwZWVyMR8wHQYDVQQDExZwZWVyMS5vcmcx\nLmV4YW1wbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEoII9k8db/Q2g\nRHw5rk3SYw+OMFw9jNbsJJyC5ttJRvc12Dn7lQ8ZR9hW1vLQ3NtqO/couccDJcHg\nt47iHBNadaNNMEswDgYDVR0PAQH/BAQDAgeAMAwGA1UdEwEB/wQCMAAwKwYDVR0j\nBCQwIoAgcecTOxTes6rfgyxHH6KIW7hsRAw2bhP9ikCHkvtv/RcwCgYIKoZIzj0E\nAwIDRwAwRAIgGHGtRVxcFVeMQr9yRlebs23OXEECNo6hNqd/4ChLwwoCIBFKFd6t\nlL5BVzVMGQyXWcZGrjFgl4+fDrwjmMe+jAfa\n-----END CERTIFICATE-----\n",
		"Chaincodes": null
	}
]
```

As seen, this command outputs a JSON containing membership information
about all the peers in the channel that the peer queried possesses.

The `Identity` that is returned is the enrollment certificate of the
peer, and it can be parsed with a combination of `jq` and `openssl`:

```
$ discover --configFile conf.yaml peers --channel mychannel  --server peer0.org1.example.com:7051  | jq .[0].Identity | sed "s/\\\n/\n/g" | sed "s/\"//g"  | openssl x509 -text -noout
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            55:e9:3f:97:94:d5:74:db:e2:d6:99:3c:01:24:be:bf
    Signature Algorithm: ecdsa-with-SHA256
        Issuer: C=US, ST=California, L=San Francisco, O=org2.example.com, CN=ca.org2.example.com
        Validity
            Not Before: Jun  9 11:58:28 2018 GMT
            Not After : Jun  6 11:58:28 2028 GMT
        Subject: C=US, ST=California, L=San Francisco, OU=peer, CN=peer0.org2.example.com
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:f5:69:7a:11:65:d9:85:96:65:b7:b7:1b:08:77:
                    43:de:cb:ad:3a:79:ec:cc:2a:bc:d7:93:68:ae:92:
                    1c:4b:d8:32:47:d6:3d:72:32:f1:f1:fb:26:e4:69:
                    c2:eb:c9:45:69:99:78:d7:68:a9:77:09:88:c6:53:
                    01:2a:c1:f8:c0
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Basic Constraints: critical
                CA:FALSE
            X509v3 Authority Key Identifier:
                keyid:8E:58:82:C9:0A:11:10:A9:0B:93:03:EE:A0:54:42:F4:A3:EF:11:4C:82:B6:F9:CE:10:A2:1E:24:AB:13:82:A0

    Signature Algorithm: ecdsa-with-SHA256
         30:44:02:20:29:3f:55:2b:9f:7b:99:b2:cb:06:ca:15:3f:93:
         a1:3d:65:5c:7b:79:a1:7a:d1:94:50:f0:cd:db:ea:61:81:7a:
         02:20:3b:40:5b:60:51:3c:f8:0f:9b:fc:ae:fc:21:fd:c8:36:
         a3:18:39:58:20:72:3d:1a:43:74:30:f3:56:01:aa:26
```

Configuration query:
--------------------

The configuration query returns a mapping from MSP IDs to orderer
endpoints, as well as the `FabricMSPConfig` which can be used to verify
all peer and orderer nodes by the SDK:

```
$ discover --configFile conf.yaml config --channel mychannel  --server peer0.org1.example.com:7051
{
    "msps": {
        "OrdererOrg": {
            "name": "OrdererMSP",
            "root_certs": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNMekNDQWRhZ0F3SUJBZ0lSQU1pWkxUb3RmMHR6VTRzNUdIdkQ0UjR3Q2dZSUtvWkl6ajBFQXdJd2FURUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJjd0ZRWURWUVFERXc1allTNWxlR0Z0CmNHeGxMbU52YlRBZUZ3MHhPREEyTURreE1UVTRNamhhRncweU9EQTJNRFl4TVRVNE1qaGFNR2t4Q3pBSkJnTlYKQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVFlXNGdSbkpoYm1OcApjMk52TVJRd0VnWURWUVFLRXd0bGVHRnRjR3hsTG1OdmJURVhNQlVHQTFVRUF4TU9ZMkV1WlhoaGJYQnNaUzVqCmIyMHdXVEFUQmdjcWhrak9QUUlCQmdncWhrak9QUU1CQndOQ0FBUW9ySjVSamFTQUZRci9yc2xoMWdobnNCWEQKeDVsR1lXTUtFS1pDYXJDdkZBekE0bHUwb2NQd0IzNWJmTVN5bFJPVmdVdHF1ZU9IcFBNc2ZLNEFrWjR5bzE4dwpYVEFPQmdOVkhROEJBZjhFQkFNQ0FhWXdEd1lEVlIwbEJBZ3dCZ1lFVlIwbEFEQVBCZ05WSFJNQkFmOEVCVEFECkFRSC9NQ2tHQTFVZERnUWlCQ0JnbmZJd0pzNlBaWUZCclpZVkRpU05vSjNGZWNFWHYvN2xHL3QxVUJDbVREQUsKQmdncWhrak9QUVFEQWdOSEFEQkVBaUE5NGFkc21UK0hLalpFVVpnM0VkaWdSM296L3pEQkNhWUY3TEJUVXpuQgpEZ0lnYS9RZFNPQnk1TUx2c0lSNTFDN0N4UnR2NUM5V05WRVlmWk5SaGdXRXpoOD0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
            ],
            "admins": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNDVENDQWJDZ0F3SUJBZ0lRR2wzTjhaSzRDekRRQmZqYVpwMVF5VEFLQmdncWhrak9QUVFEQWpCcE1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVVNQklHQTFVRUNoTUxaWGhoYlhCc1pTNWpiMjB4RnpBVkJnTlZCQU1URG1OaExtVjRZVzF3CmJHVXVZMjl0TUI0WERURTRNRFl3T1RFeE5UZ3lPRm9YRFRJNE1EWXdOakV4TlRneU9Gb3dWakVMTUFrR0ExVUUKQmhNQ1ZWTXhFekFSQmdOVkJBZ1RDa05oYkdsbWIzSnVhV0V4RmpBVUJnTlZCQWNURFZOaGJpQkdjbUZ1WTJsegpZMjh4R2pBWUJnTlZCQU1NRVVGa2JXbHVRR1Y0WVcxd2JHVXVZMjl0TUZrd0V3WUhLb1pJemowQ0FRWUlLb1pJCnpqMERBUWNEUWdBRWl2TXQybVdiQ2FHb1FZaWpka1BRM1NuTGFkMi8rV0FESEFYMnRGNWthMTBteG1OMEx3VysKdmE5U1dLMmJhRGY5RDQ2TVROZ2gycnRhUitNWXFWRm84Nk5OTUVzd0RnWURWUjBQQVFIL0JBUURBZ2VBTUF3RwpBMVVkRXdFQi93UUNNQUF3S3dZRFZSMGpCQ1F3SW9BZ1lKM3lNQ2JPajJXQlFhMldGUTRramFDZHhYbkJGNy8rCjVSdjdkVkFRcGt3d0NnWUlLb1pJemowRUF3SURSd0F3UkFJZ2RIc0pUcGM5T01DZ3JPVFRLTFNnU043UWk3MWIKSWpkdzE4MzJOeXFQZnJ3Q0lCOXBhSlRnL2R5ckNhWUx1ZndUbUtFSnZZMEtXVzcrRnJTeG5CTGdzZjJpCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
            ],
            "crypto_config": {
                "signature_hash_family": "SHA2",
                "identity_identifier_hash_function": "SHA256"
            },
            "tls_root_certs": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNORENDQWR1Z0F3SUJBZ0lRZDdodzFIaHNZTXI2a25ETWJrZThTakFLQmdncWhrak9QUVFEQWpCc01Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVVNQklHQTFVRUNoTUxaWGhoYlhCc1pTNWpiMjB4R2pBWUJnTlZCQU1URVhSc2MyTmhMbVY0CllXMXdiR1V1WTI5dE1CNFhEVEU0TURZd09URXhOVGd5T0ZvWERUSTRNRFl3TmpFeE5UZ3lPRm93YkRFTE1Ba0cKQTFVRUJoTUNWVk14RXpBUkJnTlZCQWdUQ2tOaGJHbG1iM0p1YVdFeEZqQVVCZ05WQkFjVERWTmhiaUJHY21GdQpZMmx6WTI4eEZEQVNCZ05WQkFvVEMyVjRZVzF3YkdVdVkyOXRNUm93R0FZRFZRUURFeEYwYkhOallTNWxlR0Z0CmNHeGxMbU52YlRCWk1CTUdCeXFHU000OUFnRUdDQ3FHU000OUF3RUhBMElBQk9ZZGdpNm53a3pYcTBKQUF2cTIKZU5xNE5Ybi85L0VRaU13Tzc1dXdpTWJVbklYOGM1N2NYU2dQdy9NMUNVUGFwNmRyMldvTjA3RGhHb1B6ZXZaMwp1aFdqWHpCZE1BNEdBMVVkRHdFQi93UUVBd0lCcGpBUEJnTlZIU1VFQ0RBR0JnUlZIU1VBTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0tRWURWUjBPQkNJRUlCcW0xZW9aZy9qSW52Z1ZYR2cwbzVNamxrd2tSekRlalAzZkplbW8KU1hBek1Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lEUG9FRkF5bFVYcEJOMnh4VEo0MVplaS9ZQWFvN29aL0tEMwpvTVBpQ3RTOUFpQmFiU1dNS3UwR1l4eXdsZkFwdi9CWitxUEJNS0JMNk5EQ1haUnpZZmtENEE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
            ]
        },
        "Org1MSP": {
            "name": "Org1MSP",
            "root_certs": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQU1nN2VETnhwS0t0ZGl0TDRVNDRZMUl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGd3TmpBNU1URTFPREk0V2hjTk1qZ3dOakEyTVRFMU9ESTQKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQk41d040THpVNGRpcUZSWnB6d3FSVm9JbWw1MVh0YWkzbWgzUXo0UEZxWkhXTW9lZ0ovUWRNKzF4L3RobERPcwpnbmVRcndGd216WGpvSSszaHJUSmRuU2pYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSU9CZFFMRitjTVdhNmUxcDJDcE8KRXg3U0hVaW56VnZkNTVoTG03dzZ2NzJvTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFDQyt6T1lHcll0ZTB4SgpSbDVYdUxjUWJySW9UeHpsRnJLZWFNWnJXMnVaSkFJZ0NVVGU5MEl4aW55dk4wUkh4UFhoVGNJTFdEZzdLUEJOCmVrNW5TRlh3Y0lZPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
            ],
            "admins": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNLakNDQWRDZ0F3SUJBZ0lRRTRFK0tqSHgwdTlzRSsxZUgrL1dOakFLQmdncWhrak9QUVFEQWpCek1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTVM1bGVHRnRjR3hsTG1OdmJURWNNQm9HQTFVRUF4TVRZMkV1CmIzSm5NUzVsZUdGdGNHeGxMbU52YlRBZUZ3MHhPREEyTURreE1UVTRNamhhRncweU9EQTJNRFl4TVRVNE1qaGEKTUd3eEN6QUpCZ05WQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVApZVzRnUm5KaGJtTnBjMk52TVE4d0RRWURWUVFMRXdaamJHbGxiblF4SHpBZEJnTlZCQU1NRmtGa2JXbHVRRzl5Clp6RXVaWGhoYlhCc1pTNWpiMjB3V1RBVEJnY3Foa2pPUFFJQkJnZ3Foa2pPUFFNQkJ3TkNBQVFqK01MZk1ESnUKQ2FlWjV5TDR2TnczaWp4ZUxjd2YwSHo1blFrbXVpSnFETjRhQ0ZwVitNTTVablFEQmx1dWRyUS80UFA1Sk1WeQpreWZsQ3pJa2NCNjdvMDB3U3pBT0JnTlZIUThCQWY4RUJBTUNCNEF3REFZRFZSMFRBUUgvQkFJd0FEQXJCZ05WCkhTTUVKREFpZ0NEZ1hVQ3hmbkRGbXVudGFkZ3FUaE1lMGgxSXA4MWIzZWVZUzV1OE9yKzlxREFLQmdncWhrak8KUFFRREFnTklBREJGQWlFQXlJV21QcjlQakdpSk1QM1pVd05MRENnNnVwMlVQVXNJSzd2L2h3RVRra01DSUE0cQo3cHhQZy9VVldiamZYeE0wUCsvcTEzbXFFaFlYaVpTTXpoUENFNkNmCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
            ],
            "crypto_config": {
                "signature_hash_family": "SHA2",
                "identity_identifier_hash_function": "SHA256"
            },
            "tls_root_certs": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNTVENDQWUrZ0F3SUJBZ0lRZlRWTE9iTENVUjdxVEY3Z283UXgvakFLQmdncWhrak9QUVFEQWpCMk1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTVM1bGVHRnRjR3hsTG1OdmJURWZNQjBHQTFVRUF4TVdkR3h6ClkyRXViM0puTVM1bGVHRnRjR3hsTG1OdmJUQWVGdzB4T0RBMk1Ea3hNVFU0TWpoYUZ3MHlPREEyTURZeE1UVTQKTWpoYU1IWXhDekFKQmdOVkJBWVRBbFZUTVJNd0VRWURWUVFJRXdwRFlXeHBabTl5Ym1saE1SWXdGQVlEVlFRSApFdzFUWVc0Z1JuSmhibU5wYzJOdk1Sa3dGd1lEVlFRS0V4QnZjbWN4TG1WNFlXMXdiR1V1WTI5dE1SOHdIUVlEClZRUURFeFowYkhOallTNXZjbWN4TG1WNFlXMXdiR1V1WTI5dE1Ga3dFd1lIS29aSXpqMENBUVlJS29aSXpqMEQKQVFjRFFnQUVZbnp4bmMzVUpHS0ZLWDNUNmR0VGpkZnhJTVYybGhTVzNab0lWSW9mb04rWnNsWWp0d0g2ZXZXYgptTkZGQmRaYWExTjluaXRpbmxxbVVzTU1NQ2JieXFOZk1GMHdEZ1lEVlIwUEFRSC9CQVFEQWdHbU1BOEdBMVVkCkpRUUlNQVlHQkZVZEpRQXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QXBCZ05WSFE0RUlnUWdlVTAwNlNaUllUNDIKN1Uxb2YwL3RGdHUvRFVtazVLY3hnajFCaklJakduZ3dDZ1lJS29aSXpqMEVBd0lEU0FBd1JRSWhBSWpvcldJTwpRNVNjYjNoZDluRi9UamxWcmk1UHdTaDNVNmJaMFdYWEsxYzVBaUFlMmM5QmkyNFE1WjQ0aXQ1MkI5cm1hU1NpCkttM2NZVlY0cWJ6RFhMOHZYUT09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
            ],
            "fabric_node_ous": {
                "enable": true,
                "client_ou_identifier": {
                    "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQU1nN2VETnhwS0t0ZGl0TDRVNDRZMUl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGd3TmpBNU1URTFPREk0V2hjTk1qZ3dOakEyTVRFMU9ESTQKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQk41d040THpVNGRpcUZSWnB6d3FSVm9JbWw1MVh0YWkzbWgzUXo0UEZxWkhXTW9lZ0ovUWRNKzF4L3RobERPcwpnbmVRcndGd216WGpvSSszaHJUSmRuU2pYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSU9CZFFMRitjTVdhNmUxcDJDcE8KRXg3U0hVaW56VnZkNTVoTG03dzZ2NzJvTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFDQyt6T1lHcll0ZTB4SgpSbDVYdUxjUWJySW9UeHpsRnJLZWFNWnJXMnVaSkFJZ0NVVGU5MEl4aW55dk4wUkh4UFhoVGNJTFdEZzdLUEJOCmVrNW5TRlh3Y0lZPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
                    "organizational_unit_identifier": "client"
                },
                "peer_ou_identifier": {
                    "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQU1nN2VETnhwS0t0ZGl0TDRVNDRZMUl3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpFdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGd3TmpBNU1URTFPREk0V2hjTk1qZ3dOakEyTVRFMU9ESTQKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NUzVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQk41d040THpVNGRpcUZSWnB6d3FSVm9JbWw1MVh0YWkzbWgzUXo0UEZxWkhXTW9lZ0ovUWRNKzF4L3RobERPcwpnbmVRcndGd216WGpvSSszaHJUSmRuU2pYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSU9CZFFMRitjTVdhNmUxcDJDcE8KRXg3U0hVaW56VnZkNTVoTG03dzZ2NzJvTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFDQyt6T1lHcll0ZTB4SgpSbDVYdUxjUWJySW9UeHpsRnJLZWFNWnJXMnVaSkFJZ0NVVGU5MEl4aW55dk4wUkh4UFhoVGNJTFdEZzdLUEJOCmVrNW5TRlh3Y0lZPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
                    "organizational_unit_identifier": "peer"
                }
            }
        },
        "Org2MSP": {
            "name": "Org2MSP",
            "root_certs": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQUx2SWV2KzE4Vm9LZFR2V1RLNCtaZ2d3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGd3TmpBNU1URTFPREk0V2hjTk1qZ3dOakEyTVRFMU9ESTQKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkhUS01aall0TDdnSXZ0ekN4Y2pMQit4NlZNdENzVW0wbExIcGtIeDFQaW5LUU1ybzFJWWNIMEpGVmdFempvSQpCcUdMYURyQmhWQkpoS1kwS21kMUJJZWpYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSUk1WWdza0tFUkNwQzVNRDdxQlUKUXZTajd4Rk1ncmI1emhDaUhpU3JFNEtnTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFDWnNSUjVBVU5KUjdJbwpQQzgzUCt1UlF1RmpUYS94eitzVkpZYnBsNEh1Z1FJZ0QzUlhuQWFqaGlPMU1EL1JzSC9JN2FPL1RuWUxkQUl6Cnd4VlNJenhQbWd3PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
            ],
            "admins": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNLVENDQWRDZ0F3SUJBZ0lRU1lpeE1vdmpoM1N2c25WMmFUOXl1REFLQmdncWhrak9QUVFEQWpCek1Rc3cKQ1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeQpZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTWk1bGVHRnRjR3hsTG1OdmJURWNNQm9HQTFVRUF4TVRZMkV1CmIzSm5NaTVsZUdGdGNHeGxMbU52YlRBZUZ3MHhPREEyTURreE1UVTRNamhhRncweU9EQTJNRFl4TVRVNE1qaGEKTUd3eEN6QUpCZ05WQkFZVEFsVlRNUk13RVFZRFZRUUlFd3BEWVd4cFptOXlibWxoTVJZd0ZBWURWUVFIRXcxVApZVzRnUm5KaGJtTnBjMk52TVE4d0RRWURWUVFMRXdaamJHbGxiblF4SHpBZEJnTlZCQU1NRmtGa2JXbHVRRzl5Clp6SXVaWGhoYlhCc1pTNWpiMjB3V1RBVEJnY3Foa2pPUFFJQkJnZ3Foa2pPUFFNQkJ3TkNBQVJFdStKc3l3QlQKdkFYUUdwT2FuS3ZkOVhCNlMxVGU4NTJ2L0xRODVWM1Rld0hlYXZXeGUydUszYTBvRHA5WDV5SlJ4YXN2b2hCcwpOMGJIRWErV1ZFQjdvMDB3U3pBT0JnTlZIUThCQWY4RUJBTUNCNEF3REFZRFZSMFRBUUgvQkFJd0FEQXJCZ05WCkhTTUVKREFpZ0NDT1dJTEpDaEVRcVF1VEErNmdWRUwwbys4UlRJSzIrYzRRb2g0a3F4T0NvREFLQmdncWhrak8KUFFRREFnTkhBREJFQWlCVUFsRStvbFBjMTZBMitmNVBRSmdTZFp0SjNPeXBieG9JVlhOdi90VUJ2QUlnVGFNcgo1K2k2TUxpaU9FZ0wzcWZSWmdkcG1yVm1SbHlIdVdabWE0NXdnaE09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
            ],
            "crypto_config": {
                "signature_hash_family": "SHA2",
                "identity_identifier_hash_function": "SHA256"
            },
            "tls_root_certs": [
                "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNTakNDQWZDZ0F3SUJBZ0lSQUtoUFFxUGZSYnVpSktqL0JRanQ3RXN3Q2dZSUtvWkl6ajBFQXdJd2RqRUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIekFkQmdOVkJBTVRGblJzCmMyTmhMbTl5WnpJdVpYaGhiWEJzWlM1amIyMHdIaGNOTVRnd05qQTVNVEUxT0RJNFdoY05Namd3TmpBMk1URTEKT0RJNFdqQjJNUXN3Q1FZRFZRUUdFd0pWVXpFVE1CRUdBMVVFQ0JNS1EyRnNhV1p2Y201cFlURVdNQlFHQTFVRQpCeE1OVTJGdUlFWnlZVzVqYVhOamJ6RVpNQmNHQTFVRUNoTVFiM0puTWk1bGVHRnRjR3hsTG1OdmJURWZNQjBHCkExVUVBeE1XZEd4elkyRXViM0puTWk1bGVHRnRjR3hsTG1OdmJUQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDkKQXdFSEEwSUFCRVIrMnREOWdkME9NTlk5Y20rbllZR2NUeWszRStCMnBsWWxDL2ZVdGdUU0QyZUVyY2kyWmltdQo5N25YeUIrM0NwNFJwVjFIVHdaR0JMbmNnbVIyb1J5alh6QmRNQTRHQTFVZER3RUIvd1FFQXdJQnBqQVBCZ05WCkhTVUVDREFHQmdSVkhTVUFNQThHQTFVZEV3RUIvd1FGTUFNQkFmOHdLUVlEVlIwT0JDSUVJUEN0V01JRFRtWC8KcGxseS8wNDI4eFRXZHlhazQybU9tbVNJSENCcnAyN0tNQW9HQ0NxR1NNNDlCQU1DQTBnQU1FVUNJUUNtN2xmVQpjbG91VHJrS2Z1YjhmdmdJTTU3QS85bW5IdzhpQnAycURtamZhUUlnSjkwcnRUV204YzVBbE93bFpyYkd0NWZMCjF6WXg5QW5DMTJBNnhOZDIzTG89Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
            ],
            "fabric_node_ous": {
                "enable": true,
                "client_ou_identifier": {
                    "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQUx2SWV2KzE4Vm9LZFR2V1RLNCtaZ2d3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGd3TmpBNU1URTFPREk0V2hjTk1qZ3dOakEyTVRFMU9ESTQKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkhUS01aall0TDdnSXZ0ekN4Y2pMQit4NlZNdENzVW0wbExIcGtIeDFQaW5LUU1ybzFJWWNIMEpGVmdFempvSQpCcUdMYURyQmhWQkpoS1kwS21kMUJJZWpYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSUk1WWdza0tFUkNwQzVNRDdxQlUKUXZTajd4Rk1ncmI1emhDaUhpU3JFNEtnTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFDWnNSUjVBVU5KUjdJbwpQQzgzUCt1UlF1RmpUYS94eitzVkpZYnBsNEh1Z1FJZ0QzUlhuQWFqaGlPMU1EL1JzSC9JN2FPL1RuWUxkQUl6Cnd4VlNJenhQbWd3PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
                    "organizational_unit_identifier": "client"
                },
                "peer_ou_identifier": {
                    "certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSRENDQWVxZ0F3SUJBZ0lSQUx2SWV2KzE4Vm9LZFR2V1RLNCtaZ2d3Q2dZSUtvWkl6ajBFQXdJd2N6RUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhHVEFYQmdOVkJBb1RFRzl5WnpJdVpYaGhiWEJzWlM1amIyMHhIREFhQmdOVkJBTVRFMk5oCkxtOXlaekl1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGd3TmpBNU1URTFPREk0V2hjTk1qZ3dOakEyTVRFMU9ESTQKV2pCek1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGc2FXWnZjbTVwWVRFV01CUUdBMVVFQnhNTgpVMkZ1SUVaeVlXNWphWE5qYnpFWk1CY0dBMVVFQ2hNUWIzSm5NaTVsZUdGdGNHeGxMbU52YlRFY01Cb0dBMVVFCkF4TVRZMkV1YjNKbk1pNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUEKQkhUS01aall0TDdnSXZ0ekN4Y2pMQit4NlZNdENzVW0wbExIcGtIeDFQaW5LUU1ybzFJWWNIMEpGVmdFempvSQpCcUdMYURyQmhWQkpoS1kwS21kMUJJZWpYekJkTUE0R0ExVWREd0VCL3dRRUF3SUJwakFQQmdOVkhTVUVDREFHCkJnUlZIU1VBTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3S1FZRFZSME9CQ0lFSUk1WWdza0tFUkNwQzVNRDdxQlUKUXZTajd4Rk1ncmI1emhDaUhpU3JFNEtnTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDSVFDWnNSUjVBVU5KUjdJbwpQQzgzUCt1UlF1RmpUYS94eitzVkpZYnBsNEh1Z1FJZ0QzUlhuQWFqaGlPMU1EL1JzSC9JN2FPL1RuWUxkQUl6Cnd4VlNJenhQbWd3PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
                    "organizational_unit_identifier": "peer"
                }
            }
        },
        "Org3MSP": {
            "name": "Org3MSP",
            "root_certs": [
                "CgJPVQoEUm9sZQoMRW5yb2xsbWVudElEChBSZXZvY2F0aW9uSGFuZGxlEkQKIKoEXcq/psdYnMKCiT79N+dS1hM8k+SuzU1blOgTuN++EiBe2m3E+FjWLuQGMNRGRrEVTMqTvC4A/5jvCLv2ja1sZxpECiDBbI0kwetxAwFzHwb1hi8TlkGW3OofvuVzfFt9VlewcRIgyvsxG5/THdWyKJTdNx8Gle2hoCbVF0Y1/DQESBjGOGciRAog25fMyWps+FLOjzj1vIsGUyO457ri3YMvmUcycIH2FvQSICTtzaFvSPUiDtNtAVz+uetuB9kfmjUdUSQxjyXULOm2IkQKIO8FKzwoWwu8Mo77GNqnKFGCZaJL9tlrkdTuEMu9ujzbEiA4xtzo8oo8oEhFVsl6010mNoj1VuI0Wmz4tvUgXolCIiJECiDZcZPuwk/uaJMuVph7Dy/icgnAtVYHShET41O0Eh3Q5BIgy5q9VMQrch9VW5yajhY8dH1uA593gKd5kBqGdLfiXzAiRAogAnUYq/kwKzFfmIm/W4nZxi1kjG2C8NRjsYYBkeAOQ6wSIGyX5GGmwgvxgXXehNWBfijyNIJALGRVhO8YtBqr+vnrKogBCiDHR1XQsDbpcBoZFJ09V97zsIKNVTxjUow7/wwC+tq3oBIgSWT/peiO2BI0DecypKfgMpVR8DWXl8ZHSrPISsL3Mc8aINem9+BOezLwFKCbtVH1KAHIRLyyiNP+TkIKW6x9RkThIiAbIJCYU6O02EB8uX6rqLU/1lHxV0vtWdIsKCTLx2EZmDJECiCPXeyUyFzPS3iFv8CQUOLCPZxf6buZS5JlM6EE/gCRaxIgmF9GKPLLmEoA77+AU3J8Iwnu9pBxnaHtUlyf/F9p30c6RAogG7ENKWlOZ4aF0HprqXAjl++Iao7/iE8xeVcKRlmfq1ASIGtmmavDAVS2bw3zClQd4ZBD2DrqCBO9NPOcLNB0IWeIQiCjxTdbmcuBNINZYWe+5fWyI1oY9LavKzDVkdh+miu26EogY2uJtJGfKrQQjy+pgf9FdPMUk+8PNUBtH9LCD4bos7JSIPl6m5lEP/PRAmBaeTQLXdbMxIthxM2gw+Zkc5+IJEWX"
            ],
            "intermediate_certs": [
                "CtgCCkQKIP0UVivtH8NlnRNrZuuu6jpaj2ZbEB4/secGS57MfbINEiDSJweLUMIQSW12jugBQG81lIQflJWvi7vi925u+PU/+xJECiDgOGdNbAiGSoHmTjKhT22fqUqYLIVh+JBHetm4kF4skhIg9XTWRkUqtsfYKENzPgm7ZUSmCHNF8xH7Vnhuc1EpAUgaINwSnJKofiMoyDRZwUBhgfwMH9DJzMccvRVW7IvLMe/cIiCnlRj+mfNVAJGKthLgQBB/JKM14NbUeutyJtTgrmDDiCogme25qGvxJfgQNnzldMMicVyiI6YMfnoThAUyqsTzyXkqIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAKiCZ7bmoa/El+BA2fOV0wyJxXKIjpgx+ehOEBTKqxPPJeSogAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAESIFYUenRvjbmEh+37YHJrvFJt4lGq9ShtJ4kEBrfHArPjGgNPVTEqA09VMTL0ARKIAQog/gwzULTJbCAoVg9XfCiROs4cU5oSv4Q80iYWtonAnvsSIE6mYFdzisBU21rhxjfYE7kk3Xjih9A1idJp7TSjfmorGiBwIEbnxUKjs3Z3DXUSTj5R78skdY1hWEjpCbSBvtwn/yIgBVTjvNOIwpBC7qZJKX6yn4tMvoCCGpiz4BKBEUqtBJsaZzBlAjBwZ4WXYOttkhsNA2r94gBfLUdx/4VhW4hwUImcztlau1T14UlNzJolCNkdiLc9CqsCMQD6OBkgDWGq9UlhkK9dJBzU+RElcZdSfVV1hDbbqt+lFRWOzzEkZ+BXCR1k3xybz+o="
            ],
            "admins": [
                "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUhZd0VBWUhLb1pJemowQ0FRWUZLNEVFQUNJRFlnQUVUYk13SEZteEpEMWR3SjE2K0hnVnRDZkpVRzdKK2FTYgorbkVvVmVkREVHYmtTc1owa1lraEpyYkx5SHlYZm15ZWV0ejFIUk1rWjRvMjdxRlMzTlVFb1J2QlM3RHJPWDJjCnZLaDRnbWhHTmlPbzRiWjFOVG9ZL2o3QnpqMFlMSXNlCi0tLS0tRU5EIFBVQkxJQyBLRVktLS0tLQo="
            ]
        }
    },
    "orderers": {
        "OrdererOrg": {
            "endpoint": [
                {
                    "host": "orderer.example.com",
                    "port": 7050
                }
            ]
        }
    }
}
```

It's important to note that the certificates here are base64 encoded,
and thus should decoded in a manner similar to the following:

```
$ discover --configFile conf.yaml config --channel mychannel  --server peer0.org1.example.com:7051 | jq .msps.OrdererOrg.root_certs[0] | sed "s/\"//g" | base64 --decode | openssl x509 -text -noout
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            c8:99:2d:3a:2d:7f:4b:73:53:8b:39:18:7b:c3:e1:1e
    Signature Algorithm: ecdsa-with-SHA256
        Issuer: C=US, ST=California, L=San Francisco, O=example.com, CN=ca.example.com
        Validity
            Not Before: Jun  9 11:58:28 2018 GMT
            Not After : Jun  6 11:58:28 2028 GMT
        Subject: C=US, ST=California, L=San Francisco, O=example.com, CN=ca.example.com
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:28:ac:9e:51:8d:a4:80:15:0a:ff:ae:c9:61:d6:
                    08:67:b0:15:c3:c7:99:46:61:63:0a:10:a6:42:6a:
                    b0:af:14:0c:c0:e2:5b:b4:a1:c3:f0:07:7e:5b:7c:
                    c4:b2:95:13:95:81:4b:6a:b9:e3:87:a4:f3:2c:7c:
                    ae:00:91:9e:32
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature, Key Encipherment, Certificate Sign, CRL Sign
            X509v3 Extended Key Usage:
                Any Extended Key Usage
            X509v3 Basic Constraints: critical
                CA:TRUE
            X509v3 Subject Key Identifier:
                60:9D:F2:30:26:CE:8F:65:81:41:AD:96:15:0E:24:8D:A0:9D:C5:79:C1:17:BF:FE:E5:1B:FB:75:50:10:A6:4C
    Signature Algorithm: ecdsa-with-SHA256
         30:44:02:20:3d:e1:a7:6c:99:3f:87:2a:36:44:51:98:37:11:
         d8:a0:47:7a:33:ff:30:c1:09:a6:05:ec:b0:53:53:39:c1:0e:
         02:20:6b:f4:1d:48:e0:72:e4:c2:ef:b0:84:79:d4:2e:c2:c5:
         1b:6f:e4:2f:56:35:51:18:7d:93:51:86:05:84:ce:1f
```

Endorsers query:
----------------

To query for the endorsers of a chaincode call, additional flags need to
be supplied:

-   The `--chaincode` flag is mandatory and it provides the chaincode
    name(s). To query for a chaincode-to-chaincode invocation, one needs
    to repeat the `--chaincode` flag with all the chaincodes.
-   The `--collection` is used to specify private data collections that
    are expected to used by the chaincode(s). To map from thechaincodes
    passed via `--chaincode` to the collections, the following syntax
    should be used: `collection=CC:Collection1,Collection2,...`.
- The `--noPrivateReads` is used to indicate that the transaction is not expected
    to read private data for a certain chaincode.
    This is useful for private data "blind writes", among other things.

For example, to query for a chaincode invocation that results in both
cc1 and cc2 to be invoked, as well as writes to private data collection
col1 by cc2, one needs to specify:
`--chaincode=cc1 --chaincode=cc2 --collection=cc2:col1`

If chaincode cc2 is not expected to read from collection `col1` then `--noPrivateReads=cc2` should be used.

Below is the output of an endorsers query for chaincode **mycc** when
the endorsement policy is `AND('Org1.peer', 'Org2.peer')`:

```
$ discover --configFile conf.yaml endorsers --channel mychannel  --server peer0.org1.example.com:7051 --chaincode mycc
[
    {
        "Chaincode": "mycc",
        "EndorsersByGroups": {
            "G0": [
                {
                    "MSPID": "Org1MSP",
                    "LedgerHeight": 5,
                    "Endpoint": "peer0.org1.example.com:7051",
                    "Identity": "-----BEGIN CERTIFICATE-----\nMIICKDCCAc+gAwIBAgIRANTiKfUVHVGnrYVzEy1ZSKIwCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzEuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzEuZXhhbXBsZS5jb20wHhcNMTgwNjA5MTE1ODI4WhcNMjgwNjA2MTE1ODI4\nWjBqMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzENMAsGA1UECxMEcGVlcjEfMB0GA1UEAxMWcGVlcjAub3Jn\nMS5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD8jGz1l5Rrw\n5UWqAYnc4JrR46mCYwHhHFgwydccuytb00ouD4rECiBsCaeZFr5tODAK70jFOP/k\n/CtORCDPQ02jTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1Ud\nIwQkMCKAIOBdQLF+cMWa6e1p2CpOEx7SHUinzVvd55hLm7w6v72oMAoGCCqGSM49\nBAMCA0cAMEQCIC3bacbDYphXfHrNULxpV/zwD08t7hJxNe8MwgP8/48fAiBiC0cr\nu99oLsRNCFB7R3egyKg1YYao0KWTrr1T+rK9Bg==\n-----END CERTIFICATE-----\n"
                }
            ],
            "G1": [
                {
                    "MSPID": "Org2MSP",
                    "LedgerHeight": 5,
                    "Endpoint": "peer1.org2.example.com:10051",
                    "Identity": "-----BEGIN CERTIFICATE-----\nMIICKDCCAc+gAwIBAgIRAIs6fFxk4Y5cJxSwTjyJ9A8wCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzIuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzIuZXhhbXBsZS5jb20wHhcNMTgwNjA5MTE1ODI4WhcNMjgwNjA2MTE1ODI4\nWjBqMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzENMAsGA1UECxMEcGVlcjEfMB0GA1UEAxMWcGVlcjEub3Jn\nMi5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABOVFyWVmKZ25\nxDYV3xZBDX4gKQ7rAZfYgOu1djD9EHccZhJVPsdwSjbRsvrfs9Z8mMuwEeSWq/cq\n0cGrMKR93vKjTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1Ud\nIwQkMCKAII5YgskKERCpC5MD7qBUQvSj7xFMgrb5zhCiHiSrE4KgMAoGCCqGSM49\nBAMCA0cAMEQCIDJmxseFul1GZ26djKa6jZ6zYYf6hchNF5xxMRWXpCnuAiBMf6JZ\njZjVM9F/OidQ2SBR7OZyMAzgXc5nAabWZpdkuQ==\n-----END CERTIFICATE-----\n"
                },
                {
                    "MSPID": "Org2MSP",
                    "LedgerHeight": 5,
                    "Endpoint": "peer0.org2.example.com:9051",
                    "Identity": "-----BEGIN CERTIFICATE-----\nMIICJzCCAc6gAwIBAgIQVek/l5TVdNvi1pk8ASS+vzAKBggqhkjOPQQDAjBzMQsw\nCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy\nYW5jaXNjbzEZMBcGA1UEChMQb3JnMi5leGFtcGxlLmNvbTEcMBoGA1UEAxMTY2Eu\nb3JnMi5leGFtcGxlLmNvbTAeFw0xODA2MDkxMTU4MjhaFw0yODA2MDYxMTU4Mjha\nMGoxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T\nYW4gRnJhbmNpc2NvMQ0wCwYDVQQLEwRwZWVyMR8wHQYDVQQDExZwZWVyMC5vcmcy\nLmV4YW1wbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE9Wl6EWXZhZZl\nt7cbCHdD3sutOnnszCq815NorpIcS9gyR9Y9cjLx8fsm5GnC68lFaZl412ipdwmI\nxlMBKsH4wKNNMEswDgYDVR0PAQH/BAQDAgeAMAwGA1UdEwEB/wQCMAAwKwYDVR0j\nBCQwIoAgjliCyQoREKkLkwPuoFRC9KPvEUyCtvnOEKIeJKsTgqAwCgYIKoZIzj0E\nAwIDRwAwRAIgKT9VK597mbLLBsoVP5OhPWVce3mhetGUUPDN2+phgXoCIDtAW2BR\nPPgPm/yu/CH9yDajGDlYIHI9GkN0MPNWAaom\n-----END CERTIFICATE-----\n"
                }
            ]
        },
        "Layouts": [
            {
                "quantities_by_group": {
                    "G0": 1,
                    "G1": 1
                }
            }
        ]
    }
]
```

Not using a configuration file
------------------------------

It is possible to execute the discovery CLI without having a
configuration file, and just passing all needed configuration as
commandline flags. The following is an example of a local peer membership
query which loads administrator credentials:

```
$ discover --peerTLSCA tls/ca.crt --userKey msp/keystore/cf31339d09e8311ac9ca5ed4e27a104a7f82f1e5904b3296a170ba4725ffde0d_sk --userCert msp/signcerts/Admin\@org1.example.com-cert.pem --MSP Org1MSP --tlsCert tls/client.crt --tlsKey tls/client.key peers --server peer0.org1.example.com:7051
[
	{
		"MSPID": "Org1MSP",
		"Endpoint": "peer1.org1.example.com:8051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICJzCCAc6gAwIBAgIQO7zMEHlMfRhnP6Xt65jwtDAKBggqhkjOPQQDAjBzMQsw\nCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy\nYW5jaXNjbzEZMBcGA1UEChMQb3JnMS5leGFtcGxlLmNvbTEcMBoGA1UEAxMTY2Eu\nb3JnMS5leGFtcGxlLmNvbTAeFw0xODA2MTcxMzQ1MjFaFw0yODA2MTQxMzQ1MjFa\nMGoxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T\nYW4gRnJhbmNpc2NvMQ0wCwYDVQQLEwRwZWVyMR8wHQYDVQQDExZwZWVyMS5vcmcx\nLmV4YW1wbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEoII9k8db/Q2g\nRHw5rk3SYw+OMFw9jNbsJJyC5ttJRvc12Dn7lQ8ZR9hW1vLQ3NtqO/couccDJcHg\nt47iHBNadaNNMEswDgYDVR0PAQH/BAQDAgeAMAwGA1UdEwEB/wQCMAAwKwYDVR0j\nBCQwIoAgcecTOxTes6rfgyxHH6KIW7hsRAw2bhP9ikCHkvtv/RcwCgYIKoZIzj0E\nAwIDRwAwRAIgGHGtRVxcFVeMQr9yRlebs23OXEECNo6hNqd/4ChLwwoCIBFKFd6t\nlL5BVzVMGQyXWcZGrjFgl4+fDrwjmMe+jAfa\n-----END CERTIFICATE-----\n",
	},
	{
		"MSPID": "Org1MSP",
		"Endpoint": "peer0.org1.example.com:7051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICKDCCAc6gAwIBAgIQP18LeXtEXGoN8pTqzXTHZTAKBggqhkjOPQQDAjBzMQsw\nCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy\nYW5jaXNjbzEZMBcGA1UEChMQb3JnMS5leGFtcGxlLmNvbTEcMBoGA1UEAxMTY2Eu\nb3JnMS5leGFtcGxlLmNvbTAeFw0xODA2MTcxMzQ1MjFaFw0yODA2MTQxMzQ1MjFa\nMGoxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T\nYW4gRnJhbmNpc2NvMQ0wCwYDVQQLEwRwZWVyMR8wHQYDVQQDExZwZWVyMC5vcmcx\nLmV4YW1wbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEKeC/1Rg/ynSk\nNNItaMlaCDZOaQvxJEl6o3fqx1PVFlfXE4NarY3OO1N3YZI41hWWoXksSwJu/35S\nM7wMEzw+3KNNMEswDgYDVR0PAQH/BAQDAgeAMAwGA1UdEwEB/wQCMAAwKwYDVR0j\nBCQwIoAgcecTOxTes6rfgyxHH6KIW7hsRAw2bhP9ikCHkvtv/RcwCgYIKoZIzj0E\nAwIDSAAwRQIhAKiJEv79XBmr8gGY6kHrGL0L3sq95E7IsCYzYdAQHj+DAiBPcBTg\nRuA0//Kq+3aHJ2T0KpKHqD3FfhZZolKDkcrkwQ==\n-----END CERTIFICATE-----\n",
	},
	{
		"MSPID": "Org2MSP",
		"Endpoint": "peer0.org2.example.com:9051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICKTCCAc+gAwIBAgIRANK4WBck5gKuzTxVQIwhYMUwCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzIuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzIuZXhhbXBsZS5jb20wHhcNMTgwNjE3MTM0NTIxWhcNMjgwNjE0MTM0NTIx\nWjBqMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzENMAsGA1UECxMEcGVlcjEfMB0GA1UEAxMWcGVlcjAub3Jn\nMi5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABJa0gkMRqJCi\nzmx+L9xy/ecJNvdAV2zmSx5Sf2qospVAH1MYCHyudDEvkiRuBPgmCdOdwJsE0g+h\nz0nZdKq6/X+jTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1Ud\nIwQkMCKAIFZMuZfUtY6n2iyxaVr3rl+x5lU0CdG9x7KAeYydQGTMMAoGCCqGSM49\nBAMCA0gAMEUCIQC0M9/LJ7j3I9NEPQ/B1BpnJP+UNPnGO2peVrM/mJ1nVgIgS1ZA\nA1tsxuDyllaQuHx2P+P9NDFdjXx5T08lZhxuWYM=\n-----END CERTIFICATE-----\n",
	},
	{
		"MSPID": "Org2MSP",
		"Endpoint": "peer1.org2.example.com:10051",
		"Identity": "-----BEGIN CERTIFICATE-----\nMIICKDCCAc+gAwIBAgIRALnNJzplCrYy4Y8CjZtqL7AwCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzIuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzIuZXhhbXBsZS5jb20wHhcNMTgwNjE3MTM0NTIxWhcNMjgwNjE0MTM0NTIx\nWjBqMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzENMAsGA1UECxMEcGVlcjEfMB0GA1UEAxMWcGVlcjEub3Jn\nMi5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABNDopAkHlDdu\nq10HEkdxvdpkbs7EJyqv1clvCt/YMn1hS6sM+bFDgkJKalG7s9Hg3URF0aGpy51R\nU+4F9Muo+XajTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1Ud\nIwQkMCKAIFZMuZfUtY6n2iyxaVr3rl+x5lU0CdG9x7KAeYydQGTMMAoGCCqGSM49\nBAMCA0cAMEQCIAR4fBmIBKW2jp0HbbabVepNtl1c7+6++riIrEBnoyIVAiBBvWmI\nyG02c5hu4wPAuVQMB7AU6tGSeYaWSAAo/ExunQ==\n-----END CERTIFICATE-----\n",
	}
]
```
