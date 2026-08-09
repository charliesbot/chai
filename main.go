package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"

	chaiadd "github.com/charliesbot/chai/internal/add"
	"github.com/charliesbot/chai/internal/clean"
	"github.com/charliesbot/chai/internal/config"
	chaiinit "github.com/charliesbot/chai/internal/init"
	chaisync "github.com/charliesbot/chai/internal/sync"
	"github.com/charliesbot/chai/internal/update"
	"github.com/peterbourgon/ff/v3/ffcli"
)

// version is set at build time via -ldflags "-X main.version=..." for release builds.
// For local/go-install builds, resolveVersion() falls back to module version or git SHA.
var version = "dev"

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	// Prefer VCS info when present (repo builds) — shorter and more readable
	// than Go's synthesized pseudo-versions.
	var rev string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if dirty {
			return "dev-" + rev + "-dirty"
		}
		return "dev-" + rev
	}
	// No VCS info — likely a `go install pkg@vX.Y.Z` build from the module cache.
	if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	initCmd := &ffcli.Command{
		Name:       "init",
		ShortUsage: "chai init",
		ShortHelp:  "Scaffold a ~/chai.toml and AGENTS.md",
		Exec: func(ctx context.Context, args []string) error {
			return chaiinit.Run()
		},
	}

	addCmd := &ffcli.Command{
		Name:       "add",
		ShortUsage: "chai add <source> [--skill <name>...] [--list] [--yes] [--global]",
		ShortHelp:  "Add a local or public GitHub skill source",
		Exec: func(ctx context.Context, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			configPath := filepath.Join(home, "chai.toml")
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return chaiadd.RunWithHome(ctx, cfg, configPath, home, args, chaiadd.Options{
				SyncOptions: chaisync.Options{Prompt: chaisync.InteractivePrompt()},
			})
		},
	}

	syncFlags := flag.NewFlagSet("chai sync", flag.ExitOnError)
	force := syncFlags.Bool("force", false, "overwrite files even if manually edited")
	dryRun := syncFlags.Bool("dry-run", false, "preview sync without writing files")

	syncCmd := &ffcli.Command{
		Name:       "sync",
		ShortUsage: "chai sync [--force] [--dry-run]",
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
			opts := chaisync.Options{Force: *force, DryRun: *dryRun}
			if !*force && !*dryRun {
				opts.Prompt = chaisync.InteractivePrompt()
			}
			return chaisync.Run(ctx, cfg, opts)
		},
	}

	cleanFlags := flag.NewFlagSet("chai clean", flag.ExitOnError)
	cleanDryRun := cleanFlags.Bool("dry-run", false, "preview clean without deleting files")

	cleanCmd := &ffcli.Command{
		Name:       "clean",
		ShortUsage: "chai clean [--dry-run]",
		ShortHelp:  "Remove generated skills, subagents, and MCP config",
		FlagSet:    cleanFlags,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("clean does not accept positional arguments")
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(filepath.Join(home, "chai.toml"))
			if err != nil {
				return err
			}
			return clean.RunWithHome(ctx, cfg, home, clean.Options{DryRun: *cleanDryRun})
		},
	}

	updateCmd := &ffcli.Command{
		Name:       "update",
		ShortUsage: "chai update",
		ShortHelp:  "Refresh GitHub skills, dependencies, and plugins, then sync",
		Exec: func(ctx context.Context, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(filepath.Join(home, "chai.toml"))
			if err != nil {
				return err
			}
			return update.Run(ctx, cfg, update.Options{
				SyncOptions: chaisync.Options{Prompt: chaisync.InteractivePrompt()},
			})
		},
	}

	rootFlags := flag.NewFlagSet("chai", flag.ExitOnError)
	showVersion := rootFlags.Bool("version", false, "print version and exit")

	root := &ffcli.Command{
		ShortUsage:  "chai <command> [flags]",
		ShortHelp:   "Keep AI coding agent configs in sync",
		FlagSet:     rootFlags,
		Subcommands: []*ffcli.Command{initCmd, addCmd, syncCmd, cleanCmd, updateCmd},
		Exec: func(ctx context.Context, args []string) error {
			if *showVersion {
				fmt.Println(resolveVersion())
				return nil
			}
			fmt.Println("chai — run 'chai init', 'chai add', 'chai sync', 'chai clean', or 'chai update'")
			return nil
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := root.ParseAndRun(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
