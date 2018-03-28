Requesting a Linux Foundation Account
=====================================

Contributions to the Hyperledger Fabric code base require a
`Linux Foundation <https://linuxfoundation.org/>`__
account --- follow the steps below to create one if you don't
already have one.

Creating a Linux Foundation ID
------------------------------

1. Go to the `Linux Foundation ID
   website <https://identity.linuxfoundation.org/>`__.

2. Select the option ``I need to create a Linux Foundation ID``, and fill
   out the form that appears.

3. Wait a few minutes, then look for an email message with the subject line:
   "Validate your Linux Foundation ID email".

4. Open the received URL to validate your email address.

5. Verify that your browser displays the message
   ``You have successfully validated your e-mail address``.

6. Access Gerrit by selecting ``Sign In``, and use your new
   Linux Foundation account ID to sign in.

Configuring Gerrit to Use SSH
-----------------------------

Gerrit uses SSH to interact with your Git client. If you already have an SSH
key pair, you can skip the part of this section that explains how to generate one.

What follows explains how to generate an SSH key pair in a Linux environment ---
follow the equivalent steps on your OS.

First, create an SSH key pair with the command:

::

    ssh-keygen -t rsa -C "John Doe john.doe@example.com"

**Note:** This will ask you for a password to protect the private key as
it generates a unique key. Please keep this password private, and DO NOT
enter a blank password.

The generated SSH key pair can be found in the files ``~/.ssh/id_rsa`` and
``~/.ssh/id_rsa.pub``.

Next, add the private key in the ``id_rsa`` file to your key ring, e.g.:

::

    ssh-add ~/.ssh/id_rsa

Finally, add the public key of the generated key pair to the Gerrit server,
with the following steps:

1. Go to
   `Gerrit <https://gerrit.hyperledger.org/r/#/admin/projects/fabric>`__.

2. Click on your account name in the upper right corner.

3. From the pop-up menu, select ``Settings``.

4. On the left side menu, click on ``SSH Public Keys``.

5. Paste the contents of your public key ``~/.ssh/id_rsa.pub`` and click
   ``Add key``.

**Note:** The ``id_rsa.pub`` file can be opened with any text editor.
Ensure that all the contents of the file are selected, copied and pasted
into the ``Add SSH key`` window in Gerrit.

**Note:** The SSH key generation instructions operate on the assumption
that you are using the default naming. It is possible to generate
multiple SSH keys and to name the resulting files differently. See the
`ssh-keygen <https://en.wikipedia.org/wiki/Ssh-keygen>`__ documentation
for details on how to do that. Once you have generated non-default keys,
you need to configure SSH to use the correct key for Gerrit. In that
case, you need to create a ``~/.ssh/config`` file modeled after the one
below.

::

    host gerrit.hyperledger.org
     HostName gerrit.hyperledger.org
     IdentityFile ~/.ssh/id_rsa_hyperledger_gerrit
     User <LFID>

where <LFID> is your Linux Foundation ID and the value of IdentityFile is the
name of the public key file you generated.

**Warning:** Potential Security Risk! Do not copy your private key
``~/.ssh/id_rsa``. Use only the public ``~/.ssh/id_rsa.pub``.

Checking Out the Source Code
----------------------------

Once you've set up SSH as explained in the previous section, you can clone
the source code repository with the command:

::

    git clone ssh://<LFID>@gerrit.hyperledger.org:29418/fabric fabric

You have now successfully checked out a copy of the source code to your
local machine.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

