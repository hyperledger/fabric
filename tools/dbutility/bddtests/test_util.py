import subprocess

def cli_call(arg_list, expect_success=True):
    p = subprocess.Popen(arg_list, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, error = p.communicate()
    if p.returncode != 0:
        if output is not None:
            print("Output:\n" + output)
        if error is not None:
            print("Error Message:\n" + error)
        if expect_success:
            raise subprocess.CalledProcessError(p.returncode, arg_list, output)
    return output, error, p.returncode
