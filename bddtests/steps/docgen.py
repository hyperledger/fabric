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

from StringIO import StringIO
from itertools import chain
from google.protobuf.message import Message

from b3j0f.aop import weave, unweave, is_intercepted, weave_on

from jinja2 import Environment, PackageLoader, select_autoescape, FileSystemLoader, Template
env = Environment(
    loader=FileSystemLoader(searchpath="templates"),
    autoescape=select_autoescape(['html', 'xml']),
    trim_blocks=True,
    lstrip_blocks=True
)

from bootstrap_util import getDirectory

class DocumentGenerator:


    def __init__(self, contextHelper, scenario):
        self.contextHelper = contextHelper
        self.directory = getDirectory(contextHelper.context)
        self.output = StringIO()
        self.currentStep = 0
        self.composition = None

        #Weave advices into contextHelper
        weave(target=self.contextHelper.before_step, advices=self.beforeStepAdvice)
        weave(target=self.contextHelper.after_step, advices=self.afterStepAdvice)
        weave(target=self.contextHelper.after_scenario, advices=self.afterScenarioAdvice)
        weave(target=self.contextHelper.getBootrapHelper, advices=self.getBootstrapHelperAdvice)
        weave(target=self.contextHelper.registerComposition, advices=self.registerCompositionAdvice)

        # Weave advices into Directory
        weave(target=self.directory._registerOrg, advices=self.registerOrgAdvice)
        weave(target=self.directory._registerUser, advices=self.registerUserAdvice)
        weave(target=self.directory.registerOrdererAdminTuple, advices=self.registerNamedNodeAdminTupleAdvice)

    def beforeStepAdvice(self, joinpoint):
        self.currentStep += 1
        step = joinpoint.kwargs['step']
        # Now the jinja template
        self.output.write(env.get_template("html/step.html").render(step_id="Step {0}".format(self.currentStep), step=step))
        return joinpoint.proceed()

    def afterStepAdvice(self, joinpoint):
        step = joinpoint.kwargs['step']
        # Now the jinja template
        if step.status=="failed":
            self.output.write(env.get_template("html/error.html").render(err=step.error_message))
        return joinpoint.proceed()


    def compositionCallCLIAdvice(self, joinpoint):
        'This advice is called around the compositions usage of the cli'
        result = joinpoint.proceed()
        # Create table for environment
        composition = joinpoint.kwargs['self']
        envAdditions = composition.getEnvAdditions()
        keys = envAdditions.keys()
        keys.sort()
        envPreamble = " ".join(["{0}={1}".format(key,envAdditions[key]) for key in keys])
        args= " ".join(joinpoint.kwargs['argList'])
        self.output.write(env.get_template("html/cli.html").render(command="{0} {1}".format(envPreamble, args)))
        return result

    def _getNetworkGroup(self, serviceName):
        groups = {"peer" : 1, "orderer" : 2, "kafka" : 7, "zookeeper" : 8, "couchdb" : 9}
        groupId = 0
        for group, id in groups.iteritems():
            if serviceName.lower().startswith(group):
                groupId = id
        return groupId

    def _getNetworkForConfig(self, configAsYaml):
        import yaml
        config = yaml.load(configAsYaml)
        assert "services" in config, "Expected config from docker-compose config to have services key at top level:  \n{0}".format(config)
        network = {"nodes": [], "links" : []}
        for serviceName in config['services'].keys():
            network['nodes'].append({"id" : serviceName, "group" : self._getNetworkGroup(serviceName), "type" : "node"})
            # Now get links
            if "depends_on" in config['services'][serviceName]:
                for dependedOnServiceName in config['services'][serviceName]['depends_on']:
                    network['links'].append({"source": serviceName, "target": dependedOnServiceName, "value" : 1})
        return network

    def _getNetworkForDirectory(self):
        network = {"nodes":[], "links": []}
        for orgName, org in self.directory.getOrganizations().iteritems():
            network['nodes'].append({"id" : orgName, "group" : 3, "type" : "org"})
        for userName, user in self.directory.getUsers().iteritems():
            network['nodes'].append({"id" : userName, "group" : 4, "type" : "user"})
        # Now get links
        for nct, cert in self.directory.getNamedCtxTuples().iteritems():
            nctId = "{0}-{1}-{2}".format(nct.user, nct.nodeName, nct.organization)
            network['nodes'].append({"id" : nctId, "group" : 5, "type" : "cert"})
            network['links'].append({"source": nctId, "target": nct.organization, "value" : 1})
            network['links'].append({"source": nctId, "target": nct.user, "value" : 1})
            # Only add the context link if it is a compose service, else the target may not exist.
            if nct.nodeName in self.composition.getServiceNames():
                network['links'].append({"source": nctId, "target": nct.nodeName, "value" : 1})
        return network

    def _writeNetworkJson(self):
        if self.composition:
            import json
            configNetwork = self._getNetworkForConfig(configAsYaml=self.composition.getConfig())
            directoryNetwork = self._getNetworkForDirectory()
            # Join the network info together
            fullNetwork = dict(chain([(key, configNetwork[key] + directoryNetwork[key]) for key in configNetwork.keys()]))
            (fileName, fileExists) = self.contextHelper.getTmpPathForName("network", extension="json")
            with open(fileName, "w") as f:
                f.write(json.dumps(fullNetwork))


    def registerCompositionAdvice(self, joinpoint):
        composition = joinpoint.kwargs['composition']
        weave(target=composition._callCLI, advices=self.compositionCallCLIAdvice)
        result = joinpoint.proceed()
        if composition:
            #Now get the config for the composition and dump out.
            self.composition = composition
            configAsYaml = composition.getConfig()
            (dokerComposeYmlFileName, fileExists) = self.contextHelper.getTmpPathForName(name="docker-compose", extension="yml")
            with open(dokerComposeYmlFileName, 'w') as f:
                f.write(configAsYaml)
            self.output.write(env.get_template("html/composition-py.html").render(compose_project_name= self.composition.projectName,docker_compose_yml_file=dokerComposeYmlFileName))
            self.output.write(env.get_template("html/header.html").render(text="Configuration", level=4))
            self.output.write(env.get_template("html/cli.html").render(command=configAsYaml))
            #Inject the graph
            self.output.write(env.get_template("html/header.html").render(text="Network Graph", level=4))
            self.output.write(env.get_template("html/graph.html").render())
        return result

    def _addLinkToFile(self, fileName ,linkText):
        import ntpath
        baseName = ntpath.basename(fileName)
        # self.markdownWriter.addLink(linkUrl="./{0}".format(baseName), linkText=linkText, linkTitle=baseName)

    def _getLinkInfoForFile(self, fileName):
        import ntpath
        return "./{0}".format(ntpath.basename(fileName))

    def registerOrgAdvice(self, joinpoint):
        orgName = joinpoint.kwargs['orgName']
        newlyRegisteredOrg = joinpoint.proceed()
        orgCert = newlyRegisteredOrg.getCertAsPEM()
        #Write out key material
        (fileName, fileExists) = self.contextHelper.getTmpPathForName(name="dir-org-{0}-cert".format(orgName), extension="pem")
        with open(fileName, 'w') as f:
            f.write(orgCert)
        self._addLinkToFile(fileName=fileName, linkText="Public cert for Organization")
        #Now the jinja output
        self.output.write(env.get_template("html/org.html").render(org=newlyRegisteredOrg, cert_href=self._getLinkInfoForFile(fileName), path_to_cert=fileName))
        return newlyRegisteredOrg

    def registerUserAdvice(self, joinpoint):
        userName = joinpoint.kwargs['userName']
        newlyRegisteredUser = joinpoint.proceed()
        #Write out key material
        privateKeyAsPem = newlyRegisteredUser.getPrivateKeyAsPEM()
        (fileName, fileExists) = self.contextHelper.getTmpPathForName(name="dir-user-{0}-privatekey".format(userName), extension="pem")
        with open(fileName, 'w') as f:
            f.write(privateKeyAsPem)
        #Weave into user tags setting
        weave(target=newlyRegisteredUser.setTagValue, advices=self.userSetTagValueAdvice)
        #Now the jinja output
        self.output.write(env.get_template("html/user.html").render(user=newlyRegisteredUser, private_key_href=self._getLinkInfoForFile(fileName)))
        return newlyRegisteredUser

    def _dump_context(self):
        (dirPickleFileName, fileExists) = self.contextHelper.getTmpPathForName("dir", extension="pickle")
        with open(dirPickleFileName, 'w') as f:
            self.directory.dump(f)
        #Now the jinja output
        self.output.write(env.get_template("html/directory.html").render(directory=self.directory, path_to_pickle=dirPickleFileName))
        if self.composition:
            (dokerComposeYmlFileName, fileExists) = self.contextHelper.getTmpPathForName(name="docker-compose", extension="yml")
            self.output.write(env.get_template("html/appendix-py.html").render(directory=self.directory,
                                                                               path_to_pickle=dirPickleFileName,
                                                                               compose_project_name=self.composition.projectName,
                                                                               docker_compose_yml_file=dokerComposeYmlFileName))


    def afterScenarioAdvice(self, joinpoint):
        scenario = joinpoint.kwargs['scenario']
        self._dump_context()
        #Render with jinja
        header = env.get_template("html/scenario.html").render(scenario=scenario, steps=scenario.steps)
        main = env.get_template("html/main.html").render(header=header, body=self.output.getvalue())
        (fileName, fileExists) = self.contextHelper.getTmpPathForName("scenario", extension="html")
        with open(fileName, 'w') as f:
            f.write(main.encode("utf-8"))
        self._writeNetworkJson()
        return joinpoint.proceed()

    def registerNamedNodeAdminTupleAdvice(self, joinpoint):
        namedNodeAdminTuple = joinpoint.proceed()
        directory = joinpoint.kwargs['self']
        #jinja
        newCertAsPEM = directory.getCertAsPEM(namedNodeAdminTuple)
        self.output.write(env.get_template("html/header.html").render(text="Created new named node admin tuple: {0}".format(namedNodeAdminTuple), level=4))
        self.output.write(env.get_template("html/cli.html").render(command=newCertAsPEM))
        #Write cert out
        fileNameTocheck = "dir-user-{0}-cert-{1}-{2}".format(namedNodeAdminTuple.user, namedNodeAdminTuple.nodeName, namedNodeAdminTuple.organization)
        (fileName, fileExists) = self.contextHelper.getTmpPathForName(fileNameTocheck, extension="pem")
        with open(fileName, 'w') as f:
            f.write(newCertAsPEM)
        return namedNodeAdminTuple

    def bootstrapHelperSignConfigItemAdvice(self, joinpoint):
        configItem = joinpoint.kwargs['configItem']
        #jinja
        self.output.write(env.get_template("html/header.html").render(text="Dumping signed config item...", level=4))
        self.output.write(env.get_template("html/protobuf.html").render(msg=configItem, msgLength=len(str(configItem))))

        signedConfigItem = joinpoint.proceed()
        return signedConfigItem

    def getBootstrapHelperAdvice(self, joinpoint):
        bootstrapHelper = joinpoint.proceed()
        weave(target=bootstrapHelper.signConfigItem, advices=self.bootstrapHelperSignConfigItemAdvice)
        return bootstrapHelper

    def _isProtobufMessage(self, target):
        return isinstance(target, Message)

    def _isListOfProtobufMessages(self, target):
        result = False
        if isinstance(target, list):
            messageList = [item for item in target if self._isProtobufMessage(item)]
            result = len(messageList) == len(target)
        return result

    def _isDictOfProtobufMessages(self, target):
        result = False
        if isinstance(target, dict):
            messageList = [item for item in target.values() if self._isProtobufMessage(item)]
            result = len(messageList) == len(target)
        return result

    def _writeProtobuf(self, fileName, msg):
        import ntpath
        baseName = ntpath.basename(fileName)
        dataToWrite = msg.SerializeToString()
        with open("{0}".format(fileName), 'wb') as f:
            f.write(dataToWrite)
        self.output.write(env.get_template("html/protobuf.html").render(id=baseName, msg=msg, path_to_protobuf=fileName, msgLength=len(dataToWrite),linkUrl="./{0}".format(baseName), linkText="Protobuf message in binary form", linkTitle=baseName))


    def userSetTagValueAdvice(self, joinpoint):
        result = joinpoint.proceed()
        user = joinpoint.kwargs['self']
        tagKey = joinpoint.kwargs['tagKey']
        tagValue = joinpoint.kwargs['tagValue']

        #jinja invoke
        self.output.write(env.get_template("html/tag.html").render(user=user, tag_key=tagKey))

        # If protobuf message, write out in binary form
        if self._isProtobufMessage(tagValue):
            import ntpath
            (fileName, fileExists) = self.contextHelper.getTmpPathForName("{0}-{1}".format(user.getUserName(), tagKey), extension="protobuf")
            self._writeProtobuf(fileName=fileName, msg=tagValue)
        # If protobuf message, write out in binary form
        elif self._isListOfProtobufMessages(tagValue):
            index = 0
            for msg in tagValue:
                (fileName, fileExists) = self.contextHelper.getTmpPathForName("{0}-{1}-{2:0>4}".format(user.getUserName(), tagKey, index), extension="protobuf")
                self._writeProtobuf(fileName=fileName, msg=msg)
                index += 1
        elif self._isDictOfProtobufMessages(tagValue):
            for key,msg in tagValue.iteritems():
                (fileName, fileExists) = self.contextHelper.getTmpPathForName("{0}-{1}-{2}".format(user.getUserName(), tagKey, key), extension="protobuf")
                self._writeProtobuf(fileName=fileName, msg=msg)
        else:
            self.output.write(env.get_template("html/cli.html").render(command=str(tagValue)))
        return result