Submitting a Change to Gerrit
=============================

Carefully review the following before submitting a change. These
guidelines apply to developers that are new to open source, as well as
to experienced open source developers.

Change Requirements
-------------------

This section contains guidelines for submitting code changes for review.
For more information on how to submit a change using Gerrit, please see
:doc:`Gerrit <gerrit>`.

Changes are submitted as Git commits. Each commit must contain:

-  a short and descriptive subject line that is 72 characters or fewer,
   followed by a blank line.
-  a change description with your logic or reasoning for the changes,
   followed by a blank line
-  a Signed-off-by line, followed by a colon (Signed-off-by:)
-  a Change-Id identifier line, followed by a colon (Change-Id:). Gerrit
   won't accept patches without this identifier.

A commit with the above details is considered well-formed.

All changes and topics sent to Gerrit must be well-formed.
Informationally, ``commit messages`` must include:

-  **what** the change does,
-  **why** you chose that approach, and
-  **how** you know it works -- for example, which tests you ran.

Commits must :doc:`build cleanly <../dev-setup/build>` when applied on
top of each other, thus avoiding breaking bisectability. Each commit
must address a single identifiable issue and must be logically
self-contained.

For example: One commit fixes whitespace issues, another renames a
function and a third one changes the code's functionality. An example
commit file is illustrated below in detail:

::

    [FAB-XXXX] A short description of your change with no period at the end

    You can add more details here in several paragraphs, but please keep each line
    width less than 80 characters. A bug fix should include the issue number.

    Change-Id: IF7b6ac513b2eca5f2bab9728ebd8b7e504d3cebe1
    Signed-off-by: Your Name <commit-sender@email.address>

Include the issue ID in the one line description of your commit message for
readability. Gerrit will link issue IDs automatically to the corresponding
entry in Jira.

Each commit must also contain the following line at the bottom of the commit
message:

::

    Signed-off-by: Your Name <your@email.address>

The name in the Signed-off-by line and your email must match the change
authorship information. Make sure your :file:``.git/config`` is set up
correctly. Always submit the full set of changes via Gerrit.

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
you spot a problem with your CR amend your commit and push it to
update it. The CI process will kick off again.

If you see nothing wrong with your CR it might be that the CI process
simply failed for some reason unrelated to your change. In that case
you may want to restart the CI process by posting a reply to your CR
with the simple content "reverify". Check the `CI management page
<https://github.com/hyperledger/ci-management/blob/master/docs/fabric_ci_process.md>`__
for additional information and options on this.


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
