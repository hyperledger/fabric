Working with Gerrit
===================

Follow these instructions to collaborate on Hyperledger Fabric
through the Gerrit review system.

Please be sure that you are subscribed to the `mailing
list <http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric>`__
and of course, you can reach out on
`chat <https://chat.hyperledger.org/>`__ if you need help.

Gerrit assigns the following roles to users:

-  **Submitters**: May submit changes for consideration, review other
   code changes, and make recommendations for acceptance or rejection by
   voting +1 or -1, respectively.
-  **Maintainers**: May approve or reject changes based upon feedback
   from reviewers voting +2 or -2, respectively.
-  **Builders**: (e.g. Jenkins) May use the build automation
   infrastructure to verify the change.

Maintainers should be familiar with the :doc:`review
process <reviewing>`. However, anyone is welcome to (and
encouraged!) review changes, and hence may find that document of value.

Git-review
----------

There's a **very** useful tool for working with Gerrit called
`git-review <https://www.mediawiki.org/wiki/Gerrit/git-review>`__. This
command-line tool can automate most of the ensuing sections for you. Of
course, reading the information below is also highly recommended so that
you understand what's going on behind the scenes.

Sandbox project
---------------

We have created a `sandbox
project <https://gerrit.hyperledger.org/r/#/admin/projects/lf-sandbox>`__
to allow developers to familiarize themselves with Gerrit and our
workflows. Please do feel free to use this project to experiment with
the commands and tools, below.

Getting deeper into Gerrit
--------------------------

A comprehensive walk-through of Gerrit is beyond the scope of this
document. There are plenty of resources available on the Internet. A
good summary can be found
`here <https://www.mediawiki.org/wiki/Gerrit/Tutorial>`__. We have also
provided a set of :doc:`Best Practices <best-practices>` that you may
find helpful.

Working with a local clone of the repository
--------------------------------------------

To work on something, whether a new feature or a bugfix:

1. Open the Gerrit `Projects
   page <https://gerrit.hyperledger.org/r/#/admin/projects/>`__

2. Select the project you wish to work on.

3. Open a terminal window and clone the project locally using the
   ``Clone with git hook`` URL. Be sure that ``ssh`` is also selected,
   as this will make authentication much simpler:

   ::

       git clone ssh://LFID@gerrit.hyperledger.org:29418/fabric && scp -p -P 29418 LFID@gerrit.hyperledger.org:hooks/commit-msg fabric/.git/hooks/

**Note:** if you are cloning the fabric project repository, you will
want to clone it to the ``$GOPATH/src/github.com/hyperledger`` directory
so that it will build, and so that you can use it with the Vagrant
:doc:`development environment <../dev-setup/devenv>`.

4. Create a descriptively-named branch off of your cloned repository

::

    cd fabric
    git checkout -b issue-nnnn

5. Commit your code. For an in-depth discussion of creating an effective
   commit, please read :doc:`this document on submitting changes <changes>`.

::

    git commit -s -a

Then input precise and readable commit msg and submit.

6. Any code changes that affect documentation should be accompanied by
   corresponding changes (or additions) to the documentation and tests.
   This will ensure that if the merged PR is reversed, all traces of the
   change will be reversed as well.

Submitting a Change
-------------------

Currently, Gerrit is the only method to submit a change for review.

**Note:** Please review the :doc:`guidelines <changes>` for making and
submitting a change.

Use git review
~~~~~~~~~~~~~~

**Note:** if you prefer, you can use the `git-review <#git-review>`__
tool instead of the following. e.g.

Add the following section to ``.git/config``, and replace ``<USERNAME>``
with your gerrit id.

