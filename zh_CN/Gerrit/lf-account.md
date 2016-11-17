# Requesting a Linux Foundation Account

Contributions to the Fabric code base require a Linux Foundation account.
Follow the steps below to create a Linux Foundation account.

## Creating a Linux Foundation ID

1. Go to the [Linux Foundation ID website](https://identity.linuxfoundation.org/).

2. Select the option `I need to create a Linux Foundation ID`.

3. Fill out the form that appears:

4. Open your email account and look for a message with the subject line:
   "Validate your Linux Foundation ID email".

5. Open the received URL to validate your email address.

6. Verify the browser displays the message `You have successfully
   validated your e-mail address`.

7. Access `Gerrit` by selecting `Sign In`:

8. Use your Linux Foundation ID to Sign In:

## Configuring Gerrit to Use SSH

Gerrit uses SSH to interact with your Git client. A SSH private key
needs to be generated on the development machine with a matching public
key on the Gerrit server.

If you already have a SSH key-pair, skip this section.

As an example, we provide the steps to generate the SSH key-pair on a Linux
environment. Follow the equivalent steps on your OS.

1. Create a key-pair, enter:

```
ssh-keygen -t rsa -C "John Doe john.doe@example.com"
```

**Note:** This will ask you for a password to protect the private key as it
generates a unique key. Please keep this password private, and DO NOT
enter a blank password.

The generated key-pair is found in: `~/.ssh/id_rsa` and `~/.ssh/id_rsa.pub`.

1. Add the private key in the `id_rsa` file in your key ring, e.g.:

```
ssh-add ~/.ssh/id_rsa
```

Once the key-pair has been generated, the public key must be added to Gerrit.

Follow these steps to add your public key `id_rsa.pub` to the Gerrit
account:

1. Go to [Gerrit](https://gerrit.hyperledger.org/r/#/admin/projects/fabric).

2. Click on your account name in the upper right corner.

3. From the pop-up menu, select `Settings`.

4. On the left side menu, click on `SSH Public Keys`.

5. Paste the contents of your public key `~/.ssh/id_rsa.pub` and click
   `Add key`.

**Note:** The `id_rsa.pub` file can be opened with any text editor. Ensure
   that all the contents of the file are selected, copied and pasted into the
   `Add SSH key` window in Gerrit.

**Note:** The ssh key generation instructions operate on the assumtion that
you are using the default naming. It is possible to generate multiple ssh Keys
and to name the resulting files differently. See the [ssh-keygen](https://en.wikipedia.org/wiki/Ssh-keygen)
documentation for details on how to do that. Once you have generated non-default
keys, you need to configure ssh to use the correct key for Gerrit. In that case,
you need to create a `~/.ssh/config` file modeled after the one below.

```
host gerrit.hyperledger.org
 HostName gerrit.hyperledger.org
 IdentityFile ~/.ssh/id_rsa_hyperledger_gerrit
 User <LFID>
```
where <LFID> is your Linux Foundation ID and the value of IdentityFile is the
name of the public key file you generated.

**Warning:** Potential Security Risk! Do not copy your private key
   `~/.ssh/id_rsa` Use only the public `~/.ssh/id_rsa.pub`.

## Checking Out the Source Code

1. Ensure that SSH has been set up properly. See
   `Configuring Gerrit to Use SSH` for details.

2. Clone the repository with your Linux Foundation ID (<LFID>):

```
git clone ssh://<LFID>@gerrit.hyperledger.org:29418/fabric fabric
```

You have successfully checked out a copy of the source code to your local
machine.
