# Making a documentation change

**Audience**: Anyone who would like to contribute to the Fabric documentation.

This short guide describes how the Fabric documentation is structured, built and
published, as well as a few conventions one should be aware of before making
changes to the Fabric documentation.

In this topic, we're going to cover:
* [An introduction to the documentation](#introduction)
* [Repository structure](#repository-structure)
* [Testing your changes](#testing-your-changes)
* [Building the documentation locally](#building-locally)
* [Building the documentation on GitHub and ReadTheDocs](#building-on-github)
* [Making a change to Commands Reference](#commands-reference-updates)
* [Adding a new CLI command](#adding-a-new-cli-command)

## Introduction

The Fabric documentation is written in a combination of
[Markdown](https://www.markdownguide.org/) and
[reStructuredText](http://docutils.sourceforge.net/rst.html), and as a new
author you can use either or both.  We recommend you use Markdown as an easy and
powerful way to get started; though if you have a background in Python, you may
prefer to use RST.

As part of the build process, the documentation source files are converted to
HTML using [Sphinx](http://www.sphinx-doc.org/en/stable/), and published
[here](http://hyperledger-fabric.readthedocs.io). There is a GitHub hook for the
main [Fabric repository](https://github.com/hyperledger/fabric) such that any
new or changed content in `docs/source` will trigger a new build and subsequent
publication of the doc.

Translations of the fabric documentation are available in different languages:

  * [Chinese documentation](https://hyperledger-fabric.readthedocs.io/zh_CN/latest/)
  * [Malayalam
    documentation](https://hyperledgerlabsml.readthedocs.io/en/latest/)
  * Brazilian Portuguese documentation -- coming soon

These are each built from their language-specific repository in the
[Hyperledger Labs](https://github.com/hyperledger-labs) organization.

For example:

 * [Chinese language repository](https://github.com/hyperledger-labs/fabric-docs-cn)
 * [Malayalam language repository](https://github.com/hyperledger-labs/fabric-docs-ml)
 * Brazilian Portuguese language repository -- coming soon

Once a language repository is nearly complete, it can contribute to the main
Fabric publication site. For example, the Chinese language docs are available on
the [main documentation
site](https://hyperledger-fabric.readthedocs.io/zh_CN/latest/).

## Repository structure

In each of these repositories, the Fabric docs are always kept under the `/docs`
top level folder.

```bash
(docs) bash-3.2$ ls -l docs
total 56
-rw-r--r--    1 user  staff   2107  4 Jun 09:42 Makefile
-rw-r--r--    1 user  staff    199  4 Jun 09:42 Pipfile
-rw-r--r--    1 user  staff  10924  4 Jun 09:42 Pipfile.lock
-rw-r--r--@   1 user  staff    288  4 Jun 14:50 README.md
drwxr-xr-x    4 user  staff    128  4 Jun 10:10 build
drwxr-xr-x    3 user  staff     96  4 Jun 09:42 custom_theme
-rw-r--r--    1 user  staff    283  4 Jun 09:42 requirements.txt
drwxr-xr-x  103 user  staff   3296  4 Jun 12:32 source
drwxr-xr-x   18 user  staff    576  4 Jun 09:42 wrappers
```

The files in this top level directory are largely configuration files for the
build process.  All the documentation is contained within the `/source` folder:

```bash
(docs) bash-3.2$ ls -l docs/source
total 2576
-rw-r--r--   1 user  staff   20045  4 Jun 12:33 CONTRIBUTING.rst
-rw-r--r--   1 user  staff    1263  4 Jun 09:42 DCO1.1.txt
-rw-r--r--   1 user  staff   10559  4 Jun 09:42 Fabric-FAQ.rst
drwxr-xr-x   4 user  staff     128  4 Jun 09:42 _static
drwxr-xr-x   4 user  staff     128  4 Jun 09:42 _templates
-rw-r--r--   1 user  staff   10995  4 Jun 09:42 access_control.md
-rw-r--r--   1 user  staff     353  4 Jun 09:42 architecture.rst
-rw-r--r--   1 user  staff   11020  4 Jun 09:42 blockchain.rst
-rw-r--r--   1 user  staff   75552  4 Jun 09:42 build_network.rst
-rw-r--r--   1 user  staff    9115  4 Jun 09:42 capabilities_concept.md
-rw-r--r--   1 user  staff    2851  4 Jun 09:42 capability_requirements.rst
...
```

These files and directories map directly to the documentation structure you see
in the [published docs](https://hyperledger-fabric.readthedocs.io/en/latest/).
Specifically, the table of contents has
[`index.rst`](https://github.com/hyperledger/fabric/blob/master/docs/source/index.rst)
as its root file, which links every other file in [`/docs/source`](https://github.com/hyperledger/fabric/tree/master/docs/source).

Spend some time navigating these directories and files to see how they are
linked together.

To update the documentation, you simply change one or more of these files using
git, build the change locally to check it's OK, and then submit a Pull Request
(PR) to the main Fabric repository. If the change is accepted by the
maintainers, it will be merged into the main Fabric repository and become part
of the documentation that is published.  It really is that easy!

You can learn how to make a PR [here](./github/github.html), but before you do
that, read on to see how to build your change locally first. Moreover, if you
are new to git and GitHub, you will find the [Git
book](https://git-scm.com/book/en/v2) invaluable.

## Testing your changes

You are strongly encouraged to test your changes to the documentation before you
submit a PR. You should start by building the docs on your own machine, and
subsequently push your changes to your own GitHub staging repo where they can
populate your [ReadTheDocs](https://readthedocs.org/) publication website. Once
you are happy with your change, you can submit it via a PR for inclusion in the
main Fabric repository.

The following sections cover first how to build the docs locally, and then
use your own Github fork to publish on ReadTheDocs.

## Building locally

Once you've cloned the Fabric [repository]() to your local machine, use these
quick steps to build the Fabric documentation on your local machine. Note: you
may need to adjust depending on your OS.

Prereqs:
 - [Python 3.7](https://wiki.python.org/moin/BeginnersGuide/Download)
 - [Pipenv](https://docs.pipenv.org/en/latest/#install-pipenv-today)

```
cd fabric/docs
pipenv install
pipenv shell
make html
```

This will generate all Fabric documentation html files in `docs/build/html`
which you can then start browsing locally using your browser; simply navigate to
the `index.html` file.

Make a small change to a file, and rebuild the documentation to verify that your
change was built locally. Every time you make a change to the documentation you
will of course need to rerun `make html`.

In addition, if you'd like, you may also run a local web server with the
following commands (or equivalent depending on your OS):

```
sudo apt-get install apache2
cd build/html
sudo cp -r * /var/www/html/
```

You can then access the html files at `http://localhost/index.html`.

## Building on GitHub

It can often be helpful to use your fork of the Fabric repository to build the
Fabric documentation in a public site, available to others. The following
instructions show you how to use ReadTheDocs to do this.

1. Go to http://readthedocs.org and sign up for an account.
2. Create a project. Your username will preface the URL and you may want to
   append `-fabric` to ensure that you can distinguish between this and other
   docs that you need to create for other projects. So for example:
   `YOURGITHUBID-fabric.readthedocs.io/en/latest`.
3. Click `Admin`, click `Integrations`, click `Add integration`, choose `GitHub
   incoming webhook`, then click `Add integration`.
4. Fork [Fabric on GitHub](https://github.com/hyperledger/fabric).
5. From your fork, go to `Settings` in the upper right portion of the screen.
6. Click `Webhooks`.
7. Click `Add webhook`.
8. Add the ReadTheDocs's URL into `Payload URL`.
9. Choose `Let me select individual events`:`Pushes`、`Branch or tag creation`、
   `Branch or tag deletion`.
10. Click `Add webhook`.

Now anytime you modify or add documentation content to your fork, this
URL will automatically get updated with your changes!

## Commands Reference updates

Updating content in the [Commands
Reference](https://hyperledger-fabric.readthedocs.io/en/latest/command_ref.html)
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