package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"

	"github.com/charliesbot/chai/internal/config"
	chaiinit "github.com/charliesbot/chai/internal/init"
	"github.com/charliesbot/chai/internal/platform"
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

	updateCmd := &ffcli.Command{
		Name:       "update",
		ShortUsage: "chai update",
		ShortHelp:  "Clone or pull dependencies",
		Exec: func(ctx context.Context, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(filepath.Join(home, "chai.toml"))
			if err != nil {
				return err
			}
			extensions := cfg.Gemini.Extensions
			if !platform.HasPlatform(cfg.Platforms, "gemini") {
				extensions = nil
			}
			return update.Run(cfg.Deps, extensions)
		},
	}

	rootFlags := flag.NewFlagSet("chai", flag.ExitOnError)
	showVersion := rootFlags.Bool("version", false, "print version and exit")

	root := &ffcli.Command{
		ShortUsage:  "chai <command> [flags]",
		ShortHelp:   "Keep AI coding agent configs in sync",
		FlagSet:     rootFlags,
		Subcommands: []*ffcli.Command{initCmd, syncCmd, updateCmd},
		Exec: func(ctx context.Context, args []string) error {
			if *showVersion {
				fmt.Println(resolveVersion())
				return nil
			}
			fmt.Println("chai — run 'chai init', 'chai sync', or 'chai update'")
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
