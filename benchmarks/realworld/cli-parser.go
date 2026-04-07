// Copyright 2015 Steve Francia. All rights reserved.
// Licensed under the Apache License, Version 2.0.
// Source: github.com/spf13/cobra (Apache 2.0 License)
// This is a representative snippet for benchmarking purposes.

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoCommand is returned when no subcommand is specified.
var ErrNoCommand = errors.New("no command specified")

// ErrUnknownCommand is returned when an unrecognized command name is used.
type ErrUnknownCommand struct {
	Name string
}

func (e *ErrUnknownCommand) Error() string {
	return fmt.Sprintf("unknown command: %q", e.Name)
}

// Flag represents a single CLI flag definition.
type Flag struct {
	Name      string
	Shorthand string
	Usage     string
	Default   interface{}
	value     interface{}
}

// FlagSet holds a collection of flags for a command.
type FlagSet struct {
	flags map[string]*Flag
}

// NewFlagSet creates an empty FlagSet.
func NewFlagSet() *FlagSet {
	return &FlagSet{flags: make(map[string]*Flag)}
}

// StringVar registers a string flag.
func (fs *FlagSet) StringVar(p *string, name, shorthand, defaultVal, usage string) {
	*p = defaultVal
	fs.flags[name] = &Flag{Name: name, Shorthand: shorthand, Usage: usage, Default: defaultVal, value: p}
}

// BoolVar registers a boolean flag.
func (fs *FlagSet) BoolVar(p *bool, name, shorthand string, defaultVal bool, usage string) {
	*p = defaultVal
	fs.flags[name] = &Flag{Name: name, Shorthand: shorthand, Usage: usage, Default: defaultVal, value: p}
}

// IntVar registers an integer flag.
func (fs *FlagSet) IntVar(p *int, name, shorthand string, defaultVal int, usage string) {
	*p = defaultVal
	fs.flags[name] = &Flag{Name: name, Shorthand: shorthand, Usage: usage, Default: defaultVal, value: p}
}

// Parse processes a slice of arguments against the registered flags.
// It returns remaining positional arguments.
func (fs *FlagSet) Parse(args []string) ([]string, error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		value := ""
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
		}

		flag, ok := fs.lookup(name)
		if !ok {
			return nil, fmt.Errorf("unknown flag: --%s", name)
		}

		switch p := flag.value.(type) {
		case *bool:
			if value == "" || value == "true" {
				*p = true
			} else if value == "false" {
				*p = false
			} else {
				return nil, fmt.Errorf("invalid boolean value %q for --%s", value, name)
			}
		case *string:
			if value == "" {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag --%s requires an argument", name)
				}
				i++
				value = args[i]
			}
			*p = value
		case *int:
			if value == "" {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag --%s requires an argument", name)
				}
				i++
				value = args[i]
			}
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
				return nil, fmt.Errorf("invalid integer value %q for --%s", value, name)
			}
			*p = n
		}
	}
	return positional, nil
}

func (fs *FlagSet) lookup(name string) (*Flag, bool) {
	if f, ok := fs.flags[name]; ok {
		return f, true
	}
	for _, f := range fs.flags {
		if f.Shorthand == name {
			return f, true
		}
	}
	return nil, false
}

// RunFunc is the function signature for command handlers.
type RunFunc func(cmd *Command, args []string) error

// Command represents a CLI command with optional subcommands.
type Command struct {
	Use     string
	Short   string
	Long    string
	RunE    RunFunc
	Flags   *FlagSet
	parent  *Command
	cmds    map[string]*Command
	out     io.Writer
}

// NewCommand creates a new root command.
func NewCommand(use, short string) *Command {
	return &Command{
		Use:   use,
		Short: short,
		Flags: NewFlagSet(),
		cmds:  make(map[string]*Command),
		out:   os.Stdout,
	}
}

// AddCommand registers a subcommand.
func (c *Command) AddCommand(sub *Command) {
	name := strings.Fields(sub.Use)[0]
	sub.parent = c
	c.cmds[name] = sub
}

// SetOutput redirects all command output.
func (c *Command) SetOutput(w io.Writer) {
	c.out = w
}

// UsageLine returns a short one-line usage description.
func (c *Command) UsageLine() string {
	parts := []string{c.commandPath()}
	if len(c.Flags.flags) > 0 {
		parts = append(parts, "[flags]")
	}
	if len(c.cmds) > 0 {
		parts = append(parts, "[command]")
	}
	return strings.Join(parts, " ")
}

func (c *Command) commandPath() string {
	if c.parent == nil {
		return filepath.Base(strings.Fields(c.Use)[0])
	}
	return c.parent.commandPath() + " " + strings.Fields(c.Use)[0]
}

// PrintUsage writes formatted usage information.
func (c *Command) PrintUsage() {
	fmt.Fprintf(c.out, "Usage:\n  %s\n\n", c.UsageLine())
	if c.Long != "" {
		fmt.Fprintf(c.out, "%s\n\n", c.Long)
	} else if c.Short != "" {
		fmt.Fprintf(c.out, "%s\n\n", c.Short)
	}

	if len(c.cmds) > 0 {
		fmt.Fprintln(c.out, "Available Commands:")
		names := make([]string, 0, len(c.cmds))
		for name := range c.cmds {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			sub := c.cmds[name]
			fmt.Fprintf(c.out, "  %-15s %s\n", name, sub.Short)
		}
		fmt.Fprintln(c.out)
	}

	if len(c.Flags.flags) > 0 {
		fmt.Fprintln(c.out, "Flags:")
		names := make([]string, 0, len(c.Flags.flags))
		for name := range c.Flags.flags {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			f := c.Flags.flags[name]
			short := ""
			if f.Shorthand != "" {
				short = fmt.Sprintf("-%s, ", f.Shorthand)
			}
			fmt.Fprintf(c.out, "  %s--%s\t%s (default: %v)\n", short, f.Name, f.Usage, f.Default)
		}
		fmt.Fprintln(c.out)
	}
}

// Execute parses os.Args[1:] and dispatches to the appropriate command.
func (c *Command) Execute() error {
	return c.ExecuteArgs(os.Args[1:])
}

// ExecuteArgs parses the given args and dispatches accordingly.
func (c *Command) ExecuteArgs(args []string) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name := args[0]
		if name == "help" {
			c.PrintUsage()
			return nil
		}
		if sub, ok := c.cmds[name]; ok {
			return sub.ExecuteArgs(args[1:])
		}
		return &ErrUnknownCommand{Name: name}
	}

	remaining, err := c.Flags.Parse(args)
	if err != nil {
		fmt.Fprintf(c.out, "Error: %v\n\n", err)
		c.PrintUsage()
		return err
	}

	if c.RunE == nil {
		if len(c.cmds) > 0 {
			c.PrintUsage()
			return ErrNoCommand
		}
		return nil
	}

	return c.RunE(c, remaining)
}
