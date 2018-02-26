Cryptogen Commands
==================

Cryptogen is an utility for generating Hyperledger Fabric key material.
It is mainly meant to be used for testing environment.

Syntax
^^^^^^

The ``cryptogen`` command has different subcommands within it:

.. code:: bash

  cryptogen [subcommand]

as follows

.. code:: bash

  cryptogen generate
  cryptogen showtemplate
  cryptogen version
  cryptogen extend
  cryptogen help
  cryptogen

These subcommands separate the different functions provided by they utility.

Within each subcommand there are many different options available and because of
this, each is described in its own section in this topic.

If a command option is not specified then ``cryptogen`` will return some high level
help text as described in in the ``--help`` flag below.

``cryptogen`` flags
^^^^^^^^^^^^^^^^^^^

The ``cryptogen`` command also has a set of associated flags:

.. code:: bash

  cryptogen [flags]

as follows

.. code:: bash

  cryptogen --help
  cryptogen generate --help

These flags provide more information about cryptogen, and are designated *global*
because they can be used at any command level. For example the ``--help`` flag can
provide help on the ``cryptogen`` command, the ``cryptogen generate`` command, as well as their
respective options.

Flag details
^^^^^^^^^^^^

* ``--help``

  Use `help` to get brief help text for the ``cryptogen`` command. The ``help`` flag can
  often be used at different levels to get individual command help, or even a
  help on a command option. See individual commands for more detail.

Usage
^^^^^

Here's some examples using the different available flags on the `peer` command.

* ``--help`` flag

.. code:: bash

   cryptogen --help

   usage: cryptogen [<flags>] <command> [<args> ...]

   Utility for generating Hyperledger Fabric key material

   Flags:
     --help  Show context-sensitive help (also try --help-long and --help-man).

   Commands:
     help [<command>...]
       Show help.

     generate [<flags>]
       Generate key material

     showtemplate
       Show the default configuration template

     version
       Show version information

     extend [<flags>]
       Extend existing network


The ``cryptogen generate`` Command
----------------------------------

The ``cryptogen generate`` command allows the generation of the key material.

Syntax
^^^^^^

The ``cryptogen generate`` command has the following syntax:

.. code:: bash

  cryptogen generate [<flags>]


``cryptogen generate`` flags
^^^^^^^^^^^^^^^^^^^^^^^^^^^^

The ``cryptogen generate`` command has different flags available to it, and because of
this, each flag is described in the relevant command topic.

.. code:: bash

  cryptogen generate [flags]

as follows

.. code:: bash

  cryptogen generate --output="crypto-config"
  cryptogen generate --config=CONFIG

The global ``cryptogen`` command flags also apply as described in the `cryptogen command`
flags:

* ``--help``

Flag details
^^^^^^^^^^^^

* ``--output="crypto-config"``

  the output directory in which to place artifacts.

* ``--config=CONFIG``

  the configuration template to use.

Usage
^^^^^

Here's some examples using the different available flags on the ``cryptogen generate``
command.

.. code:: bash

    ./cryptogen generate --output="crypto-config"

    org1.example.com
    org2.example.com

The ``cryptogen showtemplate`` command
--------------------------------------

The ``cryptogen showtemplate`` command shows the default configuration template.

Syntax
^^^^^^

The ``cryptogen showtemplate`` command has the following syntax:

.. code:: bash

  cryptogen showtemplate

Usage
^^^^^

Output from the ``cryptogen showtemplate`` command is  following:

