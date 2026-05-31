package assert

import (
	"fmt"
	"os"
	"log"
)

func runAssert(msg string) {
	log.Fatal(msg)
}

func Assert(truth bool, msg string) {
	if !truth {
		runAssert(msg)
	}
}

func NoError(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Assert No Error: %s\n", err)
		runAssert(msg)
	}
}
