Contributions Welcome!
======================

We welcome contributions to Hyperledger in many forms, and there's always plenty
to do!

First things first, please review the Hyperledger `Code of
Conduct <https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct>`__
before participating. It is important that we keep things civil.

.. note:: If you want to contribute to this documentation, please check out the :doc:`style_guide`.

Ways to contribute
------------------
There are many ways you can contribute to Hyperledger Fabric, both as a user and
as a developer.

As a user:

- `Making Feature/Enhancement Proposals`_
- `Reporting bugs`_

As a writer or information developer:

- Update the documentation using your experience of Fabric and this
  documentation to improve existing topics and create new ones.  A documentation
  change is an easy way to get started as a contributor, makes it easier for
  other users to understand and use Fabric, and grows your open source commit
  history.

- Participate in a language translation to keep the Fabric documentation current
  in your chosen language.  The Fabric documentation is available in a number of
  languages -- English, Chinese, Malayalam and Brazilian Portuguese -- so why
  not join a team that keeps your favorite documentation up-to-date? You'll find
  a friendly community of users, writers and developers to collaborate with.

- Start a new language translation if the Fabric documentation isn't
  available in your language.  The Chinese, Malayalam and Portuguese Brazilian
  teams got started this way, and you can too!  It's more work, as you'll have
  to form a community of writers, and organize contributions; but it's really
  fulfilling to see the Fabric documentation available in your chosen language.

Jump to `Contributing documentation`_ to get started on your journey.

As a developer:

- If you only have a little time, consider picking up a
  `"good first issue" <https://github.com/hyperledger/fabric/labels/good%20first%20issue>`_ task,
  see `Fixing issues and working stories`_.
- If you can commit to full-time development, either propose a new feature
  (see `Making Feature/Enhancement Proposals`_) and
  bring a team to implement it, or join one of the teams working on an existing Epic.
  If you see an Epic that interests you on the
  `GitHub epic backlog <https://github.com/hyperledger/fabric/labels/Epic>`_,
  contact the Epic assignee via the GitHub issue.

Contributing documentation
--------------------------

It's a good idea to make your first change a documentation change. It's quick
and easy to do, ensures that you have a correctly configured machine, (including
the required pre-requisite software), and gets you familiar with the
contribution process.  Use the following topics to help you get started:

.. toctree::
   :maxdepth: 1

   advice_for_writers
   docs_guide
   international_languages
   style_guide

Project Governance
------------------

Hyperledger Fabric is managed under an open governance model as described in
our `charter <https://www.hyperledger.org/about/charter>`__. Projects and
sub-projects are lead by a set of maintainers. New sub-projects can
designate an initial set of maintainers that will be approved by the
top-level project's existing maintainers when the project is first
approved.

Maintainers
~~~~~~~~~~~

The Fabric project is lead by the project's top level `maintainers <https://github.com/hyperledger/fabric/blob/main/MAINTAINERS.md>`__.
The maintainers are responsible for reviewing and merging all patches submitted
for review, and they guide the overall technical direction of the project within
the guidelines established by the Hyperledger Technical Steering Committee (TSC).

Becoming a maintainer
~~~~~~~~~~~~~~~~~~~~~

The project's maintainers will, from time-to-time, consider
adding a maintainer, based on the following criteria:

- Demonstrated track record of PR reviews (both quality and quantity of reviews)
- Demonstrated thought leadership in the project
- Demonstrated shepherding of project work and contributors

An existing maintainer can submit a pull request to the
`maintainers <https://github.com/hyperledger/fabric/blob/main/MAINTAINERS.md>`__ file.
A nominated Contributor may become a Maintainer by a majority approval of the proposal
by the existing Maintainers. Once approved, the change set is then merged
and the individual is added to the maintainers group.

Maintainers may be removed by explicit resignation, for prolonged
inactivity (e.g. 3 or more months with no review comments),
or for some infraction of the `code of conduct
<https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct>`__
or by consistently demonstrating poor judgement. A proposed removal
also requires a majority approval. A maintainer removed for
inactivity should be restored following a sustained resumption of contributions
and reviews (a month or more) demonstrating a renewed commitment to the project.

