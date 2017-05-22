# Copyright IBM Corp. 2017 All Rights Reserved.
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
import json


@when(u'a user deploys chaincode at path {path} with {args} with name {name}')
def deploy_impl(context, path, args, name):
    pass

@when(u'a user deploys chaincode at path {path} with {args}')
def step_impl(context, path, args):
    deploy_impl(context, path, json.loads(args), "mycc")

@when(u'a user deploys chaincode')
def step_impl(context):
    deploy_impl(context,
                "github.com/hyperledger/fabric//chaincode_example02",
                ["init", "a", "100" , "b", "200"],
                "mycc")

@when(u'a user queries on the chaincode named {name} with args {args} on {component}')
def query_impl(context, name, args, component):
    pass

@when(u'a user queries on the chaincode named {name} with args {args}')
def step_impl(context, name, args):
    query_impl(context, name, json.loads(args), "peer0")

@when(u'a user queries on the chaincode named {name}')
def step_impl(context, name):
    query_impl(context, name, ["query", "a"], "peer0")

@when(u'a user queries on the chaincode')
def step_impl(context):
    query_impl(context, "mycc", ["query", "a"], "peer0")

@when(u'a user invokes {count} times on the chaincode named {name} with args {args}')
def invokes_impl(context, count, name, args):
    pass

@when(u'a user invokes on the chaincode named {name} with args {args}')
def step_impl(context, name, args):
    invokes_impl(context, 1, name, json.loads(args))

@when(u'a user invokes {count} times on the chaincode')
def step_impl(context, count):
    invokes_impl(context, count, "mycc", ["txId1", "invoke", "a", 5])

@when(u'a user invokes on the chaincode named {name}')
def step_impl(context, name):
    invokes_impl(context, 1, name, ["txId1", "invoke", "a", 5])

@when(u'a user invokes on the chaincode')
def step_impl(context):
    invokes_impl(context, 1, "mycc", ["txId1", "invoke", "a", 5])

@then(u'the chaincode is deployed')
def step_impl(context):
    pass

@then(u'a user receives expected response of {response}')
def expected_impl(context, response):
    pass

@then(u'a user receives expected response')
def step_impl(context):
    expected_impl(context, 1000)
