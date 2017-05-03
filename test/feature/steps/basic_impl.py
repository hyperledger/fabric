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

from behave import *
import time

@given(u'I wait "{seconds}" seconds')
@when(u'I wait "{seconds}" seconds')
@then(u'I wait "{seconds}" seconds')
def step_impl(context, seconds):
    time.sleep(float(seconds))

@given(u'I have a bootstrapped fabric network of')
def step_impl(context):
    bootstrapped_impl(context, "solo")

@given(u'I have a bootstrapped fabric network of type {type}')
def bootstrapped_impl(context, type):
    pass
