Pluggable transaction endorsement and validation
================================================

Motivation
----------

When a transaction is validated at time of commit, the peer performs various
checks before applying the state changes that come with the transaction itself:

- Validating the identities that signed the transaction.
- Verifying the signatures of the endorsers on the transaction.
- Ensuring the transaction satisfies the endorsement policies of the namespaces
  of the corresponding chaincodes.

There are use cases which demand custom transaction validation rules different
from the default Fabric validation rules, such as:

- **UTXO (Unspent Transaction Output):** When the validation takes into account
  whether the transaction doesn't double spend its inputs.
- **Anonymous transactions:** When the endorsement doesn't contain the identity
  of the peer, but a signature and a public key are shared that can't be linked
  to the peer's identity.

Pluggable endorsement and validation logic
------------------------------------------

Fabric allows for the implementation and deployment of custom endorsement and
validation logic into the peer to be associated with chaincode handling in a
pluggable manner. This logic can be either compiled into the peer as built in
selectable logic, or compiled and deployed alongside the peer as a
`Go plugin <https://golang.org/pkg/plugin/>`_.

By default, A chaincode will use the built in endorsement and validation logic.
However, users have the option of selecting custom endorsement and validation
plugins as part of the chaincode definition. An administrator can extend the
endorsement/validation logic available to the peer by customizing the peer's
local configuration.

Configuration
-------------

Each peer has a local configuration (``core.yaml``) that declares a mapping
between the endorsement/validation logic name and the implementation that is to
be run.

The default logic are called ``ESCC`` (with the "E" standing for endorsement) and
``VSCC`` (validation), and they can be found in the peer local configuration in
the ``handlers`` section:

.. code-block:: YAML

    handlers:
        endorsers:
          escc:
            name: DefaultEndorsement
        validators:
          vscc:
            name: DefaultValidation

When the endorsement or validation implementation is compiled into the peer, the
``name`` property represents the initialization function that is to be run in order
to obtain the factory that creates instances of the endorsement/validation logic.

The function is an instance method of the ``HandlerLibrary`` construct under
``core/handlers/library/library.go`` and in order for custom endorsement or
validation logic to be added, this construct needs to be extended with any
additional methods.

Since this is cumbersome and poses a deployment challenge, one can also deploy
custom endorsement and validation as a Go plugin by adding another property
under the ``name`` called ``library``.

For example, if we have custom endorsement and validation logic which is
implemented as a plugin, we would have the following entries in the configuration
in ``core.yaml``:

.. code-block:: YAML

    handlers:
        endorsers:
          escc:
            name: DefaultEndorsement
          custom:
            name: customEndorsement
            library: /etc/hyperledger/fabric/plugins/customEndorsement.so
        validators:
          vscc:
            name: DefaultValidation
          custom:
            name: customValidation
            library: /etc/hyperledger/fabric/plugins/customValidation.so

And we'd have to place the ``.so`` plugin files in the peer's local file system.

The name of the custom plugin needs to be referenced by the chaincode definition
to be used by the chaincode. If you are using the peer CLI to approve the
chaincode definition, use the ``--escc`` and ``--vscc`` flag to select the name
of the custom endorsement or validation library. If you are using the
Fabric SDK for Node.js, visit `How to install and start your chaincode <https://hyperledger.github.io/fabric-sdk-node/{BRANCH}/tutorial-chaincode-lifecycle.html>`__.
For more information, see :doc:`chaincode_lifecycle`.

.. note:: Hereafter, custom endorsement or validation logic implementation is
          going to be referred to as "plugins", even if they are compiled into
          the peer.

Endorsement plugin implementation
---------------------------------

To implement an endorsement plugin, one must implement the ``Plugin`` interface
found in ``core/handlers/endorsement/api/endorsement.go``:

.. code-block:: Go

    // Plugin endorses a proposal response
    type Plugin interface {
    	// Endorse signs the given payload(ProposalResponsePayload bytes), and optionally mutates it.
    	// Returns:
    	// The Endorsement: A signature over the payload, and an identity that is used to verify the signature
    	// The payload that was given as input (could be modified within this function)
    	// Or error on failure
    	Endorse(payload []byte, sp *peer.SignedProposal) (*peer.Endorsement, []byte, error)

    	// Init injects dependencies into the instance of the Plugin
    	Init(dependencies ...Dependency) error
    }

An endorsement plugin instance of a given plugin type (identified either by the
method name as an instance method of the ``HandlerLibrary`` or by the plugin ``.so``
file path) is created for each channel by having the peer invoke the ``New``
method in the ``PluginFactory`` interface which is also expected to be implemented
by the plugin developer:

.. code-block:: Go

    // PluginFactory creates a new instance of a Plugin
    type PluginFactory interface {
    	New() Plugin
    }


The ``Init`` method is expected to receive as input all the dependencies declared
under ``core/handlers/endorsement/api/``, identified as embedding the ``Dependency``
interface.

After the creation of the ``Plugin`` instance, the ``Init`` method is invoked on
it by the peer with the ``dependencies`` passed as parameters.

Currently Fabric comes with the following dependencies for endorsement plugins:

- ``SigningIdentityFetcher``: Returns an instance of ``SigningIdentity`` based
  on a given signed proposal:

.. code-block:: Go

    // SigningIdentity signs messages and serializes its public identity to bytes
    type SigningIdentity interface {
    	// Serialize returns a byte representation of this identity which is used to verify
    	// messages signed by this SigningIdentity
    	Serialize() ([]byte, error)

    	// Sign signs the given payload and returns a signature
    	Sign([]byte) ([]byte, error)
    }

