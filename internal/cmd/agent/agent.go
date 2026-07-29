package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent"
	"github.com/spf13/pflag"
)

// parse parses the command line flags and populates the config struct.
// It returns true if a subcommand was handled and the program should exit.
func parse() bool {
	subcommand := ""
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}

	// Subcommands that don't require any pflag parsing
	switch subcommand {
	}

	// pflag.CommandLine.ParseErrorsWhitelist.UnknownFlags = true
	version := pflag.BoolP("version", "v", false, "Show version information")
	help := pflag.BoolP("help", "h", false, "Show this help message")

	// Convert old single-dash long flags to double-dash for backward compatibility
	pflag.Usage = func() {
		builder := strings.Builder{}
		builder.WriteString("Usage: ")
		builder.WriteString(os.Args[0])
		builder.WriteString(" [command] [flags]\n")
		builder.WriteString("\nFlags:\n")
		fmt.Print(builder.String())
		pflag.PrintDefaults()
	}

	// Parse all arguments with pflag
	pflag.Parse()

	// Must run after pflag.Parse()
	switch {
	case *version:
		fmt.Println(beszel.AppName+"-agent", beszel.Version)
		return true
	case *help || subcommand == "help":
		pflag.Usage()
		return true
	}

	return false
}

func main() {
	subcommandHandled := parse()

	if subcommandHandled {
		return
	}

	a, err := agent.NewAgent()
	if err != nil {
		log.Fatal("Failed to create agent: ", err)
	}

	if err := a.Start(); err != nil {
		log.Fatal("Failed to start: ", err)
	}
}
