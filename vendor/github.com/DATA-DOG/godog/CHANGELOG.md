# Change LOG

**2016-06-14** - **v0.5.0**
- godog now uses **go tool compile** and **go tool link** to support
  vendor directory dependencies. It also compiles test executable the same
  way as standard **go test** utility. With this change, only go
  versions from **1.5** are now supported.

**2016-06-01**
- parse flags in main command, to show version and help without needing
  to compile test package and buildable go sources.

**2016-05-28**
- show nicely formatted called step func name and file path

**2016-05-26**
- pack gherkin dependency in a subpackage to prevent compatibility
  conflicts in the future. If recently upgraded, probably you will need to
  reference gherkin as `github.com/DATA-DOG/godog/gherkin` instead.

**2016-05-25**
- refactored test suite build tooling in order to use standard **go test**
  tool. Which allows to compile package with godog runner script in **go**
  idiomatic way. It also supports all build environment options as usual.
- **godog.Run** now returns an **int** exit status. It was not returning
  anything before, so there is no compatibility breaks.

**2016-03-04**
- added **junit** compatible output formatter, which prints **xml**
  results to **os.Stdout**
- fixed #14 which skipped printing background steps when there was
  scenario outline in feature.

**2015-07-03**
- changed **godog.Suite** from interface to struct. Context registration should be updated accordingly. The reason
for change: since it exports the same methods and there is no need to mock a function in tests, there is no
obvious reason to keep an interface.
- in order to support running suite concurrently, needed to refactor an entry point of application. The **Run** method
now is a func of godog package which initializes and run the suite (or more suites). Method **New** is removed. This
change made godog a little cleaner.
- renamed **RegisterFormatter** func to **Format** to be more consistent.

