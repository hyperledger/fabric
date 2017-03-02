# Gerrit Recommended Practices

This document presents some best practices to help you use Gerrit more
effectively.  The intent is to show how content can be submitted easily. Use the
recommended practices to reduce your troubleshooting time and improve
participation in the community.

## Browsing the Git Tree

Visit [Gerrit](https://gerrit.hyperledger.org/r/#/admin/projects/fabric) then
select `Projects --> List --> SELECT-PROJECT --> Branches`.  Select the branch
that interests you, click on `gitweb` located on the right-hand side.  Now,
`gitweb` loads your selection on the Git web interface and redirects
appropriately.

## Watching a Project

Visit [Gerrit](https://gerrit.hyperledger.org/r/#/admin/projects/fabric), then
select `Settings`, located on the top right corner. Select `Watched Projects`
and then add any projects that interest you.

## Commit Messages

Gerrit follows the Git commit message format. Ensure the headers are at the
bottom and don't contain blank lines between one another. The following example
shows the format and content expected in a commit message:

   Brief (no more than 50 chars) one line description.

   Elaborate summary of the changes made referencing why (motivation), what was
changed and how it was tested. Note also any changes to documentation made to
remain consistent with the code changes, wrapping text at 72 chars/line.

   Jira: FAB-100  
   Change-Id: LONGHEXHASH  
   Signed-off-by: Your Name your.email@example.org  
   AnotherExampleHeader: An Example of another Value

The Gerrit server provides a precommit hook to autogenerate the Change-Id which
is one time use.

**Recommended reading:** [How to Write a Git Commit Message](http://chris.beams.io/posts/git-commit/)

## Avoid Pushing Untested Work to a Gerrit Server

To avoid pushing untested work to Gerrit.

Check your work at least three times before pushing your change to Gerrit.
Be mindful of what information you are publishing.

## Keeping Track of Changes

- Set Gerrit to send you emails:

  - Gerrit will add you to the email distribution list for a change if a
    developer adds you as a reviewer, or if you comment on a specific Patch
    Set.

- Opening a change in Gerrit's review interface is a quick way to follow that
  change.

- Watch projects in the Gerrit projects section at `Gerrit`, select at least
   *New Changes, New Patch Sets, All Comments* and *Submitted Changes*.

Always track the projects you are working on; also see the feedback/comments
mailing list to learn and help others ramp up.

## Topic branches

Topic branches are temporary branches that you push to commit a set of
logically-grouped dependent commits:

To push changes from `REMOTE/master` tree to Gerrit for being reviewed as
a topic in  **TopicName** use the following command as an example:

   $ git push REMOTE HEAD:refs/for/master/TopicName

The topic will show up in the review :abbr:`UI` and in the
`Open Changes List`.  Topic branches will disappear from the master
tree when its content is merged.

## Creating a Cover Letter for a Topic

You may decide whether or not you'd like the cover letter to appear in the
history.

1. To make a cover letter that appears in the history, use this command:

```
git commit --allow-empty
```

Edit the commit message, this message then becomes the cover letter.
The command used doesn't change any files in the source tree.

2. To make a cover letter that doesn't appear in the history follow these steps:

   - Put the empty commit at the end of your commits list so it can be ignored  
   without having to rebase.

   - Now add your commits

```
git commit ...
git commit ...
git commit ...
```

   - Finally, push the commits to a topic branch.  The following command is an
     example:

```
git push REMOTE HEAD:refs/for/master/TopicName
```

If you already have commits but you want to set a cover letter, create an empty
commit for the cover letter and move the commit so it becomes the last commit
on the list. Use the following command as an example:

```
git rebase -i HEAD~#Commits
```

Be careful to uncomment the commit before moving it.
`#Commits` is the sum of the commits plus your new cover letter.


## Finding Available Topics

```
   $ ssh -p 29418 gerrit.hyperledger.org gerrit query \ status:open project:fabric branch:master \
   | grep topic: | sort -u
```

- [gerrit.hyperledger.org]() Is the current URL where the project is hosted.
- *status* Indicates the topic's current status: open , merged, abandoned, draft,
merge conflict.
- *project* Refers to the current name of the project, in this case fabric.
- *branch* The topic is searched at this branch.
- *topic* The name of an specific topic, leave it blank to include them all.
- *sort* Sorts the found topics, in this case by update (-u).

## Downloading or Checking Out a Change

In the review UI, on the top right corner, the **Download** link provides a
list of commands and hyperlinks to checkout or download diffs or files.

We recommend the use of the *git review* plugin.
The steps to install git review are beyond the scope of this document.
Refer to the [git review documentation](https://wiki.openstack.org/wiki/Documentation/HowTo/FirstTimers) for the installation process.

To check out a specific change using Git, the following command usually works:

```
git review -d CHANGEID
```

If you don't have Git-review installed, the following commands will do the same
thing:

```
git fetch REMOTE refs/changes/NN/CHANGEIDNN/VERSION \ && git checkout FETCH_HEAD
```

For example, for the 4th version of change 2464, NN is the first two digits
(24):

```
git fetch REMOTE refs/changes/24/2464/4 \ && git checkout FETCH_HEAD
```

## Using Draft Branches

You can use draft branches to add specific reviewers before you publishing your
change.  The Draft Branches are pushed to `refs/drafts/master/TopicName`

The next command ensures a local branch is created:

```
git checkout -b BRANCHNAME
```

The next command pushes your change to the drafts branch under **TopicName**:

```
git push REMOTE HEAD:refs/drafts/master/TopicName
```

## Using Sandbox Branches

You can create your own branches to develop features. The branches are pushed to
the `refs/sandbox/USERNAME/BRANCHNAME` location.

These commands ensure the branch is created in Gerrit's server.

```
git checkout -b sandbox/USERNAME/BRANCHNAME
git push --set-upstream REMOTE HEAD:refs/heads/sandbox/USERNAME/BRANCHNAME
```

Usually, the process to create content is:

- develop the code,
- break the information into small commits,
- submit changes,
- apply feedback,
- rebase.

The next command pushes forcibly without review:

```
git push REMOTE sandbox/USERNAME/BRANCHNAME
```

You can also push forcibly with review:

```
git push REMOTE HEAD:ref/for/sandbox/USERNAME/BRANCHNAME
```

## Updating the Version of a Change

During the review process, you might be asked to update your change. It is
possible to submit multiple versions of the same change. Each version of the
change is called a patch set.

Always maintain the **Change-Id** that was assigned.
For example, there is a list of commits, **c0...c7**, which were submitted as a
topic branch:

```
git log REMOTE/master..master

c0
...
c7

git push REMOTE HEAD:refs/for/master/SOMETOPIC
```

After you get reviewers' feedback, there are changes in **c3** and **c4** that
must be fixed.  If the fix requires rebasing, rebasing changes the commit Ids,
see the [rebasing](http://git-scm.com/book/en/v2/Git-Branching-Rebasing) section
for more information. However, you must keep the same Change-Id and push the
changes again:

```
git push REMOTE HEAD:refs/for/master/SOMETOPIC
```

This new push creates a patches revision, your local history is then cleared.
However you can still access the history of your changes in Gerrit on the
`review UI` section, for each change.

It is also permitted to add more commits when pushing new versions.

## Rebasing

Rebasing is usually the last step before pushing changes to Gerrit; this allows
you to make the necessary *Change-Ids*.  The *Change-Ids* must be kept the same.

- **squash:** mixes two or more commits into a single one.
- **reword:** changes the commit message.
- **edit:** changes the commit content.
- **reorder:** allows you to interchange the order of the commits.
- **rebase:** stacks the commits on top of the master.

## Rebasing During a Pull

Before pushing a rebase to your master, ensure that the history has a
consecutive order.

For example, your `REMOTE/master` has the list of commits from **a0** to
**a4**; Then, your changes **c0...c7** are on top of **a4**; thus:

```
git log --oneline REMOTE/master..master

a0
a1
a2
a3
a4
c0
c1
...
c7
```

If `REMOTE/master` receives commits **a5**, **a6** and **a7**. Pull with a
rebase as follows:

```
git pull --rebase REMOTE master
```

This pulls **a5-a7** and re-apply **c0-c7** on top of them:


```
   $ git log --oneline REMOTE/master..master
   a0
   ...
   a7
   c0
   c1
   ...
   c7
```

## Getting Better Logs from Git

Use these commands to change the configuration of Git in order to produce better
logs:

```
git config log.abbrevCommit true
```

The command above sets the log to abbreviate the commits' hash.

```
git config log.abbrev 5
```

The command above sets the abbreviation length to the last 5 characters of the
hash.

```
git config format.pretty oneline
```

The command above avoids the insertion of an unnecessary line before the Author
line.

To make these configuration changes specifically for the current Git user,
you must add the path option `--global` to `config` as follows:
