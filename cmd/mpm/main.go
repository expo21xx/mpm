package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/expo21xx/mpm"
)

func main() {
	filepath := flag.String("file", "", "file to load")

	flag.Parse()

	m := mpm.New()

	err := m.LoadFile(*filepath)
	if err != nil {
		log.Fatal(err)
	}

	shutdownHandler(func() error {
		return m.Stop()
	})
}

func shutdownHandler(handler func() error) {
	// buffered channel because the signal module requires it
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	sig := <-shutdown

	err := handler()

	// wait for current requests to finish
	switch {
	case sig == syscall.SIGSTOP:
		log.Fatal("SIGSTOP caused shutdown")
	case err != nil:
		log.Fatal(err)
	}
}
