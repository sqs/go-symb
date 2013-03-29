package foo

import (
	"flag"
	"fmt"
)

func main() {
	flag.Usage = func() {}
	flag.Parse()
	fmt.Println("Hello!")
}
