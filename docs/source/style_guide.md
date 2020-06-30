# Style guide for contributors

**Audience**: documentation writers and editors

While this style guide will also refer to best practices using ReStructured Text (also known as RST), in general we advise writing documentation in Markdown, as it's a more universally accepted documentation standard. Both formats are usable, however, and if you decide to write a topic in RST (or are editing an RST topic), be sure to refer to this style guide.

**When in doubt, use the docs themselves for guidance on how to format things.**

* [For RST formatting](http://hyperledger-fabric.readthedocs.io/en/release-1.4/channel_update_tutorial.html).

* [For Markdown formatting](http://hyperledger-fabric.readthedocs.io/en/release-1.4/peers/peers.html).

If you just want to look at how things are formatted, you can navigate to the Fabric repo to look at the raw file by clicking on `Edit on Github` link in the upper right hand corner of the page. Then click the `Raw` tab. This will show you the formatting of the doc. **Do not attempt to edit the file on Github.** If you want to make a change, clone the repo and follow the instructions in [Contributing](./CONTRIBUTING.html) for creating pull requests.

## Word choices

**Avoid the use of the words "whitelist", "blacklist", "master", or "slave".**

Unless the use of these words is absolutely necessary (for example, when quoting a section of code that uses them), do not use these words. Either be more explicit (for example, describing what "whitelisting" actually does) or find alternate words such as "allowlist" or "blocklist".

**Tutorials should have a list of steps at the top.**

A list of steps (with links to the corresponding sections) at the beginning of a tutorial helps users find particular steps they're interested in. For an example, check out [Use private data in Fabric](./private-data/private-data.html).

**"Fabric", "Hyperledger Fabric" or "HLF"?**

The first usage should be “Hyperledger Fabric” and afterwards only “Fabric”. Don't use "HLF" or "Hyperledger" by itself.

**Chaincode vs. Chaincodes?**

One chaincode is a “chaincode”. If you’re talking about several chaincodes, use "chaincodes".

**Smart contracts?**

Colloquially, smart contracts are considered equivalent to chaincode, though at a technical level, it is more correct to say that a "smart contract" is the business logic inside of a chaincode, which encompasses the larger packaging and implementation.

**JSON vs .json?**

Use “JSON”. The same applies for any file format (for example, YAML).

**curl vs cURL.**

The tool is called “cURL”. The commands themselves are “curl” commands.

**Fabric CA.**

Do not call it "fabric-CA", "fabricCA", or FabricCA. It is the Fabric CA. The Fabric CA client binary can, however, be referred to as the `fabric-ca-client`.

**Raft and RAFT.**

"Raft" is not an acronym. Do not call it a "RAFT ordering service".

**Referring to the reader.**

It’s perfectly fine to use the “you” or “we”. Avoid using "I".

**Ampersands (&).**

Not a substitute for the word “and”. Avoid them unless you have a reason to use it (such as in a code snippet that includes it).

**Acronyms.**

The first usage of an acronym should be spelled out, unless it’s an acronym that’s in such wide usage this is unneeded. For example, “Software Development Kit (SDK)” on first usage. Then use “SDK” afterward.

**Try to avoid using the same words too often.**

If you can avoid using a word twice in one sentence, please do so. Not using it more than twice in a single paragraph is better. Of course sometimes it might not be possible to avoid this –-- a doc about the state database being used is likely to be replete with uses of the word “database” or “ledger”. But excessive usage of any particular word has a tendency to have a numbing effect on the reader.

**How should files be named?**

By using underscores between words. Also, tutorials should be named as such. For example, `identity_use_case_tutorial.md`. While not all files use this standard, new files should adhere to it.

## Formatting and punctuation

**Line lengths.**

If you look at the raw versions of the documentation, you will see that many topics conform to a line length of roughly 70 characters. This restriction is no longer necessary, so you are free to make lines as long as you want.

**When to bold?**

Not too often. The best use of them is either as a summary or as a way of drawing attention to concepts you want to talk about. “A blockchain network contains a ledger, at least one chaincode, and peers”, especially if you’re going to be talking about those things in that paragraph. Avoid using them simply to emphasize a single word, as in something like "Blockchain networks **must** use propery security protocols".

**When to surround something in back tics `nnn`?**

This is useful to draw attention to words that either don’t make sense in plain English or when referencing parts of the code (especially if you’ve put code snippets in your doc). So for example, when talking about the fabric-samples directory, surround `fabric-samples` with back tics. Same with a code function like `hf.Revoker`. It might also make sense to put back tics around words that do make sense in plain English that are part of the code if you're referencing them in a code context. For example, when referencing an `attribute` as part of an Access Control List.

**Is it ever appropriate to use a dash?**

Dashes can be incredibly useful but they're not necessarily as technically appropriate as using separate declarative sentences. Let's consider this example sentence:

```
This leaves us with a trimmed down JSON object --- config.json, located in the fabric-samples folder inside first-network --- which will serve as the baseline for our config update.
```

There are a number of ways to present this same information, but in this case the dashes break up the information while keeping it as part of the same thought. If you use a dash, make sure to use the "em" dash, which is three times longer than a hyphen. These dashes should have a space before and after them.

**When to use hyphens?**

Hyphens are mostly commonly used as part of a “compound adjective”. For example, "jet-powered car". Note that the compound adjective must immediately precede the noun being modified. In other words, "jet powered" does not by itself need a hyphen. When in doubt, use Google, as compound adjectives are tricky and are a popular discussion on grammar discussion boards.

**How many spaces after a period?**

One.

**How should numbers be rendered?**

Number zero through nine are spelled out. One, two, three, four, etc. Numbers after 10 are rendered as numbers.

Exceptions to this would be usages from code. In that case, use whatever’s in the code. And also examples like Org1. Don’t write it as OrgOne.

**Capitalization rules for doc titles.**

The standard rules for capitalization in sentences should be followed. In other words, unless a word is the first word in the title or a proper noun, do not capitalize its first letter. For example, "Understanding Identities in Fabric" should be "Understanding identities in Fabric". While not every doc follows this standard yet, it is the standard we're moving to and should be followed for new topics.

Headings inside of topics should follow the same standard.

**Use the Oxford comma?**

Yes, it’s better.

The classic example is, “I’d like to thank my parents, Ayn Rand and God”, as compared to: “I’d like to thank my parents, Ayn Rand, and God.”

**Captions.**

These should be in italics, and it’s the only real valid use for italics in our docs.

**Commands.**

In general, put each command in its own snippet. It reads better, especially when commands are long. An exception to this rule is when suggesting the export of a number of environment variables.

**Code snippets.**

In Markdown, if you want to post sample code, use three back tics to set off the snippet. For example:

```
Code goes here.

Even more code goes here.

And still more.
```

In RST, you will need to set off the code snippet using formatting similar to this:

```
.. code:: bash

   Code goes here.
```

You can substitute `bash` for a language like Java or Go, where appropriate.

**Enumerated lists in markdown.**

Note that in Markdown, enumerated lists will not work if you separate the numbers with a space. Markdown sees this as the start of a new list, not a continuation of the old one (every number will be `1.`). If you need an enumerated list, you will have to use RST. Bulleted lists are a good substitute in Markdown, and are the recommended alternative.

**Linking.**

When linking to another doc, use relative links, not direct links. When naming a link, do not just call it "link". Use a more creative and descriptive name. For accessibility reasons, the link name should also make it clear that it is a link.

**All docs have to end with a license statement.**

In RST, it’s this:

```
.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
```

In markdown:

```
<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
```

**How many spaces for indentation?**

This will depend on the use case. Frequently it’s necessary, especially in RST, to indent two spaces, especially in a code block. In a `.. note::` box in RST, you have to indent to the space after the colon after note, like this:

```
.. note:: Some words and stuff etc etc etc (line continues until the 70 character limit line)
          the line directly below has to start at the same space as the one above.
```

**When to use which type of heading.**

In RST, use this:

```
Chapter 1 Title
===============

Section 1.1 Title
-----------------

Subsection 1.1.1 Title
~~~~~~~~~~~~~~~~~~~~~~

Section 1.2 Title
-----------------
```

Note that the length of what’s under the title has to be the same as the length of the title itself. This isn’t a problem in Atom, which gives each character the same width by default (this is called “monospacing”, if you’re ever on Jeopardy! and need that information.

In markdown, it’s somewhat simpler. You go:

```
# The Name of the Doc (this will get pulled for the TOC).

## First subsection

## Second subsection
```

Both file formats don't like when these things are done out of order. For example, you might want a `####` to be the first thing after your `#` Title. Markdown won’t allow it. Similarly, RST will default to whatever order you give to the title formats (as they appear in the first sections of your doc).

**Relative links should be used whenever possible.**

  For RST, the preferred syntax is:
  ```
    :doc:`anchor text <relativepath>`
  ```
  Do not put the .rst suffix at the end of the filepath.

  For Markdown, the preferred syntax is:
  ```
    [anchor text](<relativepath>)
  ```

  For other files, such as text or YAML files, use a direct link to the file in
  github for example:

  [https://github.com/hyperledger/fabric/blob/master/docs/README.md](https://github.com/hyperledger/fabric/blob/master/docs/README.md)

  Relative links are unfortunately not working on github when browsing through a
  RST file.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
