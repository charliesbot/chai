package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	chaiinit "github.com/charliesbot/chai/internal/init"
	"github.com/peterbourgon/ff/v3/ffcli"
)

func main() {
	initCmd := &ffcli.Command{
		Name:       "init",
		ShortUsage: "chai init",
		ShortHelp:  "Scaffold a ~/chai.toml and agents.md",
		Exec: func(ctx context.Context, args []string) error {
			return chaiinit.Run()
		},
	}

	syncCmd := &ffcli.Command{
		Name:       "sync",
		ShortUsage: "chai sync",
		ShortHelp:  "Distribute config to all platforms",
		Exec: func(ctx context.Context, args []string) error {
			fmt.Println("chai sync: not implemented yet")
			return nil
		},
	}

	root := &ffcli.Command{
		ShortUsage:  "chai <command> [flags]",
		ShortHelp:   "Keep AI coding agent configs in sync",
		FlagSet:     flag.NewFlagSet("chai", flag.ExitOnError),
		Subcommands: []*ffcli.Command{initCmd, syncCmd},
		Exec: func(ctx context.Context, args []string) error {
			fmt.Println("chai — run 'chai init' or 'chai sync'")
			return nil
		},
	}

	if err := root.ParseAndRun(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
