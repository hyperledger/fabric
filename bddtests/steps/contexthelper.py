# Copyright IBM Corp. 2017 All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

import uuid
import os
import shutil
from slugify import slugify


class Context(object):
    def __init__(self):
        pass

    def __setattr__(self, attr, value):
        self.__dict__[attr] = value

    def __getattr__(self, attr):
        return self.__dict__[attr]
        # raise AttributeError(e)

    def __getstate__(self):
        return self.__dict__

    def __setstate__(self, value):
        return self.__dict__.update(value)

    def __contains__(self, attr):
        return attr in self.__dict__

class ContextHelper:

    @classmethod
    def GetHelper(cls, context):
        if not "contextHelper" in context:
            context.contextHelper = ContextHelper(context)
        return context.contextHelper

    def __init__(self, context):
        self.context = context
        self.guuid = str(uuid.uuid1()).replace('-','')

    def getBootrapHelper(self, chainId):
        import bootstrap_util
        return bootstrap_util.BootstrapHelper(chainId=chainId)

    def getGuuid(self):
        return self.guuid

    def getTmpPath(self):
        pathToReturn = "tmp"
        if not os.path.isdir(pathToReturn):
            os.makedirs(pathToReturn)
        return pathToReturn

    def getCachePath(self):
        pathToReturn = os.path.join(self.getTmpPath(), "cache")
        if not os.path.isdir(pathToReturn):
            os.makedirs(pathToReturn)
        return pathToReturn


    def getTmpProjectPath(self):
        pathToReturn = os.path.join(self.getTmpPath(), self.guuid)
        if not os.path.isdir(pathToReturn):
            os.makedirs(pathToReturn)
        return pathToReturn

    def getTmpPathForName(self, name, extension=None, copyFromCache=False, path_relative_to_tmp=''):
        'Returns the tmp path for a file, and a flag indicating if the file exists. Will also check in the cache and copy to tmp if copyFromCache==True'
        unicodeName = unicode(name)
        dir_path = os.path.join(self.getTmpProjectPath(), path_relative_to_tmp)
        if not os.path.isdir(dir_path):
            os.makedirs(dir_path)
        slugifiedName = ".".join([slugify(unicodeName), extension]) if extension else slugify(unicodeName)
        tmpPath = os.path.join(dir_path, slugifiedName)
        fileExists = False
        if os.path.isfile(tmpPath):
            # file already exists in tmp path, return path and exists flag
            fileExists = True
        elif copyFromCache:
            # See if the file exists in cache, and copy over to project folder.
            cacheFilePath = os.path.join(self.getCachePath(), slugifiedName)
            if os.path.isfile(cacheFilePath):
                shutil.copy(cacheFilePath, tmpPath)
                fileExists = True
        return (tmpPath, fileExists)

    def copyToCache(self, name):
        srcPath, fileExists = self.getTmpPathForName(name, copyFromCache=False)
        assert fileExists, "Tried to copy source file to cache, but file not found for: {0}".format(srcPath)
        # Now copy to the cache if it does not already exist
        cacheFilePath = os.path.join(self.getCachePath(), slugify(name))
        if not os.path.isfile(cacheFilePath):
            shutil.copy(srcPath, cacheFilePath)


    def isConfigEnabled(self, configName):
        return self.context.config.userdata.get(configName, "false") == "true"

    def before_scenario(self, scenario):
        # print("before_scenario: {0}".format(self))
        pass

    def after_scenario(self, scenario):
        # print("after_scenario: {0}".format(self))
        pass

    def before_step(self, step):
        # print("before_step: {0}".format(self))
        pass

    def after_step(self, step):
        # print("after_step: {0}".format(self))
        pass

    def registerComposition(self, composition):
        return composition

