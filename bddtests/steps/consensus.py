#
# Copyright IBM Corp. 2016 All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

import os, os.path
import re
import time
import copy
from behave import *
from datetime import datetime, timedelta
import base64

import json

import bdd_compose_util, bdd_test_util, bdd_request_util
#from bdd_json_util import getAttributeFromJSON
from bdd_test_util import bdd_log

import sys, yaml
import subprocess


@given(u'I stop the chaincode')
def step_impl(context):
    try:
        # Kill chaincode containers
        res, error, returncode = bdd_test_util.cli_call(
                                   ["docker", "ps", "-n=4", "-q"],
                                   expect_success=True)
        bdd_log("Killing chaincode containers: {0}".format(res))
        result, error, returncode = bdd_test_util.cli_call(
                                   ["docker", "rm", "-f"] + res.split('\n'),
                                   expect_success=False)
        bdd_log("Stopped chaincode containers")

    except:
        raise Exception("Unable to kill chaincode images")

@given(u'I remove the chaincode images')
def step_impl(context):
    try:
        # Kill chaincode images
        res, error, returncode = bdd_test_util.cli_call(
                                   ["docker", "images"],
                                   expect_success=True)
        images = res.split('\n')
        for image in images:
            if image.startswith('dev-vp'):
                fields = image.split()
                r, e, ret= bdd_test_util.cli_call(
                                   ["docker", "rmi", "-f", fields[2]],
                                   expect_success=False)
        bdd_log("Removed chaincode images")

    except:
        raise Exception("Unable to kill chaincode images")

