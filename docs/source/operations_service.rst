Enhanced Health Checks
=======================

Hyperledger Fabric provides enhanced health check endpoints for Kubernetes integration and operational monitoring. This feature extends the existing `/healthz` endpoint with readiness checks and detailed component status.

Endpoints
---------

**Liveness Probe: `/healthz`**
   - Returns basic liveness status
   - HTTP 200: Service is alive
   - HTTP 503: Service is unavailable
   - Unchanged from previous versions

**Readiness Probe: `/readyz`**
   - Returns readiness status for accepting traffic
   - HTTP 200: Service is ready (all components OK or DEGRADED)
   - HTTP 503: Service is not ready (at least one component UNAVAILABLE)
   - Checks component dependencies (gossip, ledger, orderer)
   - **Important**: Components reporting DEGRADED status do NOT cause readiness to fail.
     DEGRADED components are included in `/healthz/detailed` for informational purposes
     but do not block readiness (service can still accept traffic with reduced capability)

**Detailed Health: `/healthz/detailed`**
   - Returns comprehensive component-level status
   - Requires TLS/client authentication (if enabled)
   - HTTP 200: Service status (OK, DEGRADED, or UNAVAILABLE)
   - Provides detailed information about each component

Configuration
-------------

Health check configuration is located under ``operations.healthCheck`` in ``core.yaml``:

.. code-block:: yaml

   operations:
     healthCheck:
       timeout: 30s                    # Overall timeout for /healthz
       readinessTimeout: 10s           # Overall timeout for /readyz
       detailedHealth:
         enabled: false                # Enable /healthz/detailed endpoint
       gossip:
         enabled: true                 # Enable gossip readiness check
         minPeers: 0                    # Minimum connected peers required (default: 0)
                                       # minPeers = 0: Only check that gossip is initialized
                                       # minPeers > 0: Enforce minimum peer connectivity
         timeout: 5s                    # Timeout for gossip check
       ledger:
         enabled: true                 # Enable ledger readiness check
         failOnLag: false              # Fail if ledger is lagging (default: false)
         maxLag: 10                    # Maximum allowed lag in blocks
         timeout: 5s                    # Timeout for ledger check
       orderer:
         enabled: false                # Enable orderer connectivity check (default: false)
         timeout: 5s                    # Timeout for orderer check

Component Status
----------------

Each component reports one of three status levels:

- **OK**: Component is healthy and operational
- **DEGRADED**: Component is functional but operating below optimal conditions
- **UNAVAILABLE**: Component is not available or not ready

Gossip Checker
--------------

The gossip checker verifies that the peer is connected to a minimum number of peers in the network. This ensures the peer can participate in gossip-based block dissemination.

**Configuration:**
- ``minPeers``: Minimum number of connected peers required (default: 1)
- If ``minPeers`` is 0, the check only verifies that gossip service is initialized

**Status:**
- **OK**: Connected to more than minimum required peers (or minPeers = 0 and gossip initialized)
- **DEGRADED**: Connected to exactly minimum required peers (informational only, does not fail readiness)
- **UNAVAILABLE**: Connected to fewer than minimum required peers or gossip service not initialized

**Default Behavior (minPeers = 0):**
- Treats minPeers = 0 as "gossip initialized but no connectivity requirement"
- Only checks that gossip service is initialized
- Does not enforce peer count, avoiding false negatives in dev and single-node setups
- Only enforce peer count when minPeers > 0

Ledger Checker
--------------

The ledger checker verifies that all channel ledgers are accessible and readable. By default, it does NOT check for ledger lag (blocks behind gossip-advertised height).

**Configuration:**
- ``failOnLag``: If true, fails readiness when ledger lags behind gossip-advertised height (default: false)
- ``maxLag``: Maximum allowed lag in blocks when ``failOnLag`` is enabled (default: 10)

**Readiness Criteria:**
- Ledger is initialized: Ledger manager and channel ledgers are available
- Ledger is readable: Can successfully read blockchain info and create query executors
- Ledger lag checking is informational by default: Does NOT fail readiness due to lag unless failOnLag is explicitly enabled

**Status:**
- **OK**: All channel ledgers are accessible
- **UNAVAILABLE**: One or more channel ledgers are not accessible or readable
- **DEGRADED**: One or more channels are lagging (only if ``failOnLag`` is enabled)

**Note:** Ledger lag checking is disabled by default to avoid false negatives during normal network operations. Readiness must not fail due to lag unless ``failOnLag: true``. Only enable ``failOnLag`` if you understand the implications.

Orderer Checker
---------------

The orderer checker verifies orderer connectivity for channels that require it. This check is **disabled by default** as orderer connectivity issues may not prevent read-only operations.

**Configuration:**
- ``enabled``: Enable orderer connectivity checks (default: false)

**Status:**
- **OK**: Orderer connectivity is available (or check is disabled)
- **UNAVAILABLE**: Orderer connectivity is required but not available

**Warning:** Enabling this check may cause false negatives. Only enable if you need to ensure write-readiness.

Kubernetes Integration
----------------------

Configure Kubernetes probes as follows:

.. code-block:: yaml

   livenessProbe:
     httpGet:
       path: /healthz
       port: 9443
     initialDelaySeconds: 10
     periodSeconds: 30

   readinessProbe:
     httpGet:
       path: /readyz
       port: 9443
     initialDelaySeconds: 5
     periodSeconds: 10

Security Considerations
-----------------------

- The ``/healthz`` and ``/readyz`` endpoints are publicly accessible (no TLS required)
- The ``/healthz/detailed`` endpoint requires TLS/client authentication when enabled
- Detailed health information may expose internal topology - use with caution

Backward Compatibility
----------------------

- Existing ``/healthz`` endpoint behavior is unchanged
- All new features are opt-in via configuration
- Default configuration maintains backward compatibility
