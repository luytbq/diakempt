// Command diakempt tidies the layout of draw.io files.
//
// It reads each file, writes the result next to it with .tidy before the
// extension (or where -o says, or over the input with --in-place), and prints a
// report on stderr. Exit codes: 0 success, 1 a file could not be read, processed
// or written, 2 bad flags, 3 --strict found a warning or a worse result.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/luytbq/diakempt"
	"github.com/luytbq/diakempt/issue"
	"github.com/luytbq/diakempt/report"
)

const version = "0.1.0-dev"

const (
	exitOK     = 0
	exitFile   = 1
	exitUsage  = 2
	exitStrict = 3
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// config is the parsed command line.
type config struct {
	opt     diakempt.Options
	files   []string
	out     string
	inPlace bool
	dryRun  bool
	json    bool
	verbose bool
	strict  bool
}

// flagSet declares every flag. Operations and settings come from the core's
// declarations, so a new one gets its flag without touching this file.
func flagSet(cfg *config, stderr io.Writer) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("diakempt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	level := fs.String("level", string(diakempt.Normal), "how far the layout may change: safe, normal or aggressive")
	fs.StringVar(&cfg.opt.Kind, "type", "", "treat every diagram as this kind: "+strings.Join(diakempt.Kinds, ", "))
	fs.StringVar(&cfg.out, "o", "", "output path, or - for stdout (one input file only)")
	fs.BoolVar(&cfg.inPlace, "in-place", false, "overwrite the input, keeping a numbered .bak copy")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "write no file, only print the report")
	fs.BoolVar(&cfg.json, "json", false, "print the report as JSON on stdout")
	fs.BoolVar(&cfg.verbose, "verbose", false, "list every change and the detection signals")
	fs.BoolVar(&cfg.strict, "strict", false, "exit 3 when there is a warning or a diagram got worse")
	fs.BoolVar(&cfg.opt.Force, "force", false, "keep results that score worse than the original")
	fs.Int64Var(&cfg.opt.Seed, "seed", 1, "seed for randomized optimizers")
	showVersion := fs.Bool("version", false, "print the version and exit")

	on := map[string]*bool{}
	off := map[string]*bool{}
	for _, op := range diakempt.Operations {
		on[op.Name] = fs.Bool("with-"+op.Name, false, "run "+op.Name+" whatever the level: "+op.Help)
		off[op.Name] = fs.Bool("no-"+op.Name, false, "skip "+op.Name)
	}
	vals := map[string]*float64{}
	for _, p := range diakempt.Params {
		vals[p.Name] = fs.Float64(p.Name, p.Default, fmt.Sprintf("%s (%v to %v)", p.Help, p.Min, p.Max))
	}
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: diakempt [flags] file.drawio ...\n\n")
		fs.PrintDefaults()
	}
	finish := func() error {
		if *showVersion {
			return errVersion
		}
		cfg.opt.Level = diakempt.Level(*level)
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		for _, op := range diakempt.Operations {
			if *on[op.Name] && *off[op.Name] {
				return fmt.Errorf("--with-%s and --no-%s contradict each other", op.Name, op.Name)
			}
			if *on[op.Name] || *off[op.Name] {
				if cfg.opt.Ops == nil {
					cfg.opt.Ops = map[string]bool{}
				}
				cfg.opt.Ops[op.Name] = *on[op.Name]
			}
		}
		for _, p := range diakempt.Params {
			if set[p.Name] {
				if cfg.opt.Values == nil {
					cfg.opt.Values = map[string]float64{}
				}
				cfg.opt.Values[p.Name] = *vals[p.Name]
			}
		}
		return cfg.opt.Validate()
	}
	return fs, finish
}

var errVersion = errors.New("version")

