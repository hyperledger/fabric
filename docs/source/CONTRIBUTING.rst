Contributions Welcome!
======================

We welcome contributions to Hyperledger in many forms, and
there's always plenty to do!

First things first, please review the Hyperledger `Code of
Conduct <https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct>`__
before participating. It is important that we keep things civil.

Install prerequisites
---------------------

Before we begin, if you haven't already done so, you may wish to check that
you have all the :doc:`prerequisites <prereqs>` installed on the platform(s)
on which you'll be developing blockchain applications and/or operating
Hyperledger Fabric.

Getting a Linux Foundation account
----------------------------------

In order to participate in the development of the Hyperledger Fabric
project, you will need a :doc:`Linux Foundation
account <Gerrit/lf-account>`. You will need to use your LF ID to
access to all the Hyperledger community development tools, including
`Gerrit <https://gerrit.hyperledger.org>`__,
`Jira <https://jira.hyperledger.org>`__ and the
`Wiki <https://wiki.hyperledger.org/start>`__ (for editing, only).

Setting up your SSH key
~~~~~~~~~~~~~~~~~~~~~~~

For Gerrit, before you can submit any change set for review, you will
need to register your public SSH key. Login to
`Gerrit <https://gerrit.hyperledger.org>`__ with your
:doc:`LFID <Gerrit/lf-account>`, and click on your name in the upper
right-hand corner of your browser window and then click 'Settings'. In
the left-hand margin, you should see a link for 'SSH Public Keys'.
Copy-n-paste your `public SSH
key <https://help.github.com/articles/generating-an-ssh-key/>`__ into
the window and press 'Add'.

Getting help
------------

If you are looking for something to work on, or need some expert
assistance in debugging a problem or working out a fix to an issue, our
`community <https://www.hyperledger.org/community>`__ is always eager to
help. We hang out on
`Chat <https://chat.hyperledger.org/channel/fabric/>`__, IRC
(#hyperledger on freenode.net) and the `mailing
lists <http://lists.hyperledger.org/>`__. Most of us don't bite :grin:
and will be glad to help. The only silly question is the one you don't
ask. Questions are in fact a great way to help improve the project as
they highlight where our documentation could be clearer.

Requirements and Use Cases
--------------------------

We have a `Requirements
WG <https://wiki.hyperledger.org/groups/requirements/requirements-wg>`__
that is documenting use cases and from those use cases deriving
requirements. If you are interested in contributing to this effort,
please feel free to join the discussion in
`chat <https://chat.hyperledger.org/channel/requirements/>`__.

Reporting bugs
--------------

If you are a user and you find a bug, please submit an issue using
`JIRA <https://jira.hyperledger.org/secure/Dashboard.jspa?selectPageId=10104>`__.
Please try to provide sufficient information for someone else to reproduce the
issue. One of the project's maintainers should respond to your issue within 24
hours. If not, please bump the issue with a comment and request that it be
reviewed. You can also post to the relevant fabric channel in
`Hyperledger Rocket Chat <https://chat.hyperledger.org>`__.  For example, a doc bug should
be broadcast to ``#fabric-documentation``, a database bug to ``#fabric-ledger``,
and so on... 

Fixing issues and working stories
---------------------------------

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

Making Feature/Enhancement Proposals
------------------------------------

Review
`JIRA <https://jira.hyperledger.org/secure/Dashboard.jspa?selectPageId=10104>`__.
to be sure that there isn't already an open (or recently closed) proposal for the
same function. If there isn't, to make a proposal we recommend that you open a
JIRA Epic, Story or Improvement, whichever seems to best fit the circumstance and
link or inline a "one pager" of the proposal that states what the feature would
do and, if possible, how it might be implemented. It would help also to make a
case for why the feature should be added, such as identifying specific use
case(s) for which the feature is needed and a case for what the benefit would be
should the feature be implemented. Once the JIRA issue is created, and the
"one pager" either attached, inlined in the description field, or a link to a
publicly accessible document is added to the description, send an introductory
email to the hyperledger-fabric@lists.hyperledger.org mailing list linking the
JIRA issue, and soliciting feedback.

Discussion of the proposed feature should be conducted in the JIRA issue itself,
so that we have a consistent pattern within our community as to where to find
design discussion.

Getting the support of three or more of the Hyperledger Fabric maintainers for the new
feature will greatly enhance the probability that the feature's related CRs
will be merged.

Working with a local clone and Gerrit
-------------------------------------

We are using
`Gerrit <https://gerrit.hyperledger.org/r/#/admin/projects/fabric>`__ to
manage code contributions. If you are unfamiliar, please review this
:doc:`document <Gerrit/gerrit>` before proceeding.

After you have familiarized yourself with ``Gerrit``, and maybe played
around with the ``lf-sandbox``
`project <https://gerrit.hyperledger.org/r/#/admin/projects/lf-sandbox,branches>`__,
you should be ready to set up your local development
:doc:`environment <dev-setup/devenv>`.

Next, try :doc:`building the project <dev-setup/build>` in your local
development environment to ensure that everything is set up correctly.

The :doc:`logging-control` document describes how to tweak
the logging levels of various components within Hyperledger Fabric. Finally,
every source file needs to include a
`license header <https://github.com/hyperledger/fabric/blob/master/docs/source/dev-setup/headers.txt>`__: modified to include a copyright
statement for the principle author(s).

What makes a good change request?
---------------------------------

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

-  Minimize the lines of code per CR. Why? Maintainers have day jobs,
   too. If you send a 1,000 or 2,000 LOC change, how long do you think
   it takes to review all of that code? Keep your changes to < 200-300
   LOC, if possible. If you have a larger change, decompose it into
   multiple independent changess. If you are adding a bunch of new
   functions to fulfill the requirements of a new capability, add them
   separately with their tests, and then write the code that uses them
   to deliver the capability. Of course, there are always exceptions. If
   you add a small change and then add 300 LOC of tests, you will be
   forgiven;-) If you need to make a change that has broad impact or a
   bunch of generated code (protobufs, etc.). Again, there can be
   exceptions.

   Note: large change requests, e.g. those with more than 300 LOC are more likely
   than not going to receive a -2, and you'll be asked to refactor the change
   to conform with this guidance.

