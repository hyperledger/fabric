import os
import shutil

from test_util import cli_call

@given(u'I create a dir "{dirPath}"')
def step_impl(context, dirPath):
    os.makedirs(dirPath, 0755);

@then(u'I should delete dir "{dirPath}"')
def step_impl(contxt, dirPath):
    shutil.rmtree(dirPath)

@when(u'I execute utility with no flag')
def step_impl(context):
    cmd = ["./dump_db_stats"]
    context.output, context.error, context.returncode = cli_call(cmd, expect_success=False)

@when(u'I execute utility with flag "{flag}" and path "{path}"')
def step_impl(context, flag, path):
    cmd = ["./dump_db_stats"]
    cmd.append(flag)
    cmd.append(path)
    context.output, context.error, context.returncode = cli_call(cmd, expect_success=False)

@then(u'I should get a process exit code "{expectedReturncode}"')
def step_impl(context, expectedReturncode):
    assert (str(context.returncode) == expectedReturncode), "Return code: expected (%s), instead found (%s)" % (expectedReturncode, context.returncode)