// parseArgs accepts flags before, between and after file names; "--" ends
// flags.
func parseArgs(args []string, stderr io.Writer) (config, error) {
	var cfg config
	fs, finish := flagSet(&cfg, stderr)
	for {
		if err := fs.Parse(args); err != nil {
			return cfg, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		if rest[0] == "--" {
			cfg.files = append(cfg.files, rest[1:]...)
			break
		}
		cfg.files = append(cfg.files, rest[0])
		args = rest[1:]
	}
	if err := finish(); err != nil {
		return cfg, err
	}
	switch {
	case len(cfg.files) == 0:
		return cfg, errors.New("no input file")
	case cfg.out != "" && len(cfg.files) > 1:
		return cfg, errors.New("-o takes one input file; without it each result goes next to its input")
	case cfg.out != "" && cfg.inPlace:
		return cfg, errors.New("-o and --in-place contradict each other")
	case cfg.out == "-" && cfg.json:
		return cfg, errors.New("-o - and --json both write to stdout; use one")
	}
	return cfg, nil
}

// fileResult is one file's entry in the JSON report.
type fileResult struct {
	File   string       `json:"file"`
	Output string       `json:"output,omitempty"`
	Report *report.File `json:"report,omitempty"`
	Error  *issue.Issue `json:"error,omitempty"`
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args, stderr)
	if errors.Is(err, errVersion) {
		fmt.Fprintln(stdout, "diakempt", version)
		return exitOK
	}
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		fmt.Fprintln(stderr, "diakempt:", err)
		return exitUsage
	}
	code := exitOK
	var results []fileResult
	strictHit := false
	for _, name := range cfg.files {
		fr, ok := processFile(cfg, name, stdout, stderr)
		results = append(results, fr)
		if !ok {
			code = exitFile
			continue
		}
		if fr.Report.Warnings() > 0 || fr.Report.Worse() {
			strictHit = true
		}
	}
	if cfg.json {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"version": version, "files": results})
	}
	if code == exitOK && cfg.strict && strictHit {
		return exitStrict
	}
	return code
}

func processFile(cfg config, name string, stdout, stderr io.Writer) (fileResult, bool) {
	fr := fileResult{File: name}
	fail := func(is issue.Issue) (fileResult, bool) {
		fr.Error = &is
		fmt.Fprintf(stderr, "%s: error: %s [%s]\n", name, is.Msg, is.Code)
		return fr, false
	}
	in, err := os.ReadFile(name)
	if err != nil {
		return fail(issue.FileError("io.read", "cannot read: %v", err))
	}
	res, err := diakempt.Tidy(in, cfg.opt)
	if err != nil {
		var is issue.Issue
		if !errors.As(err, &is) {
			is = issue.FileError("internal", "%v", err)
		}
		return fail(is)
	}
	fr.Report = &res.Report
	note := ""
	switch {
	case cfg.dryRun:
		note = "dry run, nothing written"
	case cfg.out == "-":
		if _, err := stdout.Write(res.Output); err != nil {
			return fail(issue.FileError("io.write", "cannot write to stdout: %v", err))
		}
		fr.Output = "-"
	case cfg.inPlace:
		if !res.Report.Changed {
			note = "unchanged, input left as it was"
			break
		}
		bak, err := backup(name, in)
		if err != nil {
			return fail(issue.FileError("io.write", "cannot write a backup: %v", err))
		}
		if err := writeFile(name, res.Output); err != nil {
			return fail(issue.FileError("io.write", "cannot write: %v", err))
		}
		fr.Output = name
		note = "overwrote the input, backup in " + bak
	default:
		out := cfg.out
		if out == "" {
			out = tidyPath(name)
		}
		_, statErr := os.Stat(out)
		if err := writeFile(out, res.Output); err != nil {
			return fail(issue.FileError("io.write", "cannot write %s: %v", out, err))
		}
		fr.Output = out
		note = "wrote " + out
		if statErr == nil {
			note += " (replaced the existing file)"
		}
	}
	if !cfg.json {
		io.WriteString(stderr, res.Report.Text(name, cfg.verbose))
		if note != "" {
			fmt.Fprintf(stderr, "  %s\n", note)
		}
	}
	return fr, true
}

// tidyPath puts .tidy before the extension: diagram.drawio becomes
// diagram.tidy.drawio. A name that already carries .tidy is returned as is, so
// tidying a result again writes over that result instead of stacking suffixes.
func tidyPath(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if strings.HasSuffix(base, ".tidy") {
		return name
	}
	return base + ".tidy" + ext
}

// backup copies data to the first free name among name.bak, name.bak.1, ...
func backup(name string, data []byte) (string, error) {
	for i := 0; ; i++ {
		bak := name + ".bak"
		if i > 0 {
			bak = fmt.Sprintf("%s.bak.%d", name, i)
		}
		f, err := os.OpenFile(bak, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return bak, err
	}
}

// writeFile replaces name through a temporary file in the same directory, so a
// failed write never leaves a half-written file behind.
func writeFile(name string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(name); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".diakempt-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
