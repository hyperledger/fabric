import test_util
import os

def before_feature(context, feature):
    print("\nRunning go build")
    cmd = ["go", "build", "../dump_db_stats.go"]
    test_util.cli_call(context, cmd, expect_success=True)
    print("go build complete")

def after_feature(context, feature):
    print("Deleting utility binary")
    os.remove("./dump_db_stats")
