import cStringIO
import os
import glob
import errno
from collections import OrderedDict

import bdd_test_util


def testCoverage():
	#First save the coverage files
	saveCoverageFiles("coverage","scenario_1", ["bddtests_vp0_1","bddtests_vp1_1","bddtests_vp2_1","bddtests_vp3_1",], "cov")

    # Now collect the filenames for coverage files.
	files = glob.glob(os.path.join('coverage','*.cov'))

    #Create the aggregate coverage file
	coverageContents = createCoverageFile(files)

    #Ouput the aggregate coverage file
	with open('coverage.total', 'w') as outfile:
		outfile.write(coverageContents)
		outfile.close()

def createCoverageAggregate():
    # Now collect the filenames for coverage files.
    files = glob.glob(os.path.join('coverage','*.cov'))

    #Create the aggregate coverage file
    coverageContents = createCoverageFile(files)

    #Ouput the aggregate coverage file
    with open('coverage-behave.cov', 'w') as outfile:
        outfile.write(coverageContents)
        outfile.close()


def saveCoverageFiles(folderName, rootName, containerNames, extension):
    'Will save the converage files to folderName'
    # Make the directory
    try:
    	os.makedirs(folderName)
    except OSError as exception:
    	if exception.errno != errno.EEXIST:
    		raise
    for containerName in containerNames:
        srcPath = "{0}:/opt/gopath/src/github.com/hyperledger/fabric/coverage.cov".format(containerName)
        print("sourcepath = {0}".format(srcPath))
        destPath = os.path.join(folderName, "{0}-{1}.{2}".format(rootName, containerName, extension))
        output, error, returncode = \
            bdd_test_util.cli_call(["docker", "cp", srcPath, destPath], expect_success=False)

def testCreateSystemCoverageFile(folderName, rootName, containerNames, extension):
    'Will create a single aggregate coverage file fromsave the converage files to folderName'
    files = glob.glob(os.path.join('coverage','*.cov'))
    for containerName in containerNames:
        srcPath = "{0}:/opt/gopath/src/github.com/hyperledger/fabric/peer/coverage.cov".format(containerName)
        destPath = os.path.join(folderName, "{0}-{1}.{2}".format(rootName, containerName, extension))
        output, error, returncode = \
            bdd_test_util.cli_call(["docker", "cp", srcPath, destPath], expect_success=False)


def createCoverageFile(filenames):
    """Creates an aggregated coverage file"""
    output = cStringIO.StringIO()
    output.write('mode: count\n')
    linesMap = {}
    #with open('coverage.total', 'w') as outfile:
    for fname in filenames:
        with open(fname) as infile:
        	firstLine = True
        	for line in infile:
        		if firstLine:
        			firstLine = False
        			continue
        		else:
        		    # Split the line based upon white space
        		    lineParts = line.split()
            		if lineParts[0] in linesMap:
            			# Found, keep the greater
            			newCount = long(lineParts[2])
            			oldCount = long(linesMap[lineParts[0]].split()[2])
            			if newCount > oldCount:
            				linesMap[lineParts[0]] = line
            		else:
            			linesMap[lineParts[0]] = line
    # Now sort the output
    od = OrderedDict(sorted(linesMap.items(), key=lambda i: i[1]))

    for (key, line) in od.items():
    	output.write(line)
    contents = output.getvalue()
    output.close()
    return contents
