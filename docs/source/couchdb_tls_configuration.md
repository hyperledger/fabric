# CouchDB TLS Configuration Guide

## Overview

Hyperledger Fabric Peer supports TLS/SSL encrypted connections to CouchDB for secure state database communication. This guide explains how to configure TLS for CouchDB connections.

## Configuration Options

### YAML Configuration (core.yaml)

Add TLS configuration under `ledger.state.couchDBConfig.tls`:

```yaml
ledger:
  state:
    stateDatabase: CouchDB
    couchDBConfig:
       couchDBAddress: couchdb.example.com:6984
       username: admin
       password: password
       # ... other settings ...
       
       tls:
          # Enable TLS for CouchDB connection
          enabled: true
          
          # Path to CA certificate for server verification (optional)
          caCertFile: /path/to/ca-cert.pem
          
          # Skip server certificate verification (insecure - testing only)
          skipVerify: false
```

### Environment Variables

All TLS settings can be configured via environment variables:

```bash
# Enable TLS
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_ENABLED=true

# CA certificate for server verification
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_CACERTFILE=/path/to/ca-cert.pem

# Skip verification (not recommended for production)
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_SKIPVERIFY=false
```

## Configuration Scenarios

### Scenario 1: Server Authentication with Custom CA (Recommended)

This configuration verifies the CouchDB server's identity using a CA certificate:

```yaml
tls:
   enabled: true
   caCertFile: /etc/hyperledger/fabric/tls/ca-cert.pem
```

Or with environment variables:

```bash
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_ENABLED=true
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_CACERTFILE=/etc/hyperledger/fabric/tls/ca-cert.pem
```

### Scenario 2: TLS with System CA Pool

Use the system's default CA certificate pool for server verification:

```yaml
tls:
   enabled: true
```

```bash
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_ENABLED=true
```

### Scenario 3: Skip Verification (Testing Only - NOT RECOMMENDED)

**WARNING**: This is insecure and should only be used for testing purposes.

```yaml
tls:
   enabled: true
   skipVerify: true
```

```bash
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_ENABLED=true
export CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_SKIPVERIFY=true
```

## Docker Compose Example

```yaml
version: '3.7'

services:
  couchdb:
    image: couchdb:3.3
    environment:
      - COUCHDB_USER=admin
      - COUCHDB_PASSWORD=password
    ports:
      - "6984:6984"
    volumes:
      - ./couchdb-tls:/opt/couchdb/etc/certs
    # Configure CouchDB for TLS (see CouchDB documentation)

  peer:
    image: hyperledger/fabric-peer:latest
    environment:
      - CORE_LEDGER_STATE_STATEDATABASE=CouchDB
      - CORE_LEDGER_STATE_COUCHDBCONFIG_COUCHDBADDRESS=couchdb:6984
      - CORE_LEDGER_STATE_COUCHDBCONFIG_USERNAME=admin
      - CORE_LEDGER_STATE_COUCHDBCONFIG_PASSWORD=password
      - CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_ENABLED=true
      - CORE_LEDGER_STATE_COUCHDBCONFIG_TLS_CACERTFILE=/etc/hyperledger/fabric/tls/ca-cert.pem
    volumes:
      - ./crypto-config/peer/tls:/etc/hyperledger/fabric/tls
    depends_on:
      - couchdb
```

## Certificate Requirements

### CA Certificate (caCertFile)
- PEM-encoded X.509 certificate
- Used to verify the CouchDB server's certificate
- Can be a root CA or intermediate CA certificate
- If not provided, system CA pool will be used

## Generating Test Certificates

For testing purposes, you can generate self-signed certificates:

```bash
# Generate CA key and certificate
openssl req -x509 -newkey rsa:4096 -keyout ca-key.pem -out ca-cert.pem -days 365 -nodes -subj "/CN=Test CA"

# Generate server key and certificate (for CouchDB)
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server-csr.pem -subj "/CN=couchdb"
openssl x509 -req -in server-csr.pem -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial -out server-cert.pem -days 365
```

## Troubleshooting

### Connection Refused
- Ensure CouchDB is configured to listen on the TLS port
- Verify the `couchDBAddress` includes the correct port (typically 6984 for HTTPS)

### Certificate Verification Failed
- Check that the CA certificate matches the one used to sign the server certificate
- Verify certificate paths are correct and files are readable
- Ensure certificates haven't expired
- Check CouchDB server certificate CN/SAN matches the address

### TLS Handshake Timeout
- Check network connectivity between peer and CouchDB
- Verify firewall rules allow TLS traffic
- Increase `requestTimeout` if needed

## Security Best Practices

1. **Always use TLS in production** - Never transmit sensitive data over unencrypted connections
2. **Use proper CA certificates** - Verify server identity with trusted CA certificates
3. **Never skip certificate verification** - The `skipVerify` option should only be used for testing
4. **Protect CA certificates** - Ensure proper file permissions (e.g., 644) on CA cert files
5. **Use strong certificates** - Minimum 2048-bit RSA or 256-bit ECDSA keys
6. **Rotate certificates regularly** - Implement a certificate lifecycle management process
7. **Monitor certificate expiration** - Set up alerts before certificates expire

## Performance Considerations

- TLS adds minimal overhead (typically <5% latency increase)
- Connection pooling is maintained with TLS enabled
- TLS handshake occurs only on initial connection
- Consider using ECDSA certificates for better performance than RSA

## Logging

The peer logs TLS configuration at startup:

**INFO level logs:**
- TLS enabled/disabled status
- Connection URL (http vs https)
- CA certificate loading success

**DEBUG level logs:**
- HTTP client parameters
- TLS version and configuration
- Certificate verification settings

To enable DEBUG logging for CouchDB:
```bash
export FABRIC_LOGGING_SPEC=info:couchdb=debug
```

## References

- [CouchDB TLS Configuration](https://docs.couchdb.org/en/stable/config/http.html#https-ssl-tls-options)
- [Hyperledger Fabric Security](https://hyperledger-fabric.readthedocs.io/en/latest/security_model.html)
- [TLS Best Practices](https://wiki.mozilla.org/Security/Server_Side_TLS)