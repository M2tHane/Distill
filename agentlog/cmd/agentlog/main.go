// Command agentlog lists and prints coding agent sessions.
//
//	agentlog list [-agent claude|codex|pi] [-json] [-cache file]
//	agentlog show [-json] [-inputs] <session.jsonl>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/dade/agentlog"
	"github.com/dade/agentlog/sources"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "list":
		list(os.Args[2:])
	case "show":
		show(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: agentlog list [-agent name] [-json] [-cache file]\n       agentlog show [-json] [-inputs] <session.jsonl>")
	os.Exit(2)
}

func list(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	agent := fs.String("agent", "", "only this agent: claude, codex or pi")
	asJSON := fs.Bool("json", false, "print JSON lines")
	cache := fs.String("cache", "", "cache file")
	fs.Parse(args)

	start := time.Now()
	ix := &agentlog.Index{Sources: sources.All(""), CachePath: *cache}
	res, err := ix.Scan(context.Background())
	if err != nil {
		fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	n := 0
	for _, s := range res.Sessions {
		if *agent != "" && string(s.Agent) != *agent {
			continue
		}
		n++
		if *asJSON {
			enc.Encode(s)
			continue
		}
		fmt.Printf("%-6s %s  %8s  %4d msgs  %-40s  %s\n", s.Agent, s.Updated.Local().Format("01-02 15:04"),
			size(s.Size), s.Messages, filepath.Base(s.CWD), s.Title)
	}
	fmt.Fprintf(os.Stderr, "%d sessions (%d parsed, %d errors) in %v\n", n, res.Parsed, len(res.Errors), time.Since(start).Round(time.Millisecond))
	for _, e := range res.Errors {
		fmt.Fprintln(os.Stderr, "  ", e)
	}
}

func show(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print the transcript as JSON")
	inputs := fs.Bool("inputs", false, "include full tool inputs")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	path, _ := filepath.Abs(fs.Arg(0))
	var t *agentlog.Transcript
	var err error
	for _, src := range sources.All("") {
		files, _ := src.Files()
		if slices.Contains(files, path) {
			t, err = src.Parse(path)
			break
		}
	}
	if t == nil && err == nil {
		err = fmt.Errorf("%s is not a session of a known agent", path)
	}
	if err != nil {
		fatal(err)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(t)
		return
	}
	agentlog.WriteMarkdown(os.Stdout, t, agentlog.MarkdownOptions{ToolInputs: *inputs})
}

func size(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "agentlog:", err)
	os.Exit(1)
}
