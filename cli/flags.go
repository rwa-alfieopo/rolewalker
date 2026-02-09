package cli

import (
	"strconv"
	"strings"
)

// FlagSet provides simple flag parsing for CLI commands.
type FlagSet struct {
	args       []string
	positional []string
	flags      map[string]string
	boolFlags  map[string]bool
}

// ParseFlags parses command-line arguments into positional args, string flags, and boolean flags.
func ParseFlags(args []string) *FlagSet {
	fs := &FlagSet{
		args:      args,
		flags:     make(map[string]string),
		boolFlags: make(map[string]bool),
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") || (strings.HasPrefix(arg, "-") && len(arg) > 1) {
			key := strings.TrimLeft(arg, "-")
			// Peek ahead for value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				fs.flags[key] = args[i+1]
				i++
			} else {
				fs.boolFlags[key] = true
			}
		} else {
			fs.positional = append(fs.positional, arg)
		}
	}
	return fs
}

// String returns the value of a string flag, or the default if not set.
func (fs *FlagSet) String(name string, defaultVal string) string {
	if v, ok := fs.flags[name]; ok {
		return v
	}
	return defaultVal
}

// Bool returns true if a boolean flag was set.
func (fs *FlagSet) Bool(name string) bool {
	return fs.boolFlags[name]
}

// Int returns the value of an integer flag, or the default if not set.
func (fs *FlagSet) Int(name string, defaultVal int) (int, error) {
	if v, ok := fs.flags[name]; ok {
		return strconv.Atoi(v)
	}
	return defaultVal, nil
}

// Positional returns all positional (non-flag) arguments.
func (fs *FlagSet) Positional() []string { return fs.positional }

// Arg returns the positional argument at index i, or empty string if out of range.
func (fs *FlagSet) Arg(i int) string {
	if i < len(fs.positional) {
		return fs.positional[i]
	}
	return ""
}
