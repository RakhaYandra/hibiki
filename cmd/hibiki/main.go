package main

import (
	"os"

	"github.com/RakhaYandra/hibiki/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