Releases
~~~~~~~~

Fabric provides periodic releases with new features and improvements.
New feature work is merged to the Fabric main branch on `GitHub <https://github.com/hyperledger/fabric>`__.
Releases branches are created prior to each release so that the code can stabilize while
new features continue to get merged to the main branch.
Important fixes will also be backported to the most recent LTS (long-term support) release branch,
and to the prior LTS release branch during periods of LTS release overlap.

See `releases <https://github.com/hyperledger/fabric#releases>`__ for more details.

Making Feature/Enhancement Proposals
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Minor improvements can be implemented and reviewed via the normal `GitHub pull request workflow <https://guides.github.com/introduction/flow/>`__ but for changes that are more substantial Fabric follows the RFC (request for comments) process.

This process is intended to provide a consistent and controlled path for major changes to Fabric and other official project components, so that all stakeholders can be confident about the direction in which Fabric is evolving.

To propose a new feature, first, check the
`GitHub issues backlog <https://github.com/hyperledger/fabric/issues>`__ and the `Fabric RFC repository <https://github.com/hyperledger/fabric-rfcs/>`__ to be sure that there isn't already an open (or recently closed) proposal for the same functionality. If there isn't, follow `the RFC process <https://github.com/hyperledger/fabric-rfcs/blob/main/README.md>`__ to make a proposal.

Contributor meeting
~~~~~~~~~~~~~~~~~~~

The maintainers hold regular contributors meetings.
The purpose of the contributors meeting is to plan for and review the progress of
releases and contributions, and to discuss the technical and operational direction of the project
and sub-projects.

Please see the
`wiki <https://wiki.hyperledger.org/display/fabric/Contributor+Meetings>`__
for maintainer meeting details.

New feature/enhancement proposals as described above should be presented to a
maintainers meeting for consideration, feedback and acceptance.

Release roadmap
~~~~~~~~~~~~~~~

The Fabric release roadmap is managed as a list of
`GitHub issues with Epic label <https://github.com/hyperledger/fabric/labels/Epic>`__.

Communications and Getting Help
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

We use the `Fabric mailing list <https://lists.hyperledger.org/g/fabric/>`__ for formal communication and
`Discord <https://discord.com/invite/hyperledger/>`__ for community chat.
Feel free to reach out for help on one of the Fabric channels!
If you'd like contribution help or suggestions reach out on the #fabric-code-contributors channel.

Our development planning and prioritization is done using a
`GitHub Issues ZenHub board <https://app.zenhub.com/workspaces/fabric-57c43689b6f3d8060d082cf1/board>`__, and we take longer running
discussions/decisions to the `Fabric contributor meeting <https://wiki.hyperledger.org/display/fabric/Contributor+Meetings>`__.

The mailing list, Discord, and GitHub each require their own login which you can request upon your first interaction.

The Hyperledger Fabric `wiki <https://wiki.hyperledger.org/display/fabric>`__
and the legacy issue management system in `Jira <https://jira.hyperledger.org/projects/FAB/issues>`__
require a `Linux Foundation ID <https://identity.linuxfoundation.org/>`__,
but these resources are primarily for read-only reference and you will likely not need an ID.

Contribution guide
------------------

Install prerequisites
~~~~~~~~~~~~~~~~~~~~~

Before we begin, if you haven't already done so, you may wish to check that
you have all the :doc:`prerequisites <prereqs>` installed on the platform(s)
on which you'll be developing blockchain applications and/or operating
Hyperledger Fabric.

Reporting bugs
~~~~~~~~~~~~~~

If you are a user and you have found a bug, please submit an issue using
`GitHub Issues <https://github.com/hyperledger/fabric/issues>`__.
Before you create a new GitHub issue, please try to search the existing issues to
be sure no one else has previously reported it. If it has been previously
reported, then you might add a comment that you also are interested in seeing
the defect fixed.

.. note:: If the defect is security-related, please follow the Hyperledger
          `security bug reporting process <https://wiki.hyperledger.org/display/SEC/Defect+Response>`__.

