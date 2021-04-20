package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	msg := flag.String("msg", "Hello there.", "message to print")
	interval := flag.Int("interval", 5, "interval to print message in seconds")

	flag.Parse()

	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				fmt.Println(*msg)
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	shutdownHandler(func() error {
		quit <- struct{}{}
		return nil
	})
}

func shutdownHandler(handler func() error) {
	// buffered channel because the signal module requires it
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	sig := <-shutdown

	err := handler()

	switch {
	case sig == syscall.SIGSTOP:
		log.Fatal("SIGSTOP caused shutdown")
	case err != nil:
		log.Fatal(err)
	}
}
