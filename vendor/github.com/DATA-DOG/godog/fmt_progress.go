package godog

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/DATA-DOG/godog/gherkin"
)

func init() {
	Format("progress", "Prints a character per step.", &progress{
		basefmt: basefmt{
			started: time.Now(),
			indent:  2,
		},
		stepsPerRow: 70,
	})
}

type progress struct {
	basefmt
	sync.Mutex
	stepsPerRow int
	steps       int
}

func (f *progress) Node(n interface{}) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Node(n)
}

func (f *progress) Feature(ft *gherkin.Feature, p string) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Feature(ft, p)
}

func (f *progress) Summary() {
	left := math.Mod(float64(f.steps), float64(f.stepsPerRow))
	if left != 0 {
		if int(f.steps) > f.stepsPerRow {
			fmt.Printf(s(f.stepsPerRow-int(left)) + fmt.Sprintf(" %d\n", f.steps))
		} else {
			fmt.Printf(" %d\n", f.steps)
		}
	}
	fmt.Println("")

	if len(f.failed) > 0 {
		fmt.Println("\n--- " + cl("Failed steps:", red) + "\n")
		for _, fail := range f.failed {
			fmt.Println(s(4) + cl(fail.step.Keyword+" "+fail.step.Text, red) + cl(" # "+fail.line(), black))
			fmt.Println(s(6) + cl("Error: ", red) + bcl(fail.err, red) + "\n")
		}
	}
	f.basefmt.Summary()
}

func (f *progress) step(res *stepResult) {
	switch res.typ {
	case passed:
		fmt.Print(cl(".", green))
	case skipped:
		fmt.Print(cl("-", cyan))
	case failed:
		fmt.Print(cl("F", red))
	case undefined:
		fmt.Print(cl("U", yellow))
	case pending:
		fmt.Print(cl("P", yellow))
	}
	f.steps++
	if math.Mod(float64(f.steps), float64(f.stepsPerRow)) == 0 {
		fmt.Printf(" %d\n", f.steps)
	}
}

func (f *progress) Passed(step *gherkin.Step, match *StepDef) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Passed(step, match)
	f.step(f.passed[len(f.passed)-1])
}

func (f *progress) Skipped(step *gherkin.Step) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Skipped(step)
	f.step(f.skipped[len(f.skipped)-1])
}

func (f *progress) Undefined(step *gherkin.Step) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Undefined(step)
	f.step(f.undefined[len(f.undefined)-1])
}

func (f *progress) Failed(step *gherkin.Step, match *StepDef, err error) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Failed(step, match, err)
	f.step(f.failed[len(f.failed)-1])
}

func (f *progress) Pending(step *gherkin.Step, match *StepDef) {
	f.Lock()
	defer f.Unlock()
	f.basefmt.Pending(step, match)
	f.step(f.pending[len(f.pending)-1])
}
