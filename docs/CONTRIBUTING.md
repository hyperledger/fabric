# Contributions Welcome!

We welcome contributions to the Hyperledger Project in many forms, and
there's always plenty to do!

First things first, please review the Hyperledger Project's [Code of Conduct](https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct)
before participating. It is important that we keep things civil.

## Getting a Linux Foundation account

In order to participate in the development of the Hyperledger Fabric project,
you will need a [Linux Foundation account](Gerrit/lf-account.md). You will need
to use your LF ID to access to all the Hyperledger community development tools,
including [Gerrit](https://gerrit.hyperledger.org),
[Jira](https://jira.hyperledger.org) and the
[Wiki](https://wiki.hyperledger.org/start) (for editing, only).

### Setting up your SSH key

For Gerrit, before you can submit any change set for review, you will need to
register your public SSH key. Login to [Gerrit](https://gerrit.hyperledger.org)
with your [LFID](Gerrit/lf-account.md), and click on your name in the upper
right-hand corner of your browser window and then click 'Settings'. In the
left-hand margin, you should see a link for 'SSH Public Keys'. Copy-n-paste
your [public SSH key](https://help.github.com/articles/generating-an-ssh-key/)
into the window and press 'Add'.

## Getting help

If you are looking for something to work on, or need some expert assistance in
debugging a problem or working out a fix to an issue, our
[community](https://www.hyperledger.org/community) is always eager to help. We
hang out on [Slack](https://hyperledgerproject.slack.com/), IRC (#hyperledger on
freenode.net) and the [mailing lists](http://lists.hyperledger.org/). Most of us
don't bite :grin: and will be glad to help. The only silly question is the one
you don't ask. Questions are in fact a great way to help improve the project as
they highlight where our documentation could be clearer.

## Requirements and Use Cases

We have a [Requirements
WG](https://wiki.hyperledger.org/groups/requirements/requirements-wg) that is
documenting use cases and from those use cases deriving requirements. If you are
interested in contributing to this effort, please feel free to join the
discussion in
[slack](https://hyperledgerproject.slack.com/messages/requirements/).

## Reporting bugs

If you are a user and you find a bug, please submit an issue using
[JIRA](https://jira.hyperledger.org). Please try to provide
sufficient information for someone else to reproduce the issue. One of the
project's maintainers should respond to your issue within 24 hours. If not,
please bump the issue with a comment and request that it be reviewed. You can
also post to the `#fabric-maintainers` channel in
[slack](https://hyperledgerproject.slack.com/messages/requirements/).

## Fixing issues and working stories

Review the [issues list](https://github.com/hyperledger/fabric/issues) and find
something that interests you. You could also check the
["help-wanted"](https://jira.hyperledger.org/issues/?jql=project%20%3D%20Fabric%20AND%20labels%20%3D%20help-wanted)
list. It is wise to start with something relatively straight forward and
achievable, and that no one is already assigned. If no one is assigned,
then assign the issue to yourself. Please be considerate and rescind the
assignment if you cannot finish in a reasonable time, or add a comment
saying that you are still actively working the issue if you need a little
more time.

## Working with a local clone and Gerrit

We are using [Gerrit](https://gerrit.hyperledger.org/r/#/admin/projects/fabric)
to manage code contributions. If you are unfamiliar, please review this
[document](Gerrit/gerrit.md) before proceeding.

After you have familiarized yourself with `Gerrit`, and maybe played around with
the `lf-sandbox`
[project](https://gerrit.hyperledger.org/r/#/admin/projects/lf-sandbox,branches),
you should be ready to set up your local development
[environment](dev-setup/devenv.md).

## What makes a good change request?

* One change at a time. Not five, not three, not ten. One and only one. Why?
Because it limits the blast area of the change. If we have a regression, it is
much easier to identify the culprit commit than if we have some composite
change that impacts more of the code.

* Include a link to the JIRA story for the change. Why? Because a) we want to
track our velocity to better judge what we think we can deliver and when and b)
because we can justify the change more effectively. In many cases, there
should be some discussion around a proposed change and we want to link back to
that from the change itself.

* Include unit and integration tests (or changes to existing tests) with every
change. This does not mean just happy path testing, either. It also means
negative testing of any defensive code that it correctly catches input errors.
When you write code, you are responsible to test it and provide the tests that
demonstrate that your change does what it claims. Why? Because
without this we have no clue whether our current code base actually works.

* Unit tests should have NO external dependencies. You should be able to run
unit tests in place with `go test` or equivalent for the language. Any test
that requires some external dependency (e.g. needs to be scripted to run another
component) needs appropriate mocking. Anything else is not unit testing, it is
integration testing by definition. Why? Because many open source developers
do Test Driven Development. They place a watch on the directory that invokes
the tests automagically as the code is changed. This is far more efficient
than having to run a whole build between code changes.

* Minimize the lines of code per CR. Why? Maintainers have day jobs, too. If
you send a 1,000 or 2,000 LOC change, how long do you think it takes to review
all of that code? Keep your changes to < 200-300 LOC if possible. If you have a
larger change, decompose it into multiple independent changess. If you are adding
a bunch of new functions to fulfill the requirements of a new capability, add
them separately with their tests, and then write the code that uses them to
deliver the capability. Of course, there are always exceptions. If you add a
small change and then add 300 LOC of tests, you will be forgiven;-)
If you need to make a change that has broad impact or a bunch of generated
code (protobufs, etc.). Again, there can be exceptions.

* Write a meaningful commit message. Include a meaningful 50 (or less) character
title, followed by a blank line, followed my a more comprehensive description
of the change. Be sure to include the JIRA identifier corresponding to the
change (e.g. [FAB-1234]). This can be in the title but should also be in the
body of the commit message.

e.g.
```
[FAB-1234] fix foobar() panic

Fix [FAB-1234] added a check to ensure that when foobar(foo string) is called,
that there is a non-empty string argument.
```

Finally, be responsive. Don't let a change request fester with review comments
such that it gets to a point that it requires a rebase. It only further delays
getting it merged and adds more work for you - to remediate the merge conflicts.

## Coding guidelines

Be sure to check out the language-specific [style guides](Style-guides/go-style.md)
before making any changes. This will ensure a smoother review.

### Becoming a maintainer

This project is managed under an open governance model as described in our
[charter](https://www.hyperledger.org/about/charter). Projects or sub-projects
will be lead by a set of maintainers. New sub-projects can designate an initial
set of maintainers that will be approved by the top-level project's existing
maintainers when the project is first approved. The project's maintainers will,
from time-to-time, consider adding or removing a maintainer. An existing
maintainer can submit a change set to the [MAINTAINERS.md](MAINTAINERS.md) file.
If there are less than eight maintainers, a majority of the existing maintainers
on that project are required to merge the change set. If there are more than
eight existing maintainers, then if five or more of the maintainers concur with
the proposal, the change set is then merged and the individual is added to
(or alternatively, removed from) the maintainers group. explicit resignation,
some infraction of the [code of conduct](https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct)
or consistently demonstrating poor judgement.

## Legal stuff

**Note:** Each source file must include a license header for the Apache Software
License 2.0. A template of that header can be found [here](https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/headers.txt).

We have tried to make it as easy as possible to make contributions. This
applies to how we handle the legal aspects of contribution. We use the same
approach&mdash;the [Developer's Certificate of Origin 1.1 (DCO)](docs/biz/DCO1.1.txt)&mdash;that
the Linux&reg; Kernel [community](http://elinux.org/Developer_Certificate_Of_Origin)
uses to manage code contributions.

We simply ask that when submitting a patch for review, the developer must include
a sign-off statement in the commit message.

Here is an example Signed-off-by line, which indicates that the submitter
accepts the DCO:

```
Signed-off-by: John Doe <john.doe@hisdomain.com>
```
You can include this automatically when you commit a change to your local git
repository using `git commit -s`.
