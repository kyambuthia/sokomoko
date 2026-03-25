package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/kyambuthia/sokomoko/internal/config"
)

const (
	commandServe   = "serve"
	commandMigrate = "migrate"
	commandSeed    = "seed"
)

var ErrUsage = errors.New("invalid command usage")

type commandConfig struct {
	Name         string
	SeedOnServe  bool
	ShowHelpOnly bool
}

func run(args []string, stdout io.Writer) error {
	cmd, err := parseCommand(args)
	if err != nil {
		printUsage(stdout)
		return err
	}
	if cmd.ShowHelpOnly {
		printUsage(stdout)
		return nil
	}

	cfg := config.LoadFromEnv()
	switch cmd.Name {
	case commandMigrate:
		return runMigrate(cfg)
	case commandSeed:
		return runSeed(cfg)
	default:
		return runServe(cfg, cmd.SeedOnServe)
	}
}

func parseCommand(args []string) (commandConfig, error) {
	if len(args) == 0 {
		return commandConfig{Name: commandServe}, nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		return commandConfig{ShowHelpOnly: true}, nil
	case commandServe:
		cmd := commandConfig{Name: commandServe}
		for _, arg := range args[1:] {
			if arg == "--seed" {
				cmd.SeedOnServe = true
				continue
			}
			return commandConfig{}, fmt.Errorf("%w: unsupported serve option %q", ErrUsage, arg)
		}
		return cmd, nil
	case commandMigrate, commandSeed:
		if len(args) > 1 {
			return commandConfig{}, fmt.Errorf("%w: %s does not accept extra arguments", ErrUsage, args[0])
		}
		return commandConfig{Name: args[0]}, nil
	default:
		return commandConfig{}, fmt.Errorf("%w: unknown command %q", ErrUsage, args[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  sokomoko serve [--seed]")
	_, _ = fmt.Fprintln(w, "  sokomoko migrate")
	_, _ = fmt.Fprintln(w, "  sokomoko seed")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  serve      Apply schema and start the HTTP server.")
	_, _ = fmt.Fprintln(w, "  migrate    Apply schema changes and exit.")
	_, _ = fmt.Fprintln(w, "  seed       Apply schema, run bootstrap seed workflows, and exit.")
}
