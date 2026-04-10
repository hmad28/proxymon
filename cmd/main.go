package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ayanacorp/proxymon/internal/app"
	cfg "github.com/ayanacorp/proxymon/internal/config"
	"github.com/ayanacorp/proxymon/internal/tray"
)

var version = "0.1.0"

func main() {
	addr := flag.String("addr", "127.0.0.1:1080", "Proxy listen address")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("proxymon v%s\n", version)
		os.Exit(0)
	}

	logFile, err := os.OpenFile("proxymon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open log file: %v\n", err)
	} else {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Printf("Starting Proxymon v%s", version)

	var controller *app.Controller
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC: %v — attempting proxy disable", r)
			if controller != nil {
				controller.EmergencyDisableProxy()
			}
			os.Exit(1)
		}
	}()

	store, err := cfg.NewStore()
	if err != nil {
		log.Printf("Failed to initialize config store: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	controller, err = app.NewController(*addr, store, version)
	if err != nil {
		log.Printf("Failed to initialize controller: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received interrupt signal — disabling proxy")
		if controller != nil {
			controller.EmergencyDisableProxy()
		}
		os.Exit(0)
	}()

	tray.New(controller).Run()
	log.Println("Proxymon exited cleanly")
}
