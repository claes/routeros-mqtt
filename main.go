package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/claes/routeros-mqtt/lib"
)

var debug *bool

func printHelp() {
	fmt.Println("Usage: routeros-mqtt [OPTIONS]")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func main() {
	routerAddress := flag.String("address", "", "Mikrotik TLS address:port")
	username := flag.String("username", "", "Username")
	password := flag.String("password", "", "Password")
	topicPrefix := flag.String("topicPrefix", "", "MQTT topic prefix to use")
	mqttBroker := flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	help := flag.Bool("help", false, "Print help")
	debug = flag.Bool("debug", false, "Debug logging")
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	routerOSClientConfig :=
		lib.RouterOSClientConfig{RouterAddress: *routerAddress, Username: *username, Password: *password}

	mqttClientConfig :=
		lib.MQTTClientConfig{MQTTBroker: *mqttBroker}

	bridge, err := lib.NewRouterOSMQTTBridge(routerOSClientConfig, mqttClientConfig, *topicPrefix)

	if err != nil {
		slog.Error("Error creating RouterOS-MQTT bridge", "error", err)
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	fmt.Printf("Started\n")
	go bridge.MainLoop()
	<-c
	bridge.RouterOSClient.Close()
	fmt.Printf("Shut down\n")

	os.Exit(0)
}