::

    [remote "gerrit"]
        url = ssh://<USERNAME>@gerrit.hyperledger.org:29418/fabric.git
        fetch = +refs/heads/*:refs/remotes/gerrit/*

Then submit your change with ``git review``.

::

    $ cd <your code dir>
    $ git review

When you update your patch, you can commit with ``git commit --amend``,
and then repeat the ``git review`` command.

Not Use git review
~~~~~~~~~~~~~~~~~~

See the :doc:`directions for building the source code <../dev-setup/build>`.

When a change is ready for submission, Gerrit requires that the change
be pushed to a special branch. The name of this special branch contains
a reference to the final branch where the code should reside, once
accepted.

For the Hyperledger Fabric repository, the special branch is called
``refs/for/master``.

To push the current local development branch to the gerrit server, open
a terminal window at the root of your cloned repository:

::

    cd <your clone dir>
    git push origin HEAD:refs/for/master

If the command executes correctly, the output should look similar to
this:

::

    Counting objects: 3, done.
    Writing objects: 100% (3/3), 306 bytes | 0 bytes/s, done.
    Total 3 (delta 0), reused 0 (delta 0)
    remote: Processing changes: new: 1, refs: 1, done
    remote:
    remote: New Changes:
    remote:   https://gerrit.hyperledger.org/r/6 Test commit
    remote:
    To ssh://LFID@gerrit.hyperledger.org:29418/fabric
    * [new branch]      HEAD -> refs/for/master

The gerrit server generates a link where the change can be tracked.

Adding reviewers
----------------

Optionally, you can add reviewers to your change.

To specify a list of reviewers via the command line, add
``%r=reviewer@project.org`` to your push command. For example:

::

    git push origin HEAD:refs/for/master%r=rev1@email.com,r=rev2@notemail.com

Alternatively, you can auto-configure GIT to add a set of reviewers if
your commits will have the same reviewers all at the time.

To add a list of default reviewers, open the :file:``.git/config`` file
in the project directory and add the following line in the
``[ branch “master” ]`` section:

::

    [branch "master"] #.... push =
    HEAD:refs/for/master%r=rev1@email.com,r=rev2@notemail.com`

Make sure to use actual email addresses instead of the
``@email.com and @notemail.com`` addressses. Don't forget to replace
``origin`` with your git remote name.

Reviewing Using Gerrit
----------------------

-  **Add**: This button allows the change submitter to manually add
   names of people who should review a change; start typing a name and
   the system will auto-complete based on the list of people registered
   and with access to the system. They will be notified by email that
   you are requesting their input.

-  **Abandon**: This button is available to the submitter only; it
   allows a committer to abandon a change and remove it from the merge
   queue.

-  **Change-ID**: This ID is generated by Gerrit (or system). It becomes
   useful when the review process determines that your commit(s) have to
   be amended. You may submit a new version; and if the same Change-ID
   header (and value) are present, Gerrit will remember it and present
   it as another version of the same change.

-  **Status**: Currently, the example change is in review status, as
   indicated by “Needs Verified” in the upper-left corner. The list of
   Reviewers will all emit their opinion, voting +1 if they agree to the
   merge, -1 if they disagree. Gerrit users with a Maintainer role can
   agree to the merge or refuse it by voting +2 or -2 respectively.

Notifications are sent to the email address in your commit message's
Signed-off-by line. Visit your `Gerrit
dashboard <https://gerrit.hyperledger.org/r/#/dashboard/self>`__, to
check the progress of your requests.

The history tab in Gerrit will show you the in-line comments and the
author of the review.

Viewing Pending Changes
-----------------------

Find all pending changes by clicking on the ``All --> Changes`` link in
the upper-left corner, or `open this
link <https://gerrit.hyperledger.org/r/#/q/project:fabric>`__.

If you collaborate in multiple projects, you may wish to limit searching
to the specific branch through the search bar in the upper-right side.

Add the filter *project:fabric* to limit the visible changes to only
those from Hyperledger Fabric.

List all current changes you submitted, or list just those changes in
need of your input by clicking on ``My --> Changes`` or `open this
link <https://gerrit.hyperledger.org/r/#/dashboard/self>`__

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