If it has not been previously reported, you may either submit a PR with a
well documented commit message describing the defect and the fix, or you
may create a new GitHub issue. Please try to provide
sufficient information for someone else to reproduce the
issue. One of the project's maintainers should respond to your issue within 24
hours. If not, please bump the issue with a comment and request that it be
reviewed. You can also post to the relevant Hyperledger Fabric channel in
`Hyperledger Discord <https://discord.com/servers/hyperledger-foundation-905194001349627914>`__.  For example, a doc bug should
be broadcast to ``#fabric-documentation``, a database bug to ``#fabric-ledger``,
and so on...

Submitting your fix
~~~~~~~~~~~~~~~~~~~

If you just submitted a GitHub issue for a bug you've discovered, and would like to
provide a fix, we would welcome that gladly! Please assign the GitHub issue to
yourself, then submit a pull request (PR). Please refer to :doc:`github/github`
for a detailed workflow.

Fixing issues and working stories
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Fabric issues and bugs are managed in `GitHub issues <https://github.com/hyperledger/fabric/issues>`__.
Review the list of issues and find
something that interests you. You could also check the
`"good first issue" <https://github.com/hyperledger/fabric/labels/good%20first%20issue>`__
list. It is wise to start with something relatively straight forward and
achievable, and that no one is already assigned. If no one is assigned,
then assign the issue to yourself. Please be considerate and rescind the
assignment if you cannot finish in a reasonable time, or add a comment
saying that you are still actively working the issue if you need a
little more time.

While GitHub issues tracks a backlog of known issues that could be worked in the future,
if you intend to immediately work on a change that does not yet have a corresponding issue,
you can submit a pull request to `Github <https://github.com/hyperledger/fabric>`__ without linking to an existing issue.

Reviewing submitted Pull Requests (PRs)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Another way to contribute and learn about Hyperledger Fabric is to help the
maintainers with the review of the PRs that are open. Indeed
maintainers have the difficult role of having to review all the PRs
that are being submitted and evaluate whether they should be merged or
not. You can review the code and/or documentation changes, test the
changes, and tell the submitters and maintainers what you think. Once
your review and/or test is complete just reply to the PR with your
findings, by adding comments and/or voting. A comment saying something
like "I tried it on system X and it works" or possibly "I got an error
on system X: xxx " will help the maintainers in their evaluation. As a
result, maintainers will be able to process PRs faster and everybody
will gain from it.

Just browse through `the open PRs on GitHub
<https://github.com/hyperledger/fabric/pulls>`__ to get started.

PR Aging
~~~~~~~~

As the Fabric project has grown, so too has the backlog of open PRs. One
problem that nearly all projects face is effectively managing that backlog
and Fabric is no exception. In an effort to keep the backlog of Fabric and
related project PRs manageable, we are introducing an aging policy which
will be enforced by bots.  This is consistent with how other large projects
manage their PR backlog.

PR Aging Policy
~~~~~~~~~~~~~~~

The Fabric project maintainers will automatically monitor all PR activity for
delinquency. If a PR has not been updated in 2 weeks, a reminder comment will be
added requesting that the PR either be updated to address any outstanding
comments or abandoned if it is to be withdrawn. If a delinquent PR goes another
2 weeks without an update, it will be automatically abandoned. If a PR has aged
more than 2 months since it was originally submitted, even if it has activity,
it will be flagged for maintainer review.

If a submitted PR has passed all validation but has not been reviewed in 72
hours (3 days), it will be flagged to the #fabric-pr-review channel daily until
it receives a review comment(s).

This policy applies to all official Fabric projects (fabric, fabric-ca,
fabric-samples, fabric-test, fabric-sdk-node, fabric-sdk-java, fabric-sdk-go, fabric-gateway-java,
fabric-chaincode-node, fabric-chaincode-java, fabric-chaincode-evm,
fabric-baseimage, and fabric-amcl).

Setting up development environment
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Next, try :doc:`building the project <dev-setup/build>` in your local
development environment to ensure that everything is set up correctly.