-  Do not stack change requests (e.g. submit a CR from the same local branch
   as your previous CR) unless they are related. This will minimize merge
   conflicts and allow changes to be merged more quickly. If you stack requests
   your subsequent requests may be held up because of review comments in the
   preceding requests.

-  Write a meaningful commit message. Include a meaningful 50 (or less)
   character title, followed by a blank line, followed by a more
   comprehensive description of the change. Each change MUST include the JIRA
   identifier corresponding to the change (e.g. [FAB-1234]). This can be
   in the title but should also be in the body of the commit message.

   Note that Gerrit will automatically create a hyperlink to the JIRA item.

e.g.

::

    [FAB-1234] fix foobar() panic

    Fix [FAB-1234] added a check to ensure that when foobar(foo string) is called,
    that there is a non-empty string argument.

Finally, be responsive. Don't let a change request fester with review
comments such that it gets to a point that it requires a rebase. It only
further delays getting it merged and adds more work for you - to
remediate the merge conflicts.

Communication
--------------

We use `RocketChat <https://chat.hyperledger.org/>`__ for communication
and Google Hangouts™ for screen sharing between developers. Our
development planning and prioritization is done in
`JIRA <https://jira.hyperledger.org>`__, and we take longer running
discussions/decisions to the `mailing
list <http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric>`__.

Maintainers
-----------

The project's :doc:`maintainers <MAINTAINERS>` are responsible for
reviewing and merging all patches submitted for review and they guide
the over-all technical direction of the project within the guidelines
established by the Hyperledger Technical Steering Committee (TSC).

Becoming a maintainer
~~~~~~~~~~~~~~~~~~~~~

This project is managed under an open governance model as described in
our `charter <https://www.hyperledger.org/about/charter>`__. Projects or
sub-projects will be lead by a set of maintainers. New sub-projects can
designate an initial set of maintainers that will be approved by the
top-level project's existing maintainers when the project is first
approved. The project's maintainers will, from time-to-time, consider
adding or removing a maintainer. An existing maintainer can submit a
change set to the :doc:`MAINTAINERS.rst <MAINTAINERS>` file. A nominated
Contributor may become a Maintainer by a majority approval of the proposal
by the existing Maintainers. Once approved, the change set is then merged
and the individual is added to (or alternatively, removed from) the maintainers
group. Maintainers may be removed by explicit resignation, or for some
infraction of the `code of conduct <https://wiki.hyperledger.org/community/hyperledger-project-code-of-conduct>`__
or by consistently demonstrating poor judgement.

Legal stuff
-----------

**Note:** Each source file must include a license header for the Apache
Software License 2.0. See the template of the `license header
<https://github.com/hyperledger/fabric/blob/master/docs/source/dev-setup/headers.txt>`__.

We have tried to make it as easy as possible to make contributions. This
applies to how we handle the legal aspects of contribution. We use the
same approach—the `Developer's Certificate of Origin 1.1
(DCO) <https://github.com/hyperledger/fabric/blob/master/docs/source/DCO1.1.txt>`__—that the Linux® Kernel
`community <http://elinux.org/Developer_Certificate_Of_Origin>`__ uses
to manage code contributions.

We simply ask that when submitting a patch for review, the developer
must include a sign-off statement in the commit message.

Here is an example Signed-off-by line, which indicates that the
submitter accepts the DCO:

::

    Signed-off-by: John Doe <john.doe@hisdomain.com>

You can include this automatically when you commit a change to your
local git repository using ``git commit -s``.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
