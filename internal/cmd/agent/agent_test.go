package main

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestParseDefault(t *testing.T) {
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	}()

	os.Args = []string{"agent"}
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	assert.False(t, parse())
}
