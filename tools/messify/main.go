// Command messify damages a clean draw.io file the way careless editing does,
// for building test inputs: it moves shapes and cuts wire ends loose.
//
//	messify [-seed N] [-jitter PX] [-share F] [-detach F] in.drawio out.drawio
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/internal/messify"
)

func main() {
	seed := flag.Int64("seed", 1, "random seed")
	jitter := flag.Float64("jitter", 30, "largest shape move, in pixels")
	share := flag.Float64("share", 0.4, "share of shapes moved")
	detach := flag.Float64("detach", 0.2, "share of wire ends cut loose")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: messify [flags] in.drawio out.drawio")
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	d, _, err := doc.Parse(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rng := rand.New(rand.NewSource(*seed))
	messify.Strip(d)
	messify.Jitter(d, rng, *share, *jitter)
	messify.Detach(d, rng, messify.DetachOptions{Share: *detach, MaxInside: 6, MaxOutside: 6})
	if err := os.WriteFile(flag.Arg(1), d.Bytes(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
