package godog

import (
	"flag"
	"fmt"
	"strings"
)

var descFeaturesArgument = "Optional feature(s) to run. Can be:\n" +
	s(4) + "- dir " + cl("(features/)", yellow) + "\n" +
	s(4) + "- feature " + cl("(*.feature)", yellow) + "\n" +
	s(4) + "- scenario at specific line " + cl("(*.feature:10)", yellow) + "\n" +
	"If no feature paths are listed, suite tries " + cl("features", yellow) + " path by default.\n"

var descConcurrencyOption = "Run the test suite with concurrency level:\n" +
	s(4) + "- " + cl(`= 1`, yellow) + ": supports all types of formats.\n" +
	s(4) + "- " + cl(`>= 2`, yellow) + ": only supports " + cl("progress", yellow) + ". Note, that\n" +
	s(4) + "your context needs to support parallel execution."

var descTagsOption = "Filter scenarios by tags. Expression can be:\n" +
	s(4) + "- " + cl(`"@wip"`, yellow) + ": run all scenarios with wip tag\n" +
	s(4) + "- " + cl(`"~@wip"`, yellow) + ": exclude all scenarios with wip tag\n" +
	s(4) + "- " + cl(`"@wip && ~@new"`, yellow) + ": run wip scenarios, but exclude new\n" +
	s(4) + "- " + cl(`"@wip,@undone"`, yellow) + ": run wip or undone scenarios"

// FlagSet allows to manage flags by external suite runner
func FlagSet(format, tags *string, defs, sof, noclr *bool, cr *int) *flag.FlagSet {
	descFormatOption := "How to format tests output. Available formats:\n"
	for _, f := range formatters {
		descFormatOption += s(4) + "- " + cl(f.name, yellow) + ": " + f.description + "\n"
	}
	descFormatOption = strings.TrimSpace(descFormatOption)

	set := flag.NewFlagSet("godog", flag.ExitOnError)
	set.StringVar(format, "format", "pretty", descFormatOption)
	set.StringVar(format, "f", "pretty", descFormatOption)
	set.StringVar(tags, "tags", "", descTagsOption)
	set.StringVar(tags, "t", "", descTagsOption)
	set.IntVar(cr, "concurrency", 1, descConcurrencyOption)
	set.IntVar(cr, "c", 1, descConcurrencyOption)
	set.BoolVar(defs, "definitions", false, "Print all available step definitions.")
	set.BoolVar(defs, "d", false, "Print all available step definitions.")
	set.BoolVar(sof, "stop-on-failure", false, "Stop processing on first failed scenario.")
	set.BoolVar(noclr, "no-colors", false, "Disable ansi colors.")
	set.Usage = usage(set)
	return set
}

type flagged struct {
	short, long, descr, dflt string
}

func (f *flagged) name() string {
	var name string
	switch {
	case len(f.short) > 0 && len(f.long) > 0:
		name = fmt.Sprintf("-%s, --%s", f.short, f.long)
	case len(f.long) > 0:
		name = fmt.Sprintf("--%s", f.long)
	case len(f.short) > 0:
		name = fmt.Sprintf("-%s", f.short)
	}
	if f.dflt != "true" && f.dflt != "false" {
		name += "=" + f.dflt
	}
	return name
}

func usage(set *flag.FlagSet) func() {
	return func() {
		var list []*flagged
		var longest int
		set.VisitAll(func(f *flag.Flag) {
			var fl *flagged
			for _, flg := range list {
				if flg.descr == f.Usage {
					fl = flg
					break
				}
			}
			if nil == fl {
				fl = &flagged{
					dflt:  f.DefValue,
					descr: f.Usage,
				}
				list = append(list, fl)
			}
			if len(f.Name) > 2 {
				fl.long = f.Name
			} else {
				fl.short = f.Name
			}
		})

		for _, f := range list {
			if len(f.name()) > longest {
				longest = len(f.name())
			}
		}

		// prints an option or argument with a description, or only description
		opt := func(name, desc string) string {
			var ret []string
			lines := strings.Split(desc, "\n")
			ret = append(ret, s(2)+cl(name, green)+s(longest+2-len(name))+lines[0])
			if len(lines) > 1 {
				for _, ln := range lines[1:] {
					ret = append(ret, s(2)+s(longest+2)+ln)
				}
			}
			return strings.Join(ret, "\n")
		}

		// --- GENERAL ---
		fmt.Println(cl("Usage:", yellow))
		fmt.Printf(s(2) + "godog [options] [<features>]\n\n")
		// description
		fmt.Println("Builds a test package and runs given feature files.")
		fmt.Printf("Command should be run from the directory of tested package and contain buildable go source.\n\n")

		// --- ARGUMENTS ---
		fmt.Println(cl("Arguments:", yellow))
		// --> features
		fmt.Println(opt("features", descFeaturesArgument))

		// --- OPTIONS ---
		fmt.Println(cl("Options:", yellow))
		for _, f := range list {
			fmt.Println(opt(f.name(), f.descr))
		}
		fmt.Println("")
	}
}
