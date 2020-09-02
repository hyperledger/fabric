# Contributing documentation

**Audience**: Anyone who would like to contribute to the Fabric documentation.

This short guide describes how the Fabric documentation is structured, built and
published, as well as a few conventions that help writers make changes to the
Fabric documentation.

In this topic, we're going to cover:
* [An introduction to the documentation](#introduction)
* [Repository folder structure](#repository-folders)
* [International language folder structure](#international-folders)
* [Making documentation changes](#making-changes)
* [Building the documentation on your local machine](#building-locally)
* [Building the documentation on GitHub with ReadTheDocs](#building-on-github)
* [Getting your change approved](#making-a-pr)
* [Making a change to Commands Reference](#commands-reference-updates)
* [Adding a new CLI command](#adding-a-new-cli-command)

## Introduction

The Fabric documentation is written in a combination of
[Markdown](https://www.markdownguide.org/) and
[reStructuredText](http://docutils.sourceforge.net/rst.html) source files. As a
new author you can use either format. We recommend that you use Markdown as an
easy and powerful way to get started. If you have a background in Python, you
may prefer to use rST.

During the documentation build process, the documentation source files are
converted to HTML using [Sphinx](http://www.sphinx-doc.org/en/stable/). The
generated HTML files are subsequently published on the [public documentation
website](http://hyperledger-fabric.readthedocs.io). Users can select both
different languages and different versions of the Fabric documentation.

For example:

  * [Latest version of US English](https://hyperledger-fabric.readthedocs.io/en/{BRANCH_DOC}/)
  * [Latest version of Chinese](https://hyperledger-fabric.readthedocs.io/zh_CN/{BRANCH_DOC}/)
  * [Version 2.2 of US English](https://hyperledger-fabric.readthedocs.io/en/release-2.2/)
  * [Version 1.4 of US English](https://hyperledger-fabric.readthedocs.io/en/release-1.4/)

For historical reasons, the US English source files live in the main [Fabric
repository](https://github.com/hyperledger/fabric/), whereas all international
language source files live in a single [Fabric i18n
repository](https://github.com/hyperledger/fabric-docs-i18n). Different versions
of the documentation are held within the appropriate GitHub release branch.

## Repository folders

Both the US English and international language repositories have essentially the
same structure, so let's start by examining the US English source files.

All files relating to documentation reside within the `fabric/docs/` folder:

```bash
fabric/docs
├── custom_theme
├── source
│   ├── _static
│   ├── _templates
│   ├── commands
│   ├── create_channel
│   ├── dev-setup
│   ├── developapps
│   ├── diagrams
│   ...
│   ├── orderer
│   ├── peers
│   ├── policies
│   ├── private-data
│   ├── smartcontract
│   ├── style-guides
│   └── tutorial
└── wrappers
```

The most important folders is `source/` becuase it holds the source language
files. The documentation build process uses the `make` command to convert these
source files to HTML, which are stored in the dynamically created `build/html/`
folder:

```bash
fabric/docs
├── build
│   ├── html
├── custom_theme
├── source
│   ├── _static
│   ├── _templates
│   ├── commands
│   ├── create_channel
│   ├── dev-setup
│   ├── developapps
│   ├── diagrams
    ...
```

Spend a little time navigating the [docs
folder](https://github.com/hyperledger/fabric/tree/master/docs) in the
Hyperledger Fabric repository. Click on the following links to see how different
source files map to their corresponding published topics.

* [`/docs/source/index.rst`](https://raw.githubusercontent.com/hyperledger/fabric/master/docs/source/index.rst) maps to [Hyperledger Fabric title page](https://hyperledger-fabric.readthedocs.io/en/{RTD_TAG}/)

* [`/docs/source/developapps/developing-applications.rst`](https://raw.githubusercontent.com/hyperledger/fabric/master/docs/source/developapps/developing_applications.rst)
  maps to [Developing
  applications](https://hyperledger-fabric.readthedocs.io/en/{RTD_TAG}/developapps/developing_applications.html)

* [`/docs/source/peers/peers.md`](https://raw.githubusercontent.com/hyperledger/fabric/master/docs/source/peers/peers.md)
  maps to
  [Peers](https://hyperledger-fabric.readthedocs.io/en/{RTD_TAG}/peers/peers.html)

We'll see how to make changes to these files a little later.

## International folders
The international language repository,
[`fabric-docs-i18n`](https://github.com/hyperledger/fabric-docs-i18n), follows
almost exactly the same structure as the
[`fabric`](https://github.com/hyperledger/fabric) repository which holds the US
English files.  The difference is that each language is located within its own
folder within `docs/locale/`:

```bash
fabric-docs-i18n/docs
└── locale
    ├── ja_JP
    ├── ml_IN
    ├── pt_BR
    └── zh_CN
```
Examining any one of these folders shows a familiar structure:
```bash
locale/ml_IN
├── custom_theme
├── source
│   ├── _static
│   ├── _templates
│   ├── commands
│   ├── dev-setup
│   ├── developapps
│   ├── diagrams
│   ...
│   ├── orderer
│   ├── peers
│   ├── policies
│   ├── private-data
│   ├── smartcontract
│   ├── style-guides
│   └── tutorial
└── wrappers
```

As we'll soon see, the similarity of the international language and US English
folder structures means that the same instructions and commands can be used to
manage different language translations.

Again, spend some time examining the [international language
repository](https://github.com/hyperledger/fabric-docs-i18n).

## Making changes

To update the documentation, you simply change one or more language source files
in a local git feature branch, build the changes locally to check they're OK,
and submit a Pull request (PR) to merge your branch with the appropriate Fabric
repository branch. Once your PR has been reviewed and approved by the appropriate
maintainers for the language, it will be merged into the repository and become
part of the published documentation. It really is that easy!

As well as being polite, it's a really good idea to test any documentation
changes before you request to include it in a repository. The following sections
show you how to:

* Build and review a documentation change on your own machine.


* Push these changes to your GitHub repository fork where they can populate your
  personal [ReadTheDocs](https://readthedocs.org/) publication website for
  collaborators to review.


* Submit your documentation PR for inclusion in the `fabric` or
  `fabric-docs-i18n` repository.

## Building locally

Use these simple steps to build the documentation.

1. Create a fork of the appropriate
[`fabric`](https://github.com/hyperledger/fabric) or
[`fabric-i18n`](https://github.com/hyperledger/fabric-docs-i18n) repository to
your GitHub account.

2. Install the following prerequisites; you may need to adjust depending on your
   OS:

   * [Docker](https://docs.docker.com/get-docker/)

3. For US English:
   ```bash
   git clone git@github.com:hyperledger/fabric.git
   cd fabric
   make docs
   ```

   For International Languages (Malayalam as an example):
   ```bash
   git clone git@github.com:hyperledger/fabric-docs-i18n.git
   cd fabric
   make docs-lang-ml_IN
   ```

   The `make` command generates documentation html files in the `build/html/`
   folder which you can now view locally; simply navigate your browser to the
   `build/html/index.html` file.

4. Now make a small change to a file, and rebuild the documentation to verify
   that your change was as expected. Every time you make a change to the
   documentation you will of course need to rerun `make docs`.

5. If you'd like, you may also run a local web server with the following
   commands (or equivalent depending on your OS):

   ```bash
   sudo apt-get install apache2
   cd build/html
   sudo cp -r * /var/www/html/
   ```

   You can then access the html files at `http://localhost/index.html`.

6. You can learn how to make a PR [here](./github/github.html). Moreover, if you
   are new to git or GitHub, you will find the [Git
   book](https://git-scm.com/book/en/v2) invaluable.

## Building on GitHub

It is often helpful to use your fork of the Fabric repository to build the
Fabric documentation so that others can review your changes before you submit
them for approval. The following instructions show you how to use ReadTheDocs to
do this.

1. Go to [`http://readthedocs.org`](http://readthedocs.org) and sign up for an
   account.
2. Create a project. Your username will preface the URL and you may want to
   append `-fabric` to ensure that you can distinguish between this and other
   docs that you need to create for other projects. So for example:
   `YOURGITHUBID-fabric.readthedocs.io/en/{BRANCH_DOC}`.
3. Click `Admin`, click `Integrations`, click `Add integration`, choose `GitHub
   incoming webhook`, then click `Add integration`.
4. Fork the [`fabric`](https://github.com/hyperledger/fabric) repository.
5. From your fork, go to `Settings` in the upper right portion of the screen.
6. Click `Webhooks`.
7. Click `Add webhook`.
8. Add the ReadTheDocs's URL into `Payload URL`.
9. Choose `Let me select individual events`:`Pushes`、`Branch or tag creation`、
   `Branch or tag deletion`.
10. Click `Add webhook`.

Use `fabric-docs-i18n` instead of `fabric` in the above instructions if you're
building an international language translation.

Now, anytime you modify or add documentation content to your fork, this URL will
automatically get updated with your changes!

## Making a PR

You can submit your PR for inclusion using the following
[instructions](./github/github.html).

Pay special attention to signing your commit with the `-s` option:

```bash
git commit -s -m "My Doc change"
```

This states that your changes conform to the [Developer Certificate of
Origin](https://en.wikipedia.org/wiki/Developer_Certificate_of_Origin).

Before your PR can be included in the appropriate `fabric` or `fabric-docs-i18n`
repository, it must be approved by an appropriate maintainer. For example, a
Japanese translation must be approved by a Japanese maintainer, and so on. You
can find the maintainers listed in the following `CODEOWNERS` files:

* US English
  [`CODEOWNERS`](https://github.com/hyperledger/fabric/blob/master/CODEOWNERS)
  and their [maintainer GitHub
  IDs](https://github.com/orgs/hyperledger/teams/fabric-core-doc-maintainers)
* International language
  [`CODEOWNERS`](https://github.com/hyperledger/fabric-docs-i18n/blob/master/CODEOWNERS)
  and their [maintainer GitHub
  IDs](https://github.com/orgs/hyperledger/teams/fabric-contributors)

Both language repositories have a GitHub webhook defined so that, once approved,
your newly merged content in the `docs/` folder will trigger an automatic build
and publication of the updated documentation.  

**Note:** Documentation maintainers are not able to to merge documentation PRs by clicking the `Merge pull request` button. Instead, if you are a documentation maintainer and have approved the PR, simply add the label `doc-merge` to the PR and a `Mergify` bot that runs every minute will merge the PR.


## Commands Reference updates

Updating content in the [Commands
Reference](https://hyperledger-fabric.readthedocs.io/en/{BRANCH_DOC}/command_ref.html)
topic requires additional steps. Because the information in the Commands
Reference topic is generated content, you cannot simply update the associated
markdown files.
- Instead you need to update the `_preamble.md` or `_postscript.md` files under
  `src/github.com/hyperledger/fabric/docs/wrappers` for the command.
- To update the command help text, you need to edit the associated `.go` file
  for the command that is located under `/fabric/internal/peer`.
- Then, from the `fabric` folder, you need to run the command `make help-docs`
  which generates the updated markdown files under `docs/source/commands`.

Remember that when you push the changes to GitHub, you need to include the
`_preamble.md`, `_postscript.md` or `_.go` file that was modified as well as the
generated markdown file.

This process only applies to English language translations. Command Reference
translation is currently not possible in international languages.

## Adding a new CLI command

To add a new CLI command, perform the following steps:

- Create a new folder under `/fabric/internal/peer` for the new command and the
  associated help text. See `internal/peer/version` for a simple example to get
  started.
- Add a section for your CLI command in
  `src/github.com/hyperledger/fabric/scripts/generateHelpDoc.sh`.
- Create two new files under `/src/github.com/hyperledger/fabric/docs/wrappers`
  with the associated content:
  - `<command>_preamble.md` (Command name and syntax)
  - `<command>_postscript.md` (Example usage)
- Run `make help-docs` to generate the markdown content and push all of the
  changed files to GitHub.

This process only applies to English language translations. CLI
command translation is currently not possible in international languages.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
