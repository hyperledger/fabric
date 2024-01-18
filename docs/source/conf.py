# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# Configuration file for the Sphinx documentation builder.
#
# For the full list of built-in configuration values, see the documentation:
# https://www.sphinx-doc.org/en/master/usage/configuration.html

import os
import sys
from os import environ
sys.path.insert(0, os.path.abspath('.'))

import sphinx_rtd_theme

rtd_tag = 'latest'
if environ.get('READTHEDOCS_VERSION') is not None:
    rtd_tag = os.environ['READTHEDOCS_VERSION']


placeholder_replacements = {
    "{BRANCH}": "main",
    "{BRANCH_DOC}" : "latest", # Used to target the correct ReadTheDocs distribution version
    "{RTD_TAG}": rtd_tag
}

# -- Project information -----------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#project-information

project = u'Hyperledger Fabric Docs'
copyright = u'2017-2024, Hyperledger Foundation'
author = u'Hyperledger Foundation'
release = u'main'
version = u'main'

# -- General configuration ---------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#general-configuration

# If extensions (or modules to document with autodoc) are in another directory,
# add these directories to sys.path here. If the directory is relative to the
# documentation root, use os.path.abspath to make it absolute, like shown here.
#

# recommonmark is a python utility that allows markdown to be used within
# Sphinx projects.
# Installed version as per directive in docs/requirement.txt
source_parsers = {
    '.md': 'recommonmark.parser.CommonMarkParser',
}

# The file extensions of source files. Sphinx considers the files with this suffix as sources. 
# The value can be a dictionary mapping file extensions to file types. For example:
source_suffix = {
    '.rst': 'restructuredtext',
    '.md': 'markdown'
}

# Add any paths that contain templates here, relative to this directory.
templates_path = ['_templates']

# List of patterns, relative to source directory, that match files and
# directories to ignore when looking for source files.
# This patterns also effect to html_static_path and html_extra_path
exclude_patterns = ['_build', 'Thumbs.db', '.DS_Store']

# Used to be "master_doc"
# The main toctree document
root_doc = 'index'

# The name of the Pygments (syntax highlighting) style to use.
pygments_style = 'sphinx'

# If true, `todo` and `todoList` produce output, else they produce nothing.
todo_include_todos = True

extensions = ['sphinx.ext.autodoc',
    'sphinx.ext.doctest',
    'sphinx.ext.intersphinx',
    'sphinx.ext.todo',
    'sphinx.ext.imgmath',
    'sphinx.ext.ifconfig',
    'sphinx.ext.viewcode',
    'myst_parser',
    'sphinxcontrib.jquery']

# -- Special API Accesses -------------------------------------------------
# They create an instance of the Sphinx object, documented here
# https://www.sphinx-doc.org/en/master/extdev/appapi.html#sphinx.application.Sphinx
# and pass it to us as "app" in this setup function.
#
# We then call it to perform special/specific customizations.

def placeholderReplace(app, docname, source):
    result = source[0]
    for key in app.config.placeholder_replacements:
        result = result.replace(key, app.config.placeholder_replacements[key])
    source[0] = result

def setup(app):
    app.add_css_file('css/custom.css')
    app.add_config_value('placeholder_replacements', {}, True)
    app.connect('source-read', placeholderReplace)


# -- Options for HTML output -------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#options-for-html-output

html_theme = 'sphinx_rtd_theme'

# html_css_files = ['css/custom.css']

html_theme_path = [sphinx_rtd_theme.get_html_theme_path()]

html_static_path = ['_static']

html_add_permalinks = True

# -- MyST-specific Options -------------------------------------------------
# https://myst-parser.readthedocs.io/en/latest/configuration.html#sphinx-config-options
myst_all_links_external = True