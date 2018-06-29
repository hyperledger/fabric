Submitting a Change to Gerrit
=============================

Carefully review the following before submitting a change to the
Hyperledger Fabric code base. These guidelines apply to developers that
are new to open source, as well as to experienced open source developers.

Change Requirements
-------------------

This section contains guidelines for submitting code changes for review.
For more information on how to submit a change using Gerrit, please see
:doc:`Working with Gerrit <gerrit>`.

All changes to Hyperledger Fabric are submitted as Git commits via Gerrit.
Each commit must contain:

-  a short and descriptive subject line that is 55 characters or fewer,
   followed by a blank line,
-  a change description with the logic or reasoning for your changes,
   followed by a blank line,
-  a Signed-off-by line, followed by a colon (Signed-off-by:), and
-  a Change-Id identifier line, followed by a colon (Change-Id:). Gerrit
   won't accept patches without this identifier.

A commit with the above details is considered well-formed.

.. note:: You don't need to supply the Change-Id identifier for a new
          commit; this is added automatically by the ``commit-msg``
          Git hook associated with the repository.
          If you subsequently amend your commit and resubmit it,
          using the same Change-Id value as the initial commit will
          guarantee that Gerrit will recognize that subsequent commit
          as an amended commit with respect to the earlier one.

All changes and topics sent to Gerrit must be well-formed.
In addition to the above mandatory content in a commit, a commit message
should include:

-  **what** the change does,
-  **why** you chose that approach, and
-  **how** you know it works --- for example, which tests you ran.

Commits must :doc:`build cleanly <../dev-setup/build>` when applied on
top of each other, thus avoiding breaking bisectability. Each commit
should address a single identifiable JIRA issue and should be logically
self-contained. For example, one commit might fix whitespace issues,
another commit might rename a function, while a third commit could
change some code's functionality.

A well-formed commit is illustrated below in detail:

::

    [FAB-XXXX] purpose of commit, no more than 55 characters

    You can add more details here in several paragraphs, but please keep
    each line less than 80 characters long.

    Change-Id: IF7b6ac513b2eca5f2bab9728ebd8b7e504d3cebe1
    Signed-off-by: Your Name <commit-sender@email.address>

The name in the ``Signed-off-by:`` line and your email must match the change
authorship information. Make sure your personal ``.gitconfig`` file is set up
correctly.

When a change is included in the set to enable other changes, but it
will not be part of the final set, please let the reviewers know this.

Check that your change request is validated by the CI process
-------------------------------------------------------------

To ensure stability of the code and limit possible regressions, we use
a Continuous Integration (CI) process based on Jenkins which triggers
a build on several platforms and runs tests against every change
request being submitted. It is your responsibility to check that your
CR passes these tests. No CR will ever be merged if it fails the
tests and you shouldn't expect anybody to pay attention to your CRs
until they pass the CI tests.

To check on the status of the CI process, simply look at your CR on
Gerrit, following the URL that was given to you as the result of the
push in the previous step. The History section at the bottom of the
page will display a set of actions taken by "Hyperledger Jobbuilder"
corresponding to the CI process being executed.

Upon completion, "Hyperledger Jobbuilder" will add to the CR a *+1
vote* if successful and a *-1 vote* otherwise.

In case of failure, explore the logs linked from the CR History. If
you spot a problem with your CR, amend your commit and push it to
update it, which will automatically kick off the CI process again.

If you see nothing wrong with your CR, it might be that the CI process
simply failed for some reason unrelated to your change. In that case
you may want to restart the CI process by posting a reply to your CR
with the simple content "reverify". Check the `CI management page
<https://github.com/hyperledger/ci-management/blob/master/docs/source/fabric_ci_process.rst>`__
for additional information and options on this.


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
