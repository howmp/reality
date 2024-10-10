package main

import (
	"fmt"
	"os"

	"github.com/howmp/reality"
	"github.com/jessevdk/go-flags"
)

func main() {
	p := flags.NewParser(nil, flags.PassAfterNonOption|flags.HelpFlag)
	p.Name = "grss"
	logger := reality.GetLogger(true)
	p.AddCommand("gen", "generate server config and client", "generate server config and client", &gen{})
	p.AddCommand("serv", "run server", "run server", &serv{})
	writer := os.Stderr
	_, err := p.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok {
			if e.Type != flags.ErrHelp {
				p.WriteHelp(writer)
			}
			writer.WriteString(fmt.Sprintln(err))
		} else {
			logger.Fatal(err)
		}
	}

}
