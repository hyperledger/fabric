# Contributions Welcome!

We welcome contributions to the Hyperledger Project in many forms, and
there's always plenty to do!

First things first, please review the Hyperledger Project's [Code of
Conduct](https://github.com/hyperledger/hyperledger/wiki/Hyperledger-Project-Code-of-Conduct)
before participating. It is important that we keep things civil.

## Getting a Linux Foundation account

In order to participate in the development of the Hyperledger Fabric project,
you will need an [LF account](Gerrit/lf-account.md). You will need to use
your LF ID to grant you access to all the Hyperledger community tools, including
[Gerrit](https://gerrit.hyperledger.org) and [Jira](https://jira.hyperledger.org).

### Setting up your SSH key

For Gerrit, you will want to register your public SSH key. Login to
[Gerrit](https://gerrit.hyperledger.org)
with your LF account, and click on your name in the upper right-hand corner
and then click 'Settings'. In the left-hand margin, you should see a link for
'SSH Public Keys'. Copy-n-paste your [public SSH key](https://help.github.com/articles/generating-an-ssh-key/)
into the window and press 'Add'.

## Getting help

If you are looking for something to work on, or need some expert assistance in
debugging a problem or working out a fix to an issue, our
[community](https://www.hyperledger.org/community) is always eager to help. We
hang out on [Slack](https://hyperledgerproject.slack.com/), IRC (#hyperledger on
freenode.net) and the [mailing lists](http://lists.hyperledger.org/). Most of us
don't bite ;-) and will be glad to help.

## Requirements and Use Cases

We have a [Requirements
WG](https://github.com/hyperledger/hyperledger/wiki/Requirements-WG) that is
documenting use cases and from those use cases deriving requirements. If you are
interested in contributing to this effort, please feel free to join the
discussion in
[slack](https://hyperledgerproject.slack.com/messages/requirements/).

## Reporting bugs

If you are a user and you find a bug, please submit an
[issue](https://github.com/hyperledger/fabric/issues). Please try to provide
sufficient information for someone else to reproduce the issue. One of the
project's maintainers should respond to your issue within 24 hours. If not,
please bump the issue and request that it be reviewed.

## Fixing issues and working stories
Review the [issues list](https://github.com/hyperledger/fabric/issues) and find
something that interests you. You could also check the ["help
wanted"](https://github.com/hyperledger/fabric/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)
and ["good first
bug"](https://github.com/hyperledger/fabric/issues?q=is%3Aissue+is%3Aopen+label%3Agood-first-bug)
lists. It is wise to start with something relatively straight forward and
achievable. Usually there will be a comment in the issue that indicates whether
someone has already self-assigned the issue. If no one has already taken it,
then add a comment assigning the issue to yourself, eg.: `I'll work on this
issue.`. Please be considerate and rescind the offer in comments if you cannot
finish in a reasonable time, or add a comment saying that you are still actively
working the issue if you need a little more time.

## Working with a local clone and Gerrit

We are using [Gerrit](https://gerrit.hyperledger.org/r/#/admin/projects/fabric)
to manage code contributions. If you are unfamiliar, please review [this
document](Gerrit/gerrit.md) before proceeding.

After you have familiarized yourself with `Gerrit`, and maybe played around with
the `lf-sandbox` project, you should be ready to set up your local [development
environment](dev-setup/devenv.md). We use a Vagrant-based approach to
development that simplifies things greatly.

## Coding guidelines

Be sure to check out the language-specific [style
guides](Style-guides/go-style.md) before making any changes. This will ensure a
smoother review.

### Becoming a maintainer

This project is managed under open governance model as described in our
[charter](https://www.hyperledger.org/about/charter). Projects or sub-projects
will be lead by a set of maintainers. New projects can designate an initial set
of maintainers that will be approved by the Technical Steering Committee when
the project is first approved. The project's maintainers will, from
time-to-time, consider adding or removing a maintainer. An existing maintainer
will post a patchset to the [MAINTAINERS.md](MAINTAINERS.md) file. If a
majority of the maintainers concur in the comments, the pull request is then
merged and the individual becomes a (or is removed as a) maintainer. Note that
removing a maintainer should not be taken lightly, but occasionally, people do
move on - hence the bar should be some period of inactivity, an explicit
resignation, some infraction of the code of conduct or consistently
demonstrating poor judgement.

## Legal stuff

**Note:** Each source file must include a license header for the Apache Software
License 2.0. A template of that header can be found [here](https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/headers.txt).

We have tried to make it as easy as possible to make contributions. This
applies to how we handle the legal aspects of contribution. We use the same
approach&mdash;the [Developer's Certificate of Origin 1.1 (DCO)](docs/biz/DCO1.1.txt)&mdash;that
the Linux&reg; Kernel [community](http://elinux.org/Developer_Certificate_Of_Origin) uses to manage code contributions.

We simply ask that when submitting a patch for review, the developer must include
a sign-off statement in the commit message.

Here is an example Signed-off-by line, which indicates that the submitter
accepts the DCO:

```
Signed-off-by: John Doe <john.doe@hisdomain.com>
```
You can include this automatically when you commit a change to your local git
repository using `git commit -s`.
