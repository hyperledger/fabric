Identity Mixer MSP configuration generator (idemixgen)
======================================================

This document describes the usage for the ``idemixgen`` utility, which can be
used to create configuration files for the identity mixer based MSP.
Two commands are available, one for creating a fresh CA key pair, and one
for creating an MSP config using a previously generated CA key.

Directory Structure
-------------------

The ``idemixgen`` tool will create directories with the following structure:

.. code:: bash

    - /ca/
        IssuerSecretKey
        IssuerPublicKey
        RevocationKey
    - /msp/
        IssuerPublicKey
        RevocationPublicKey
    - /user/
        SignerConfig

The ``ca`` directory contains the issuer secret key (including the revocation key) and should only be present
for a CA. The ``msp`` directory contains the information required to set up an
MSP verifying idemix signatures. The ``user`` directory specifies a default
signer.

CA Key Generation
-----------------

CA (issuer) keys suitable for identity mixer can be created using command
``idemixgen ca-keygen``. This will create directories ``ca`` and ``msp`` in the
working directory.

Adding a Default Signer
-----------------------
After generating the ``ca`` and ``msp`` directories with
``idemixgen ca-keygen``, a default signer specified in the ``user`` directory
can be added to the config with ``idemixgen signerconfig``.

.. code:: bash

    $ idemixgen signerconfig -h
    usage: idemixgen signerconfig [<flags>]

    Generate a default signer for this Idemix MSP

    Flags:
        -h, --help               Show context-sensitive help (also try --help-long and --help-man).
        -u, --org-unit=ORG-UNIT  The Organizational Unit of the default signer
        -a, --admin              Make the default signer admin
        -e, --enrollment-id=ENROLLMENT-ID
                                 The enrollment id of the default signer
        -r, --revocation-handle=REVOCATION-HANDLE
                                 The handle used to revoke this signer

For example, we can create a default signer that is a member of organizational
unit "OrgUnit1", with enrollment identity "johndoe", revocation handle "1234",
and that is an admin, with the following command:

.. code:: bash

    idemixgen signerconfig -u OrgUnit1 --admin -e "johndoe" -r 1234

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
