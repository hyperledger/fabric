package web

import (
	"log"
	"os"
)

// PanicReporter can receive panics that happen when serving a request and report them to a log of some sort.
type PanicReporter interface {
	// Panic is called with the URL of the request, the result of calling recover, and the stack.
	Panic(url string, err interface{}, stack string)
}

// PanicHandler will be logged to in panic conditions (eg, division by zero in an app handler).
// Applications can set web.PanicHandler = your own logger, if they wish.
// In terms of logging the requests / responses, see logger_middleware. That is a completely separate system.
var PanicHandler = PanicReporter(logPanicReporter{
	log: log.New(os.Stderr, "ERROR ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
})

type logPanicReporter struct {
	log *log.Logger
}

func (l logPanicReporter) Panic(url string, err interface{}, stack string) {
	l.log.Printf("PANIC\nURL: %v\nERROR: %v\nSTACK:\n%s\n", url, err, stack)
}
