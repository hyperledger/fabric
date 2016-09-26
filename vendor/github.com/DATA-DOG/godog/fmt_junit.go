package godog

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/DATA-DOG/godog/gherkin"
)

func init() {
	Format("junit", "Prints junit compatible xml to stdout", &junitFormatter{
		suite: &junitPackageSuite{
			Name:       "main", // @TODO: it should extract package name
			TestSuites: make([]*junitTestSuite, 0),
		},
		started: time.Now(),
	})
}

type junitFormatter struct {
	suite *junitPackageSuite

	// timing
	started     time.Time
	caseStarted time.Time
	featStarted time.Time

	outline        *gherkin.ScenarioOutline
	outlineExample int
}

func (j *junitFormatter) Feature(feature *gherkin.Feature, path string) {
	testSuite := &junitTestSuite{
		TestCases: make([]*junitTestCase, 0),
		Name:      feature.Name,
	}

	if len(j.suite.TestSuites) > 0 {
		j.current().Time = time.Since(j.featStarted).String()
	}
	j.featStarted = time.Now()
	j.suite.TestSuites = append(j.suite.TestSuites, testSuite)
}

func (j *junitFormatter) Node(node interface{}) {
	suite := j.current()
	tcase := &junitTestCase{}

	switch t := node.(type) {
	case *gherkin.ScenarioOutline:
		j.outline = t
		return
	case *gherkin.Scenario:
		tcase.Name = t.Name
		suite.Tests++
		j.suite.Tests++
	case *gherkin.Examples:
		j.outlineExample = 0
		return
	case *gherkin.TableRow:
		j.outlineExample++
		tcase.Name = fmt.Sprintf("%s #%d", j.outline.Name, j.outlineExample)
		suite.Tests++
		j.suite.Tests++
	default:
		return
	}
	if len(suite.TestCases) > 0 {
		suite.current().Time = time.Since(j.caseStarted).String()
	}
	j.caseStarted = time.Now()
	suite.TestCases = append(suite.TestCases, tcase)
}

func (j *junitFormatter) Failed(step *gherkin.Step, match *StepDef, err error) {
	suite := j.current()
	suite.Failures++
	j.suite.Failures++

	tcase := suite.current()
	tcase.Status = "failed"
	tcase.Failure = &junitFailure{
		Message: fmt.Sprintf("%s %s: %s", step.Type, step.Text, err.Error()),
	}
}

func (j *junitFormatter) Passed(step *gherkin.Step, match *StepDef) {
	suite := j.current()

	tcase := suite.current()
	tcase.Status = "passed"
}

func (j *junitFormatter) Skipped(step *gherkin.Step) {
}

func (j *junitFormatter) Undefined(step *gherkin.Step) {
	suite := j.current()
	suite.Errors++
	j.suite.Errors++

	tcase := suite.current()
	tcase.Status = "undefined"
	tcase.Error = append(tcase.Error, &junitError{
		Type:    "undefined",
		Message: fmt.Sprintf("%s %s", step.Type, step.Text),
	})
}

func (j *junitFormatter) Pending(step *gherkin.Step, match *StepDef) {
	suite := j.current()
	suite.Errors++
	j.suite.Errors++

	tcase := suite.current()
	tcase.Status = "pending"
	tcase.Error = append(tcase.Error, &junitError{
		Type:    "pending",
		Message: fmt.Sprintf("%s %s: TODO: write pending definition", step.Type, step.Text),
	})
}

func (j *junitFormatter) Summary() {
	j.suite.Time = time.Since(j.started).String()
	io.WriteString(os.Stdout, xml.Header)

	enc := xml.NewEncoder(os.Stdout)
	enc.Indent("", s(2))
	if err := enc.Encode(j.suite); err != nil {
		fmt.Println("failed to write junit xml:", err)
	}
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr,omitempty"`
}

type junitError struct {
	XMLName xml.Name `xml:"error,omitempty"`
	Message string   `xml:"message,attr"`
	Type    string   `xml:"type,attr"`
}

type junitTestCase struct {
	XMLName xml.Name      `xml:"testcase"`
	Name    string        `xml:"name,attr"`
	Status  string        `xml:"status,attr"`
	Time    string        `xml:"time,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
	Error   []*junitError
}

type junitTestSuite struct {
	XMLName   xml.Name `xml:"testsuite"`
	Name      string   `xml:"name,attr"`
	Tests     int      `xml:"tests,attr"`
	Skipped   int      `xml:"skipped,attr"`
	Failures  int      `xml:"failures,attr"`
	Errors    int      `xml:"errors,attr"`
	Time      string   `xml:"time,attr"`
	TestCases []*junitTestCase
}

func (ts *junitTestSuite) current() *junitTestCase {
	return ts.TestCases[len(ts.TestCases)-1]
}

type junitPackageSuite struct {
	XMLName    xml.Name `xml:"testsuites"`
	Name       string   `xml:"name,attr"`
	Tests      int      `xml:"tests,attr"`
	Skipped    int      `xml:"skipped,attr"`
	Failures   int      `xml:"failures,attr"`
	Errors     int      `xml:"errors,attr"`
	Time       string   `xml:"time,attr"`
	TestSuites []*junitTestSuite
}

func (j *junitFormatter) current() *junitTestSuite {
	return j.suite.TestSuites[len(j.suite.TestSuites)-1]
}