What makes a good pull request?
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

-  One change at a time. Not five, not three, not ten. One and only one.
   Why? Because it limits the blast area of the change. If we have a
   regression, it is much easier to identify the culprit commit than if
   we have some composite change that impacts more of the code.

-  If there is a corresponding GitHub issue, include a link to the
   GitHub issue in the PR summary and commit message.
   Why? Because there will often be additional discussion around
   a proposed change or bug in the GitHub issue.
   Additionally, if you use syntax like "Resolves #<GitHub issue number>"
   in the PR summary and commit message, the GitHub issue will
   automatically be closed when the PR is merged.

-  Include unit and integration tests (or changes to existing tests)
   with every change. This does not mean just happy path testing,
   either. It also means negative testing of any defensive code that it
   correctly catches input errors. When you write code, you are
   responsible to test it and provide the tests that demonstrate that
   your change does what it claims. Why? Because without this we have no
   clue whether our current code base actually works.

-  Unit tests should have NO external dependencies. You should be able
   to run unit tests in place with ``go test`` or equivalent for the
   language. Any test that requires some external dependency (e.g. needs
   to be scripted to run another component) needs appropriate mocking.
   Anything else is not unit testing, it is integration testing by
   definition. Why? Because many open source developers do Test Driven
   Development. They place a watch on the directory that invokes the
   tests automagically as the code is changed. This is far more
   efficient than having to run a whole build between code changes. See
   `this definition <http://artofunittesting.com/definition-of-a-unit-test/>`__
   of unit testing for a good set of criteria to keep in mind for writing
   effective unit tests.

-  Minimize the lines of code per PR. Why? Maintainers have day jobs,
   too. If you send a 1,000 or 2,000 LOC change, how long do you think
   it takes to review all of that code? Keep your changes to < 200-300
   LOC, if possible. If you have a larger change, decompose it into
   multiple independent changes. If you are adding a bunch of new
   functions to fulfill the requirements of a new capability, add them
   separately with their tests, and then write the code that uses them
   to deliver the capability. Of course, there are always exceptions. If
   you add a small change and then add 300 LOC of tests, you will be
   forgiven;-) If you need to make a change that has broad impact or a
   bunch of generated code (protobufs, etc.). Again, there can be
   exceptions.

.. note:: Large pull requests, e.g. those with more than 300 LOC are more than likely
          not going to receive an approval, and you'll be asked to refactor
          the change to conform with this guidance.

-  Write a meaningful commit message. Include a meaningful 55 (or less)
   character title, followed by a blank line, followed by a more
   comprehensive description of the change.

.. note:: Example commit message:

          ::

              [FAB-1234] fix foobar() panic

              Fix [FAB-1234] added a check to ensure that when foobar(foo string)
              is called, that there is a non-empty string argument.

Finally, be responsive. Don't let a pull request fester with review
comments such that it gets to a point that it requires a rebase. It only
further delays getting it merged and adds more work for you - to
remediate the merge conflicts.

Legal stuff
-----------

**Note:** Each source file must include a license header for the Apache
Software License 2.0. See the template of the `license header
<https://github.com/hyperledger/fabric/blob/main/docs/source/dev-setup/headers.txt>`__.

We have tried to make it as easy as possible to make contributions. This
applies to how we handle the legal aspects of contribution. We use the
same approach—the `Developer's Certificate of Origin 1.1
(DCO) <https://github.com/hyperledger/fabric/blob/main/docs/source/DCO1.1.txt>`__—that the Linux® Kernel
`community <https://elinux.org/Developer_Certificate_Of_Origin>`__ uses
to manage code contributions.

We simply ask that when submitting a patch for review, the developer
must include a sign-off statement in the commit message.

Here is an example Signed-off-by line, which indicates that the
submitter accepts the DCO:

::

    Signed-off-by: John Doe <john.doe@example.com>

You can include this automatically when you commit a change to your
local git repository using ``git commit -s``.

Related Topics
--------------

.. toctree::
   :maxdepth: 1

   dev-setup/devenv
   dev-setup/build
   style-guides/go-style

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
