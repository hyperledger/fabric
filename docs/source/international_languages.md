# Creating a new translation

**Audience**: Writers who would like to create a new Fabric translation.

If the Hyperledger Fabric documentation is not available in your chosen language
then why not start a new language translation? It's relatively easy to get
started, and creating a new language translation can be a very satisfying
activity for you and other Fabric users.

In this topic, we're going to cover:
* [An introduction to international language support](#introduction)
* [How to create a new language workgroup](#create-a-new-workgroup)
* [How to create a new language translation](#create-a-new-translation)

## Introduction

Hyperledger Fabric documentation is being translated into many different
languages. For example:

* [Chinese](https://github.com/hyperledger/fabric-docs-i18n/tree/master/docs/locale/zh_CN)
* [Malayalam](https://github.com/hyperledger/fabric-docs-i18n/tree/master/docs/locale/ml_IN)
* [Brazilian Portuguese](https://github.com/hyperledger/fabric-docs-i18n/tree/master/docs/locale/pt_BR)
* [Japanese](https://github.com/hyperledger/fabric-docs-i18n/tree/master/docs/locale/ja_JP)

If your chosen language is not available, then the first thing to do is to
create a new language workgroup.

## Create a new workgroup

It's much easier to translate, maintain, and manage a language repository if you
collaborate with other translators. Start this process by adding a new workgroup
to the [list of international
workgroups](https://wiki.hyperledger.org/display/fabric/International+groups),
using one of the existing workgroup pages as an exemplar.

Document how your workgroup will collaborate; meetings, chat and mailing lists
can all be very effective. Making these mechanisms clear on your workgroup page
can help build a community of translators.

Then use [Rocket chat channels](./advice_for_writers.html#rocket-chat) to let
other people know you've started a translation, and invite them to join the
workgroup.

## Create a new translation

Follow these instructions to create your own language repository. Our sample
instructions will show you how to create a new language translation for Spanish
as spoken in Mexico:

1. Fork the [`fabric-docs-i18n`
   repository](https://github.com/hyperledger/fabric-docs-i18n) to your GitHub
   account.

1. Clone your repository fork to your local machine:
   ```bash
   git clone git@github.com:YOURGITHUBID/fabric-docs-i18n.git
   ```

1. Select the Fabric version you are going to use as a baseline. We recommend
   that you start with Fabric 2.2 as this is an LTS release. You can add other
   releases later.

   ```bash
   cd fabric-docs-i18n
   git fetch origin
   git checkout release-2.2
   ```
1. Create a local feature branch:
   ```bash
   git checkout -b newtranslation
   ```
1. Identify the appropriate [two or four letter language
   code](http://www.localeplanet.com/icu/).  Mexican Spanish has the language
   code `es_MX`.

1. Update the fabric
   [`CODEOWNERS`](https://github.com/hyperledger/fabric-docs-i18n/blob/master/CODEOWNERS) file
   in the root directory. Add the following line:
   ```bash
   /docs/locale/ex_EX/ @hyperledger/fabric-core-doc-maintainers @hyperledger/fabric-es_MX-doc-maintainers
   ```

1. Create a new language folder under `docs/locale/` for your language.
   ```bash
   cd docs/locale
   mkdir es_MX
   ```

1. Copy the language files from another language folder, for example
   ```bash
   cp -R pt_BR/ es_MX/
   ```
   Alternatively, you could copy the `docs/` folder from the `fabric`
   repository.

1. Customize the `README.md` file for your new language using [this
   example](https://github.com/hyperledger/fabric-docs-i18n/tree/master/docs/locale/pt_BR/README.md).

1. Commit your changes locally:
   ```
   git add .
   git commit -s -m "First commit for Mexican Spanish"
   ```

1. Push your `newtranslation` local feature branch to the `release-2.2` branch
   of your forked `fabric-docs-i18n` repository:

   ```bash
   git push origin release-2.2:newtranslation


   Total 0 (delta 0), reused 0 (delta 0)
   remote:
   remote: Create a pull request for 'newtranslation' on GitHub by visiting:
   remote:      https://github.com/YOURGITHUBID/fabric-docs-i18n/pull/new/newtranslation
   remote:
   To github.com:ODOWDAIBM/fabric-docs-i18n.git
   * [new branch]      release-2.2 -> newtranslation
   ```

1. Connect your repository fork to ReadTheDocs using these
   [instructions](./docs_guide.html#building-on-github). Verify that your
   documentation builds correctly.

1. Create a pull request (PR) for `newtranslation` on GitHub by visiting:

   [`https://github.com/YOURGITHUBID/fabric-docs-i18n/pull/new/newtranslation`](https://github.com/YOURGITHUBID/fabric-docs-i18n/pull/new/newtranslation)

   Your PR needs to be approved by one of the [documentation
   maintainers](https://github.com/orgs/hyperledger/teams/fabric-core-doc-maintainers).
   They will be automatically informed of your PR by email, and you can contact
   them via Rocket chat.

1. On the [`i18n rocket channel`](https://chat.hyperledger.org/channel/i18n)
   request the creation of the new group of maintainers for your language,
   `@hyperledger/fabric-es_MX-doc-maintainers`. Provide your GitHubID for
   addition to this group.

   Once you've been added to this list, you can add others translators from your
   workgroup.

Congratulations! A community of translators can now translate your newly-created
language in the `fabric-docs-i18n` repository.

## First topics

Before your new language can be published to the documentation website, you must
translate the following topics.  These topics help users and translators of your
new language get started.

* [Fabric front page](https://hyperledger-fabric.readthedocs.io/zh_CN/latest/)

  This is your advert! Thanks to you, users can now see that the documentation
  is available in their language. It might not be complete yet, but its clear
  you and your team are trying to achieve. Translating tis page will help you
  recruit other translators.


* [Introduction](https://hyperledger-fabric.readthedocs.io/en/latest/whatis.html)

  This short topic gives a high level overview of Fabric, and because it's
  probably one of the first topics a new user will look at, it's important that
  it's translated.


* [Contributions Welcome!](https://hyperledger-fabric.readthedocs.io/en/latest/CONTRIBUTING.html)

  This topic is vital -- it helps contributors understand **what**, **why** and
  **how** of contributing to Fabric. You need to translate this topic so that
  others can help you collaborate in your translation.


* [Glossary](https://hyperledger-fabric.readthedocs.io/en/latest/glossary.html)

  Translating this topic provides the essential reference material that helps
  other language translators make progress; in short, it allows your workgroup
  to scale.

Once this set of topics have been translated, and you've created a language
workgroup, your translation can be published on the documentation website. For
example, the Chinese language docs are available
[here](https://hyperledger-fabric.readthedocs.io/zh_CN/latest/).

You can now request, via the [`i18n rocket
channel`](https://chat.hyperledger.org/channel/i18n), that your translation is
included on the documentation website.

## Translation tools

When translating topics from US English to your international language, it's
often helpful to use an online tool to generate a first pass of the translation,
which you then correct in a second pass review.

Language workgroups have found the following tools helpful:

* [`DocTranslator`](https://www.onlinedoctranslator.com/)

* [`TexTra`](https://mt-auto-minhon-mlt.ucri.jgn-x.jp/)

## Suggested next topics

Once you have published the mandatory initial set of topics on the documentation
website, you are encouraged to translate these topics, in order. If you choose
to adopt another order, that's fine; you still will find it helpful to agree an
order of translation in your workgroup.

* [Key concepts](https://hyperledger-fabric.readthedocs.io/en/latest/key_concepts.html)

    For solution architects, application architects, systems architects, developers,
    academics and students alike, this topic provides a comprehensive conceptual
    understanding of Fabric.


* [Getting started](https://hyperledger-fabric.readthedocs.io/en/latest/getting_started.html)

  For developers who want to get hands-on with Fabric, this topic provides key
  instructions to help install, build a sample network and get hands-on with
  Fabric.


* [Developing applications](https://hyperledger-fabric.readthedocs.io/en/latest/developapps/developing_applications.html)

  This topic helps developers write smart contracts and applications; these
  are the core elements of any solution that uses Fabric.


* [Tutorials](https://hyperledger-fabric.readthedocs.io/en/latest/tutorials.html)

  A set of hands-on tutorials to help developers and administrators try out
  specific Fabric features and capabilities.


* [What's new in Hyperledger Fabric
  v2.x](https://hyperledger-fabric.readthedocs.io/en/latest/whatsnew.html)

  This topic covers the latest features in Hyperledger Fabric.


Finally, we wish you good luck, and thank you for your contribution to
Hyperledger Fabric.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->