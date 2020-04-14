MSP Identity Validity Rules
==================================

As mentioned in MSP description, MSPs may be configured with a set of root
certificate authorities (rCAs), and optionally a set of intermediate
certificate authorities (iCAs). An MSP's iCA certificates must be signed
by **exactly one** of the MSP's rCAs or iCAs.
An MSP's configuration may contain a certificate revocation list, or CRL.
If any of the MSP's root certificate authorities are listed in the CRL,
then the MSP's configuration must not include any iCA that is also included
in the CRL, or the MSP setup will fail.

Each rCA is the root of a certification tree. That is,
each rCA may be the signer of the certificates of one or more iCAs, and these
iCAs will be the signer either of other iCAs or of user-certificates.
Here are a few examples::


              rCA1                rCA2         rCA3
            /    \                 |            |
         iCA1    iCA2             iCA3          id
          / \      |               |
      iCA11 iCA12 id              id
       |
      id

The default MSP implementation accepts as valid identities X.509 certificates
signed by the appropriate authorities. In the diagram above,
only certificates signed by iCA11, iCA12, iCA2, iCA3, and rCA3
will be considered valid. Certificates signed by internal nodes will be rejected.

Notice that the validity of a certificate is also affected, in a similar
way, if one or more organizational units are specified in the MSP configuration.
Recall that an organizational unit is specified in an MSP configuration
as a pair of two values, say (parent-cert, ou-string) representing the
certificate authority that certifies that organizational unit, and the
actual organizational unit identifier, respectively.
If a certificate C is signed by an iCA or rCA
for which an organizational unit has been specified in the MSP configuration,
then C is considered valid if, among other requirements, it includes
ou-string as part of its OU field.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