- ``StateFetcher``: Fetches a **State** object which interacts with the world
  state:

.. code-block:: Go

    // State defines interaction with the world state
    type State interface {
    	// GetPrivateDataMultipleKeys gets the values for the multiple private data items in a single call
    	GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([][]byte, error)

    	// GetStateMultipleKeys gets the values for multiple keys in a single call
    	GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error)

    	// GetTransientByTXID gets the values private data associated with the given txID
    	GetTransientByTXID(txID string) ([]*rwset.TxPvtReadWriteSet, error)

    	// Done releases resources occupied by the State
    	Done()
     }

Validation plugin implementation
--------------------------------

To implement a validation plugin, one must implement the ``Plugin`` interface
found in ``core/handlers/validation/api/validation.go``:

.. code-block:: Go

    // Plugin validates transactions
    type Plugin interface {
    	// Validate returns nil if the action at the given position inside the transaction
    	// at the given position in the given block is valid, or an error if not.
    	Validate(block *common.Block, namespace string, txPosition int, actionPosition int, contextData ...ContextDatum) error

    	// Init injects dependencies into the instance of the Plugin
    	Init(dependencies ...Dependency) error
    }

Each ``ContextDatum`` is additional runtime-derived metadata that is passed by
the peer to the validation plugin. Currently, the only ``ContextDatum`` that is
passed is one that represents the endorsement policy of the chaincode:

.. code-block:: Go

   // SerializedPolicy defines a serialized policy
  type SerializedPolicy interface {
	validation.ContextDatum

	// Bytes returns the bytes of the SerializedPolicy
	Bytes() []byte
   }

A validation plugin instance of a given plugin type (identified either by the
method name as an instance method of the ``HandlerLibrary`` or by the plugin ``.so``
file path) is created for each channel by having the peer invoke the ``New``
method in the ``PluginFactory`` interface which is also expected to be implemented
by the plugin developer:

.. code-block:: Go

    // PluginFactory creates a new instance of a Plugin
    type PluginFactory interface {
    	New() Plugin
    }

The ``Init`` method is expected to receive as input all the dependencies declared
under ``core/handlers/validation/api/``, identified as embedding the ``Dependency``
interface.

After the creation of the ``Plugin`` instance, the **Init** method is invoked on
it by the peer with the dependencies passed as parameters.

Currently Fabric comes with the following dependencies for validation plugins:

- ``IdentityDeserializer``: Converts byte representation of identities into
  ``Identity`` objects that can be used to verify signatures signed by them, be
  validated themselves against their corresponding MSP, and see whether they
  satisfy a given **MSP Principal**. The full specification can be found in
  ``core/handlers/validation/api/identities/identities.go``.

- ``PolicyEvaluator``: Evaluates whether a given policy is satisfied:

.. code-block:: Go

    // PolicyEvaluator evaluates policies
    type PolicyEvaluator interface {
    	validation.Dependency

    	// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies
    	// the policy with the given bytes
    	Evaluate(policyBytes []byte, signatureSet []*common.SignedData) error
    }

- ``StateFetcher``: Fetches a ``State`` object which interacts with the world state:

.. code-block:: Go

    // State defines interaction with the world state
    type State interface {
        // GetStateMultipleKeys gets the values for multiple keys in a single call
        GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error)

        // GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
        // startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
        // and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
        // can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
        // The returned ResultsIterator contains results of type *KV which is defined in fabric-protos/ledger/queryresult.
        GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)

        // GetStateMetadata returns the metadata for given namespace and key
        GetStateMetadata(namespace, key string) (map[string][]byte, error)

        // GetPrivateDataMetadata gets the metadata of a private data item identified by a tuple <namespace, collection, key>
        GetPrivateDataMetadata(namespace, collection, key string) (map[string][]byte, error)

        // Done releases resources occupied by the State
        Done()
    }

Important notes
---------------

- **Validation plugin consistency across peers:** In future releases, the Fabric
  channel infrastructure would guarantee that the same validation logic is used
  for a given chaincode by all peers in the channel at any given blockchain
  height in order to eliminate the chance of mis-configuration which would might
  lead to state divergence among peers that accidentally run different
  implementations. However, for now it is the sole responsibility of the system
  operators and administrators to ensure this doesn't happen.

- **Validation plugin error handling:** Whenever a validation plugin can't
  determine whether a given transaction is valid or not, because of some transient
  execution problem like inability to access the database, it should return an
  error of type **ExecutionFailureError** that is defined in ``core/handlers/validation/api/validation.go``.
  Any other error that is returned, is treated as an endorsement policy error
  and marks the transaction as invalidated by the validation logic. However,
  if an ``ExecutionFailureError`` is returned, the chain processing halts instead
  of marking the transaction as invalid. This is to prevent state divergence
  between different peers.

- **Error handling for private metadata retrieval**: In case a plugin retrieves
  metadata for private data by making use of the ``StateFetcher`` interface,
  it is important that errors are handled as follows: ``CollConfigNotDefinedError``
  and ``InvalidCollNameError``, signalling that the specified collection does
  not exist, should be handled as deterministic errors and should not lead the
  plugin to return an ``ExecutionFailureError``.

- **Importing Fabric code into the plugin**: Importing code that belongs to Fabric
  other than protobufs as part of the plugin is highly discouraged, and can lead
  to issues when the Fabric code changes between releases, or can cause inoperability
  issues when running mixed peer versions. Ideally, the plugin code should only
  use the dependencies given to it, and should import the bare minimum other
  than protobufs.

  .. Licensed under Creative Commons Attribution 4.0 International License
     https://creativecommons.org/licenses/by/4.0/
