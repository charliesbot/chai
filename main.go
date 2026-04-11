package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/config"
	chaiinit "github.com/charliesbot/chai/internal/init"
	chaisync "github.com/charliesbot/chai/internal/sync"
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

	syncFlags := flag.NewFlagSet("chai sync", flag.ExitOnError)
	force := syncFlags.Bool("force", false, "overwrite files even if manually edited")

	syncCmd := &ffcli.Command{
		Name:       "sync",
		ShortUsage: "chai sync [--force]",
		ShortHelp:  "Distribute config to all platforms",
		FlagSet:    syncFlags,
		Exec: func(ctx context.Context, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(filepath.Join(home, "chai.toml"))
			if err != nil {
				return err
			}
			return chaisync.Run(cfg, chaisync.Options{Force: *force})
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
