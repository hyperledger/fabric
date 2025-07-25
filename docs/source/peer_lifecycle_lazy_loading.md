# Peer Lifecycle Lazy Loading

## Overview

The peer lifecycle cache can be configured to use lazy loading instead of pre-initializing all installed chaincodes at startup. This feature can significantly improve peer startup time when there are many installed chaincodes.

## Configuration

To enable lazy loading, add the following configuration to your `core.yaml` file:

```yaml
peer:
  lifecycle:
    lazyLoadEnabled: true
```

## How it works

### Default Behavior (lazyLoadEnabled: false)
- At startup, the peer loads all installed chaincode packages
- Parses metadata for each chaincode
- Populates the lifecycle cache with all chaincode information
- This can be slow when there are many installed chaincodes

### Lazy Loading Behavior (lazyLoadEnabled: true)
- At startup, the peer skips loading installed chaincode packages
- Chaincode information is loaded on-demand when needed
- The first time a chaincode is accessed, its package is loaded and cached
- Subsequent accesses use the cached information

## Benefits

- **Faster startup time**: Peer starts up much faster when there are many installed chaincodes
- **Reduced memory usage**: Only loads chaincode information when actually needed
- **Better resource utilization**: Avoids loading chaincodes that may never be used

## Considerations

- **First access delay**: The first time a chaincode is accessed, there may be a slight delay as the package is loaded
- **Cache behavior**: Once loaded, chaincode information remains cached for the lifetime of the peer
- **Installation events**: When new chaincodes are installed, they are still handled immediately via the `HandleChaincodeInstalled` event

## Use Cases

This feature is particularly useful in environments where:
- There are many installed chaincodes (hundreds or thousands)
- Not all installed chaincodes are actively used
- Fast peer startup time is critical
- Memory usage needs to be optimized

## Example

For a peer with 1000 installed chaincodes:
- **Default behavior**: May take 30-60 seconds to start up
- **Lazy loading**: May start up in 5-10 seconds, with chaincodes loaded as needed

## Migration

This feature is backward compatible. You can enable it on existing peers without any data migration or downtime. The feature can be toggled on/off by changing the configuration and restarting the peer. 