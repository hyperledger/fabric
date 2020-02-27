# Planning for a CA

**Audience**: Architects, network operators, users setting up a production Fabric network and are familiar with Transport Layer Security (TLS), Public Key Infrastructure (PKI) and Membership Service Providers (MSPs).

These deployment instructions provide guidance for how to deploy a CA for a Production network. If you need to quickly stand up a network for education or testing purposes, check out the [Fabric test network](../test_network.html). While the Fabric CA server remains a preferred and tested certificate authority for Hyperledger Fabric, you can instead use certificates from a non-Fabric CA with your Fabric network; however, the scope of this deployment guide is focused on using a Fabric CA. It focuses on the most important configuration parameters you need to consider and provides best practices for configuring a CA.

You may already be familiar with the Fabric CA [User's Guide](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/users-guide.html) and the [Operations Guide](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/operations_guide.html). This topic is intended to inform your decisions before deploying a CA and provide guidance on how to configure the CA parameters based on those decisions. You may still need to reference those topics when you make your decisions.

Recall that a Fabric CA performs the following functions on a blockchain network:
- Registration of identities, or connect to LDAP as the user registry.
- Issuance of Enrollment Certificates (ECerts). Enrollment is a process whereby the Fabric CA issues a certificate key-pair, comprised of a signing certificate and a private key that forms the identity. The private and public key are first generated locally by the Fabric CA client, and then the public key is sent to the CA which returns an encoded certificate, the signing certificate.
- Certificate renewal and revocation.

You have the opportunity to customize the behavior of these functions. The first time the CA is started, it looks for a [fabric-ca-server-config.yaml
file](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/serverconfig.html) which contains the CA configuration parameters. If the file is not there, a default one is created for you. Before you deploy your CA, this topic provides guidance around the parameters in that file and the decisions you need to make in order to customize the CA according to your use case.

## What CA topology will you use on your network?

The topology of CAs on your network can vary depending on how many organizations participate on the network and how you prefer to administer your CAs.

### How many CAs are required?

Before configuring a CA, you need to understand how many CAs are required for your network. If you read the Deployment Process Overview, you'll recall that it is recommended that you deploy two CAs per organization, an enrollment CA and a TLS CA. TLS communications are required for any production network to secure communications between nodes in the organization. Thus, it is the TLS CA that issues those TLS certificates. The enrollment CA on the other hand is used to generate organization and node identities. Also, because this is a _distributed ledger_, the ordering service should not be part of the same organization as the peers, so you will need separate organizations (and therefore CAs) for your peer organizations and ordering service organization. And if multiple organizations contribute nodes to an ordering service, each ordering node would have its own organization CA.

#### Why is a separate TLS server recommended?

A separate TLS server provides an independent chain of trust just for securing communications. Most organizations prefer that the TLS communications are secured by separate crypto material -- from a different root, either from a separate Fabric TLS CA or another external Certificate Authority.

One option that Fabric provides is the ability to configure a dual-headed CA, a single CA that under the covers includes an enrollment CA and a TLS CA. They operate on the same CA node and port but are addressable by a different CA name. The `cafiles` parameter discussed in more detail later in this topic allows each CA to have its own configuration but is beneficial when you want both CAs to share the same backend database. Thus with this option, conveniently, when the enrollment CA is deployed, the TLS CA is automatically deployed for you along side it.

It is also worth noting that from a functional perspective there is no difference between a Fabric root CA and a Fabric TLS CA. The difference lies in types of certificates they generate.

#### When would I want an Intermediate CA?

Intermediate CAs are optional. For added security, organizations can deploy a chain of CAs known as intermediate CAs. An intermediate CA has their root certificate issued by the root CA or another intermediate authority (that becomes the parent CA), which establishes a “chain of trust” for any certificate that is issued by any CA in the chain. Therefore, having one or more intermediate CAs allows you to protect your root of trust.  This ability to track back to the root CA not only allows the function of CAs to scale while still providing security — allowing organizations that consume certificates to use intermediate CAs with confidence — it limits the exposure of the root CA, which, if compromised, would endanger the entire chain of trust. If an intermediate CA is compromised, on the other hand, there will be a much smaller exposure. A key benefit is that after the intermediate CAs are up and running, the root CA can be effectively turned off, limiting its vulnerability even more.

Another reason to include an intermediate CA would be when you have a very large organization with with multiple departments and you don’t want a single CA to generate certificates for all of the depts. Intermediate CAs provide a mechanism for scoping the certificates that a CA manages to a smaller department or sub-group. Intermediate CAs are not required, but they mitigate risk and scope certificate management. This pattern incurs significant overheard if there will only be a small number of members in the organization. As an alternative to deploying multiple intermediate CAs, you can configure a CA with `affiliations` (similar to departments) instead. More on this later when we talk about `affiliations`.

Also, in situations where it is acceptable for one TLS CA to be used to issue certificates to secure communications for an entire organization, it would be reasonable for intermediate CAs to use the same TLS CA as the root CA rather than having their own dedicated TLS CA.

### In what order should the CAs be deployed?

If a dual-headed CA is not configured with an enrollment CA and TLS CA, then the TLS CA is deployed separately, and needs to be deployed _before_ the enrollment CA in order to generate the TLS certificate for the enrollment CA. After the TLS CA is deployed, then the enrollment CA can be deployed followed by any intermediate CAs if required.

## A note about configuration

There are three ways to configure settings on the Fabric CA server and client. The precedence order for overriding the default settings is:

1. Use the [Fabric CA server CLI commands](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/clientcli.html).
2. Use environment variables to override configuration file settings.
3. Modify the configuration file.

However, the recommendation is that before starting the server, you **make the modifications to the configuration file** to become familiar with the available settings and how they work.  Note that some configuration settings are stored in the CA database; therefore, after a CA is started, overriding the settings can no longer be performed by editing the configuration file or by setting environment variables. Affected parameters are noted throughout these instructions. In these cases, the modifications are required to be made by using the Fabric CA server CLI commands and have the added benefit of not requiring a server restart. 

The use of environment variables to modify configuration settings is discouraged because the changes are not persisted and can lead to problems later when they do not get set or are not set to what they should be. It's important to understand the parameters in the configuration file and their dependencies on other parameter settings in the file. Blindly overriding one setting using an environment variable could affect the functionality of another setting.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