.. code:: bash

    cryptogen showtemplate

    # ---------------------------------------------------------------------------
    # "OrdererOrgs" - Definition of organizations managing orderer nodes
    # ---------------------------------------------------------------------------
    OrdererOrgs:
      # ---------------------------------------------------------------------------
      # Orderer
      # ---------------------------------------------------------------------------
      - Name: Orderer
        Domain: example.com

        # ---------------------------------------------------------------------------
        # "Specs" - See PeerOrgs below for complete description
        # ---------------------------------------------------------------------------
        Specs:
          - Hostname: orderer

    # ---------------------------------------------------------------------------
    # "PeerOrgs" - Definition of organizations managing peer nodes
    # ---------------------------------------------------------------------------
    PeerOrgs:
      # ---------------------------------------------------------------------------
      # Org1
      # ---------------------------------------------------------------------------
      - Name: Org1
        Domain: org1.example.com
        EnableNodeOUs: false

        # ---------------------------------------------------------------------------
        # "CA"
        # ---------------------------------------------------------------------------
        # Uncomment this section to enable the explicit definition of the CA for this
        # organization.  This entry is a Spec.  See "Specs" section below for details.
        # ---------------------------------------------------------------------------
        # CA:
        #    Hostname: ca # implicitly ca.org1.example.com
        #    Country: US
        #    Province: California
        #    Locality: San Francisco
        #    OrganizationalUnit: Hyperledger Fabric
        #    StreetAddress: address for org # default nil
        #    PostalCode: postalCode for org # default nil

        # ---------------------------------------------------------------------------
        # "Specs"
        # ---------------------------------------------------------------------------
        # Uncomment this section to enable the explicit definition of hosts in your
        # configuration.  Most users will want to use Template, below
        #
        # Specs is an array of Spec entries.  Each Spec entry consists of two fields:
        #   - Hostname:   (Required) The desired hostname, sans the domain.
        #   - CommonName: (Optional) Specifies the template or explicit override for
        #                 the CN.  By default, this is the template:
        #
        #                              "{{.Hostname}}.{{.Domain}}"
        #
        #                 which obtains its values from the Spec.Hostname and
        #                 Org.Domain, respectively.
        #   - SANS:       (Optional) Specifies one or more Subject Alternative Names
        #                 to be set in the resulting x509. Accepts template
        #                 variables {{.Hostname}}, {{.Domain}}, {{.CommonName}}. IP
        #                 addresses provided here will be properly recognized. Other
        #                 values will be taken as DNS names.
        #                 NOTE: Two implicit entries are created for you:
        #                     - {{ .CommonName }}
        #                     - {{ .Hostname }}
        # ---------------------------------------------------------------------------
        # Specs:
        #   - Hostname: foo # implicitly "foo.org1.example.com"
        #     CommonName: foo27.org5.example.com # overrides Hostname-based FQDN set above
        #     SANS:
        #       - "bar.{{.Domain}}"
        #       - "altfoo.{{.Domain}}"
        #       - "{{.Hostname}}.org6.net"
        #       - 172.16.10.31
        #   - Hostname: bar
        #   - Hostname: baz

        # ---------------------------------------------------------------------------
        # "Template"
        # ---------------------------------------------------------------------------
        # Allows for the definition of 1 or more hosts that are created sequentially
        # from a template. By default, this looks like "peer%d" from 0 to Count-1.
        # You may override the number of nodes (Count), the starting index (Start)
        # or the template used to construct the name (Hostname).
        #
        # Note: Template and Specs are not mutually exclusive.  You may define both
        # sections and the aggregate nodes will be created for you.  Take care with
        # name collisions
        # ---------------------------------------------------------------------------
        Template:
          Count: 1
          # Start: 5
          # Hostname: {{.Prefix}}{{.Index}} # default
          # SANS:
          #   - "{{.Hostname}}.alt.{{.Domain}}"

        # ---------------------------------------------------------------------------
        # "Users"
        # ---------------------------------------------------------------------------
        # Count: The number of user accounts _in addition_ to Admin
        # ---------------------------------------------------------------------------
        Users:
          Count: 1

      # ---------------------------------------------------------------------------
      # Org2: See "Org1" for full specification
      # ---------------------------------------------------------------------------
      - Name: Org2
        Domain: org2.example.com
        EnableNodeOUs: false
        Template:
          Count: 1
        Users:
          Count: 1

The ``cryptogen extend`` Command
--------------------------------

The ``cryptogen extend`` command allows to extend an existing network, meaning
generating all the additional key material needed by the new added entities.

Syntax
^^^^^^

The ``cryptogen extend`` command has the following syntax:

.. code:: bash

  cryptogen extend [<flags>]


``cryptogen extend`` flags
^^^^^^^^^^^^^^^^^^^^^^^^^^

The ``cryptogen extend`` command has different flags available to it, and because of
this, each flag is described in the relevant command topic.

.. code:: bash

  cryptogen extend [flags]

as follows

.. code:: bash

  cryptogen extend --input="crypto-config"
  cryptogen extend --config=CONFIG

The global ``cryptogen`` command flags also apply as described in the `cryptogen command`
flags:

* ``--help``

Flag details
^^^^^^^^^^^^

* ``--input="crypto-config"``

  the output directory in which to place artifacts.

* ``--config=CONFIG``

  the configuration template to use.

Usage
^^^^^

Here's some examples using the different available flags on the ``cryptogen extend``
command.

.. code:: bash

    cryptogen extend --input="crypto-config" --config=config.yaml

    org3.example.com

Where config.yaml add a new peer organization called ``org3.example.com``


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
