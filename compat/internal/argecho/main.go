// Command argecho prints each of its os.Args[1:] entries on its own
// line to stdout, then exits zero. It is used by compat_test.go to
// verify how Runner constructs argv (notably the WithSubcommand
// prepend behavior) without depending on any system command's flag
// parsing.
package main

import (
	"fmt"
	"os"
)

func main() {
	for _, a := range os.Args[1:] {
		fmt.Println(a)
	}
}
