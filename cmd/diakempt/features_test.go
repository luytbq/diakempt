package main

import (
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/luytbq/diakempt"
)

// TestFeaturesListEveryCapability keeps docs/features.md true: agents learn the
// tool from it, so every flag, level, operation, setting and kind the code
// declares must appear there.
func TestFeaturesListEveryCapability(t *testing.T) {
	data, err := os.ReadFile("../../docs/features.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	var cfg config
	fs, _ := flagSet(&cfg, io.Discard)
	fs.VisitAll(func(f *flag.Flag) {
		name := "--" + f.Name
		if len(f.Name) == 1 {
			name = "-" + f.Name
		}
		for _, op := range diakempt.Operations {
			if f.Name == "with-"+op.Name || f.Name == "no-"+op.Name {
				return // covered by --with-NAME, --no-NAME and the operation list
			}
		}
		if !strings.Contains(text, name) {
			t.Errorf("flag %s is missing from docs/features.md", name)
		}
	})
	for _, l := range diakempt.Levels {
		if !strings.Contains(text, string(l)) {
			t.Errorf("level %s is missing", l)
		}
	}
	for _, op := range diakempt.Operations {
		if !strings.Contains(text, op.Name) {
			t.Errorf("operation %s is missing", op.Name)
		}
	}
	for _, k := range diakempt.Kinds {
		if !strings.Contains(text, k) {
			t.Errorf("kind %s is missing", k)
		}
	}
	for _, f := range []string{"--with-NAME", "--no-NAME"} {
		if !strings.Contains(text, f) {
			t.Errorf("%s is missing", f)
		}
	}
}
