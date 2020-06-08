# International languages

**Audience**: Anyone who would like to contribute to the Fabric documentation in
a language other than English.

This short guide describes how you can make a change to one of the many
languages that Fabric supports. If you're just getting started, this guide will
show you how to join an existing language translation workgroup, or how to start
a new workgroup if your chosen language is not available.

In this topic, we're going to cover:
* [An introduction to Fabric language support](#introduction)
* [How to join an existing language workgroup](#joining-a-workgroup)
* [How to starting a new language workgroup](#starting-a-workgroup)
* [Getting connected to other language contributors](#get-connected)

## Introduction

The [main Fabric repository](https://github.com/hyperledger/fabric) is located
in GitHub under the Hyperledger organization. It contains an English translation
of the documentation in the `/docs/source` folder. When built, the files in this
folder result contribute to the in the [documentation
website](https://hyperledger-fabric.readthedocs.io/en/latest/).

This website has other language translations available, such as
[Chinese](https://hyperledger-fabric.readthedocs.io/zh_CN/latest/). However,
these languages are built from specific language repositories hosted in the [HL
Labs organization](https://github.com/hyperledger-labs). For example, the
Chinese language documentation is stored in [this
repository](https://github.com/hyperledger-labs/fabric-docs-cn).

Language repositories have a cut-down structure; they just contain documentation
related folders and files:

```bash
(base) user/git/fabric-docs-ml ls -l
total 48
-rw-r--r--   1 user  staff  11357 14 May 10:47 LICENSE
-rw-r--r--   1 user  staff   3228 14 May 17:01 README.md
drwxr-xr-x  12 user  staff    384 15 May 07:40 docs
```

Because this structure is a subset of the main Fabric repo, you can use the same
tools and processes to contribute to any language translation; you simply work
against the appropriate repository.

## Joining a workgroup

While the default Hyperledger Fabric language is English, as we've seen, other
translations are available. The [Chinese language
documentation](https://hyperledger-fabric.readthedocs.io/zh_CN/latest/) is well
progressed, and other languages such as Brazilian Portuguese and Malayalam are
in progress.

You can find a list of all [current international language
groups](https://wiki.hyperledger.org/display/fabric/International+groups) in the
Hyperledger wiki. These groups have lists of active members that you can connect
with. They hold regular meetings that you are welcome to join.

Feel free to follow [these instructions](./docs_guide.html) to contribute a
documentation change to any of the language repositories. Here's a list of
current language repositories:

* [English](https://github.com/hyperledger/fabric/tree/master/docs)
* [Chinese](https://github.com/hyperledger-labs/fabric-docs-cn)
* [Malayalam](https://github.com/hyperledger-labs/fabric-docs-ml)
* [Brazilian Portuguese]() -- coming soon.

## Starting a workgroup

If your chosen language is not available, then why not start a new language
translation? It's relatively easy to get going. A workgroup can help you
organize and share the work to translate, maintain, and manage a language
repository. Working with other contributors and maintainers on a language
translation can be a very satisfying activity for you and other Fabric users.

Follow these instructions to create your own language repository. Our
instructions will use the Sanskrit language as an example:

1. Identify the [ISO
   639-1](https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes) two letter
   international language code for your language. Sanskrit has the two letter
   code `sa`.

1. Clone the main Fabric repository to your local machine, renaming the
   repository during the clone:
   ```bash
   git clone git@github.com:hyperledger/fabric.git fabric-docs-sa
   ```

1. Select the Fabric version you are going to use as a baseline. We
   recommend that you start with at least Fabric 2.1, and ideally a Long Term
   Support version such as 2.2. You can add other releases later.

   ```bash
   cd fabric-docs-sa
   git fetch origin
   git checkout release-2.1
   ```

1. Remove all folders from the root directory except `/doc`. Likewise, remove
   all files (including hidden ones) except `LICENCE` and `README.md`, so that
   you are left with the following:

   ```bash
   ls -l
   total 40
   -rw-r--r--   1 user  staff  11358  5 Jun 14:38 LICENSE
   -rw-r--r--   1 user  staff   4822  5 Jun 15:09 README.md
   drwxr-xr-x  11 user  staff    352  5 Jun 14:38 docs
   ```

1. Update the `README.md` file to using [this
   one](https://github.com/hyperledger-labs/fabric-docs-ml/blob/master/README.md)
   as an example.

   Customize the `README.md` file for your new language.

1. Add a `.readthedocs.yml` file [like this
   one](https://github.com/hyperledger-labs/fabric-docs-ml/blob/master/.readthedocs.yml)
   to the top level folder.  This file is configured to disable ReadTheDocs PDF
   builds which may fail if you use non-latin character sets. Your top level
   repository folder will now look like this:

   ```bash
   (base) anthonyodowd/git/fabric-docs-sa ls -al
   total 96
   ...
   -rw-r--r--   1 anthonyodowd  staff    574  5 Jun 15:49 .readthedocs.yml
   -rw-r--r--   1 anthonyodowd  staff  11358  5 Jun 14:38 LICENSE
   -rw-r--r--   1 anthonyodowd  staff   4822  5 Jun 15:09 README.md
   drwxr-xr-x  11 anthonyodowd  staff    352  5 Jun 14:38 docs
   ```

1. Commit these changes locally to your local repository:

   ```bash
   git add .
   git commit -s -m "Initial commit"
   ```

1. Create a new repository on your GitHub account called `fabric-docs-sa`. In
   the description field type `Hyperledger Fabric documentation in Sanskrit
   language`.

1. Update your local git `origin` to point to this repository, replacing
   `YOURGITHUBID` with your GitHub ID:

   ```bash
   git remote set-url origin git@github.com:YOURGITHUBID/fabric-docs-sa.git
   ```

   At this stage in the process, an `upstream` cannot be set because the
   `fabric-docs-sa` repository hasn't yet been created in the HL Labs
   organization; we'll do this a little later.

   For now, confirm that the `origin` is set:

   ```bash
   git remote -v
   origin	git@github.com:ODOWDAIBM/fabric-docs-sa.git (fetch)
   origin	git@github.com:ODOWDAIBM/fabric-docs-sa.git (push)
   ```

1. Push your `release-2.1` branch to be the `master` branch in this repository:

   ```bash
   git push origin release-2.1:master

   Enumerating objects: 6, done.
   Counting objects: 100% (6/6), done.
   Delta compression using up to 8 threads
   Compressing objects: 100% (4/4), done.
   Writing objects: 100% (4/4), 1.72 KiB | 1.72 MiB/s, done.
   Total 4 (delta 1), reused 0 (delta 0)
   remote: Resolving deltas: 100% (1/1), completed with 1 local object.
   To github.com:ODOWDAIBM/fabric-docs-sa.git
   b3b9389be..7d627aeb0  release-2.1 -> master
   ```

1. Verify that your new repository `fabric-docs-sa` is correctly
   populated on GitHub under the `master` branch.

1. Connect your repository to ReadTheDocs using these
   [instructions](./docs_guide.html#building-on-github). Verify that your
   documentation builds correctly.

1. You can now perform translation updates in `fabric-docs-sa`.

   We recommend that you translate at least the [Fabric front
   page](https://hyperledger-fabric.readthedocs.io/en/latest/) and
   [Introduction](https://hyperledger-fabric.readthedocs.io/en/latest/whatis.html)
   before proceeding. This way, users will be clear on your intention to
   translate the Fabric documentation, which will help you gain contributors.
   More on this [later](#get-connected).

1. When you are happy with your repository, create a request to make an
   equivalent repository in the Hyperledger Labs organization, following [these
   instructions](https://github.com/hyperledger-labs/hyperledger-labs.github.io).

   Here's an [example
   PR](https://github.com/hyperledger-labs/hyperledger-labs.github.io/pull/126)
   to show you the process at work.

1. Once your repository has been approved, you can now add an `upstream`:

   ```bash
   git remote add upstream git@github.com:hyperledger-labs/fabric-docs-sa.git
   ```

   Confirm that your `origin`  and `upstream` remotes are now set:

   ```bash
   git remote -v
   origin	git@github.com:ODOWDAIBM/fabric-docs-sa.git (fetch)
   origin	git@github.com:ODOWDAIBM/fabric-docs-sa.git (push)
   upstream	git@github.com:hyperledger-labs/fabric-docs-sa.git (fetch)
   upstream	git@github.com:hyperledger-labs/fabric-docs-sa.git (push)
   ```

   Congratulations! You're now ready to build a community of contributors for
   your newly-created international language repository.

## Get connected

Here's a few ways to connected with other people interested in international
language translations:

  * Rocket chat

    Read the conversation or ask a question on the [Fabric documentation rocket
    channel](https://chat.hyperledger.org/channel/fabric-documentation). You'll
    find beginners and experts sharing information on the documentation.

    There also a dedicated channel for issues specific to
    [internationalization](https://chat.hyperledger.org/channel/i18n).


  * Join a documentation workgroup call

    A great place to meet people working on documentation is on a workgroup
    call. These are held regularly at a time convenient for both the Easten and
    Western hemispheres. The agenda is published in advance, and there are
    minutes and recordings of each session.  [Find out
    more](https://wiki.hyperledger.org/display/fabric/Documentation+Working+Group).


  * Join a language translation workgroup

    Each of the international languages has a workgroup that welcome and
    encouraged to join. View the [list of international
    workgroups](https://wiki.hyperledger.org/display/fabric/International+groups).
    See what your favourite workgroup is doing, and get connected with them;
    each workgroup has a list of members and their contact information.


  * Create a language translation workgroup page

    If you've decided to create a new language translation, then add a new
    workgroup to the [list of internationl
    workgroups](https://wiki.hyperledger.org/display/fabric/International+groups),
    using one of the existing workgroup pages as an exemplar.

    It's worth documentating how your workgroup will collaborate; meetings,
    chat and mailing lists can all be very effective. Making these mechanisms clear on your workgroup page can help build a community of translators.


  * Use one of the many other Fabric mechanisms such as mailing list,
    contributor meetings, maintainer meetings. Read more
    [here](./contributing.html).

Good luck and thank you for contributing to Hyperledger Fabric.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->