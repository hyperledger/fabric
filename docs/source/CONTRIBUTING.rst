.. note:: Users who are migrating from Gerrit to GitHub: You can follow simple
          Git workflows to move your development from Gerrit to GitHub. After
          forking the Fabric repo, simply push the branches you want to save from
          your current Gerrit-based local repo to your remote forked repository.
          Once you've pushed the changes you want to save, simply delete your
          local Gerrit-based repository and clone your fork.

          For a basic Git workflow recommendation please see our doc at
          :doc:`github/github`.

Contributions Welcome!
======================

We welcome contributions to Hyperledger in many forms, and
there's always plenty to do!

First things first, please review the Hyperledger `Code of
Conduct <https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct>`__
before participating. It is important that we keep things civil.

Ways to contribute
------------------
There are many ways you can contribute to Hyperledger Fabric, both as a user and
as a developer.

As a user:

- `Making Feature/Enhancement Proposals`_
- `Reporting bugs`_
- Help test an upcoming Epic on the
  `release roadmap <https://jira.hyperledger.org/secure/Dashboard.jspa?selectPageId=10104>`_.
  Contact the Epic assignee via the Jira work item or on
  `RocketChat <https://chat.hyperledger.org>`_.

As a developer:

- If you only have a little time, consider picking up a
  `"help-wanted" <https://jira.hyperledger.org/issues/?filter=10147>`_ task,
  see `Fixing issues and working stories`_.
- If you can commit to full-time development, either propose a new feature
  (see `Making Feature/Enhancement Proposals`_) and
  bring a team to implement it, or join one of the teams working on an existing Epic.
  If you see an Epic that interests you on the
  `release roadmap <https://jira.hyperledger.org/secure/Dashboard.jspa?selectPageId=10104>`_,
  contact the Epic assignee via the Jira work item or on `RocketChat <https://chat.hyperledger.org/>`_.

Getting a Linux Foundation account
----------------------------------

In order to participate in the development of the Hyperledger Fabric
project, you will need a Linux Foundation
account. Once you have a LF ID you will be able to
access all the Hyperledger community tools, including
`Jira issue management <https://jira.hyperledger.org>`__,
`RocketChat <https://chat.hyperledger.org/>`__, and the
`Wiki <https://wiki.hyperledger.org/display/fabric/Hyperledger+Fabric>`__ (for editing, only).

Follow the steps below to create a Linux Foundation account if you don't
already have one.

1. Go to the `Linux Foundation ID
   website <https://identity.linuxfoundation.org/>`__.

2. Select the option ``I need to create a Linux Foundation ID``, and fill
   out the form that appears.

3. Wait a few minutes, then look for an email message with the subject line:
   "Validate your Linux Foundation ID email".

4. Open the received URL to validate your email address.

5. Verify that your browser displays the message
   ``You have successfully validated your e-mail address``.

6. Access `Jira issue management <https://jira.hyperledger.org>`__, or
   `RocketChat <https://chat.hyperledger.org/>`__.

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

The Fabric project is lead by the project's top level :doc:`maintainers <MAINTAINERS>`.
The maintainers are responsible for reviewing and merging all patches submitted
for review, and they guide the overall technical direction of the project within
the guidelines established by the Hyperledger Technical Steering Committee (TSC).

Becoming a maintainer
~~~~~~~~~~~~~~~~~~~~~

The project's maintainers will, from time-to-time, consider
adding or removing a maintainer. An existing maintainer can submit a
change set to the :doc:`maintainers <MAINTAINERS>` file. A nominated
Contributor may become a Maintainer by a majority approval of the proposal
by the existing Maintainers. Once approved, the change set is then merged
and the individual is added to (or alternatively, removed from) the maintainers
group. Maintainers may be removed by explicit resignation, for prolonged
inactivity (3 or more months), or for some infraction of the `code of conduct
<https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct>`__
or by consistently demonstrating poor judgement. A maintainer removed for
inactivity should be restored following a sustained resumption of contributions
and reviews (a month or more) demonstrating a renewed commitment to the project.

Release cadence
~~~~~~~~~~~~~~~

The Fabric maintainers have settled on a quarterly (approximately) release
cadence (see `releases <https://github.com/hyperledger/fabric#releases>`__).
At any given time, there will be a stable LTS (long term support) release branch,
as well as the master branch for upcoming new features.
Follow the discussion on the #fabric-release channel in RocketChat.

Making Feature/Enhancement Proposals
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

First, take time to review
`JIRA <https://jira.hyperledger.org/issues/?filter=12524>`__
to be sure that there isn't already an open (or recently closed) proposal for the
same function. If there isn't, to make a proposal we recommend that you open a
JIRA Epic or Story, whichever seems to best fit the circumstance and
link or inline a "one pager" of the proposal that states what the feature would
do and, if possible, how it might be implemented. It would help also to make a
case for why the feature should be added, such as identifying specific use
case(s) for which the feature is needed and a case for what the benefit would be
should the feature be implemented. Once the JIRA issue is created, and the
"one pager" either attached, inlined in the description field, or a link to a
publicly accessible document is added to the description, send an introductory
email to the fabric@lists.hyperledger.org mailing list linking the
JIRA issue, and soliciting feedback.

Discussion of the proposed feature should be conducted in the JIRA issue itself,
so that we have a consistent pattern within our community as to where to find
design discussion.

Getting the support of three or more of the Hyperledger Fabric maintainers for
the new feature will greatly enhance the probability that the feature's related
PRs will be included in a subsequent release.

Maintainers meeting
~~~~~~~~~~~~~~~~~~~

The maintainers hold a bi-weekly meeting every other Wednesday at 9 am ET
on `Zoom <https://zoom.us/my/hyperledger.community>`__. Please see the
`community calendar <https://wiki.hyperledger.org/community/calendar-public-meetings>`__
for details.

The purpose of the maintainers meeting is to plan for and review the progress of
releases, and to discuss the technical and operational direction of the project
and sub-projects.

Please see the
`wiki <https://wiki.hyperledger.org/display/fabric/Maintainer+Meetings>`__
for maintainer meeting details.

New feature/enhancement proposals as described above should be presented to a
maintainers meeting for consideration, feedback and acceptance.

Release roadmap
~~~~~~~~~~~~~~~

The Fabric release roadmap of epics is maintained in
`JIRA <https://jira.hyperledger.org/secure/Dashboard.jspa?selectPageId=10104>`__.

Communications
~~~~~~~~~~~~~~

We use `RocketChat <https://chat.hyperledger.org/>`__ for communication
and Google Hangouts™ for screen sharing between developers. Our
development planning and prioritization is done in
`JIRA <https://jira.hyperledger.org>`__, and we take longer running
discussions/decisions to the `mailing
list <https://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric>`__.

Contribution guide
------------------

Install prerequisites
~~~~~~~~~~~~~~~~~~~~~

Before we begin, if you haven't already done so, you may wish to check that
you have all the :doc:`prerequisites <prereqs>` installed on the platform(s)
on which you'll be developing blockchain applications and/or operating
Hyperledger Fabric.

Getting help
~~~~~~~~~~~~

If you are looking for something to work on, or need some expert
assistance in debugging a problem or working out a fix to an issue, our
`community <https://www.hyperledger.org/community>`__ is always eager to
help. We hang out on
`Chat <https://chat.hyperledger.org/channel/fabric/>`__, IRC
(#hyperledger on freenode.net) and the `mailing
lists <https://lists.hyperledger.org/>`__. Most of us don't bite :grin:
and will be glad to help. The only silly question is the one you don't
ask. Questions are in fact a great way to help improve the project as
they highlight where our documentation could be clearer.

Reporting bugs
~~~~~~~~~~~~~~

If you are a user and you have found a bug, please submit an issue using
`JIRA <https://jira.hyperledger.org/secure/Dashboard.jspa?selectPageId=10104>`__.
Before you create a new JIRA issue, please try to search the existing items to
be sure no one else has previously reported it. If it has been previously
reported, then you might add a comment that you also are interested in seeing
the defect fixed.

.. note:: If the defect is security-related, please follow the Hyperledger
          `security bug reporting process <https://wiki.hyperledger.org/display/HYP/Defect+Response>`__.

If it has not been previously reported, you may either submit a PR with a
well documented commit message describing the defect and the fix, or you 
may create a new JIRA. Please try to provide
sufficient information for someone else to reproduce the
issue. One of the project's maintainers should respond to your issue within 24
hours. If not, please bump the issue with a comment and request that it be
reviewed. You can also post to the relevant Hyperledger Fabric channel in
`Hyperledger Chat <https://chat.hyperledger.org>`__.  For example, a doc bug should
be broadcast to ``#fabric-documentation``, a database bug to ``#fabric-ledger``,
and so on...

Submitting your fix
~~~~~~~~~~~~~~~~~~~

If you just submitted a JIRA for a bug you've discovered, and would like to
provide a fix, we would welcome that gladly! Please assign the JIRA issue to
yourself, then you can submit a pull request (PR).

Fixing issues and working stories
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Review the `issues
list <https://jira.hyperledger.org/issues/?filter=10580>`__ and find
something that interests you. You could also check the
`"help-wanted" <https://jira.hyperledger.org/issues/?filter=10147>`__
list. It is wise to start with something relatively straight forward and
achievable, and that no one is already assigned. If no one is assigned,
then assign the issue to yourself. Please be considerate and rescind the
assignment if you cannot finish in a reasonable time, or add a comment
saying that you are still actively working the issue if you need a
little more time.

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
fabric-samples, fabric-test, fabric-sdk-node, fabric-sdk-java,
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

-  Include a link to the JIRA story for the change. Why? Because a) we
   want to track our velocity to better judge what we think we can
   deliver and when and b) because we can justify the change more
   effectively. In many cases, there should be some discussion around a
   proposed change and we want to link back to that from the change
   itself.

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
   comprehensive description of the change. Each change MUST include the JIRA
   identifier corresponding to the change (e.g. [FAB-1234]). This can be
   in the title but should also be in the body of the commit message.

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
<https://github.com/hyperledger/fabric/blob/master/docs/source/dev-setup/headers.txt>`__.

We have tried to make it as easy as possible to make contributions. This
applies to how we handle the legal aspects of contribution. We use the
same approach—the `Developer's Certificate of Origin 1.1
(DCO) <https://github.com/hyperledger/fabric/blob/master/docs/source/DCO1.1.txt>`__—that the Linux® Kernel
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

   MAINTAINERS
   jira_navigation
   dev-setup/devenv
   dev-setup/build
   testing
   style-guides/go-style

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/