package lib

import (
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/go-routeros/routeros/v3"
)

type WifiClient struct {
	MacAddress    string `json:"mac_address"`
	Interface     string `json:"interface"`
	Uptime        string `json:"uptime"`
	LastActivity  string `json:"last_activity"`
	SignalToNoise string `json:"signal_to_noise"`
}

type RouterOSMQTTBridge struct {
	MqttClient           mqtt.Client
	RouterOSClient       *routeros.Client
	TopicPrefix          string
	MQTTClientConfig     MQTTClientConfig
	RouterOSClientConfig RouterOSClientConfig
}

type RouterOSClientConfig struct {
	RouterAddress, Username, Password string
}

type MQTTClientConfig struct {
	MQTTBroker string
}

func CreateRouterOSClient(config RouterOSClientConfig) (*routeros.Client, error) {
	client, err := routeros.DialTLS(config.RouterAddress, config.Username, config.Password, &tls.Config{
		InsecureSkipVerify: true,
	})
	return client, err
}

func CreateMQTTClient(mqttBroker string) (mqtt.Client, error) {
	slog.Info("Creating MQTT client", "broker", mqttBroker)
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker).SetAutoReconnect(true)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("Could not connect to broker", "mqttBroker", mqttBroker, "error", token.Error())
		return nil, token.Error()
	}
	slog.Info("Connected to MQTT broker", "mqttBroker", mqttBroker)

	return client, nil
}

func NewRouterOSMQTTBridge(routerOSConfig RouterOSClientConfig, mqttClientConfig MQTTClientConfig, topicPrefix string) (*RouterOSMQTTBridge, error) {

	mqttClient, err := CreateMQTTClient(mqttClientConfig.MQTTBroker)
	if err != nil {
		slog.Error("Error creating mqtt client", "error", err, "broker", mqttClientConfig.MQTTBroker)
		return nil, err
	}

	routerOSClient, err := CreateRouterOSClient(routerOSConfig)
	if err != nil {
		slog.Error("Error creating RouterOS client", "error", err, "address", routerOSConfig.RouterAddress)
		return nil, err
	}

	bridge := &RouterOSMQTTBridge{
		MqttClient:           mqttClient,
		RouterOSClient:       routerOSClient,
		TopicPrefix:          topicPrefix,
		MQTTClientConfig:     mqttClientConfig,
		RouterOSClientConfig: routerOSConfig,
	}

	routerOSClient.Listen()
	return bridge, nil
}

func prefixify(topicPrefix, subtopic string) string {
	if len(strings.TrimSpace(topicPrefix)) > 0 {
		return topicPrefix + "/" + subtopic
	} else {
		return subtopic
	}
}

func (bridge *RouterOSMQTTBridge) PublishMQTT(subtopic string, message string, retained bool) {
	token := bridge.MqttClient.Publish(prefixify(bridge.TopicPrefix, subtopic), 0, retained, message)
	token.Wait()
}

func (bridge *RouterOSMQTTBridge) MainLoop() {
	for {
		reconnectRouterOsClient := false
		reply, err := bridge.RouterOSClient.Run("/interface/wireless/registration-table/print")
		if err != nil {
			slog.Error("Could not retrieve registration table", "error", err)
			reconnectRouterOsClient = true
		} else {
			var clients []WifiClient
			for _, re := range reply.Re {
				client := WifiClient{
					MacAddress:    re.Map["mac-address"],
					Interface:     re.Map["interface"],
					Uptime:        re.Map["uptime"],
					LastActivity:  re.Map["last-activity"],
					SignalToNoise: re.Map["signal-to-noise"],
				}
				clients = append(clients, client)
			}
			jsonData, err := json.MarshalIndent(clients, "", "    ")
			if err != nil {
				slog.Error("Failed to create json", "error", err)
				continue
			}
			bridge.PublishMQTT("routeros/wificlients", string(jsonData), false)
			bridge.MqttClient.IsConnected()
		}

		time.Sleep(30 * time.Second)
		if reconnectRouterOsClient {
			slog.Error("Reconnecting RouterOS client")
			err = bridge.RouterOSClient.Close()
			if err != nil {
				slog.Error("Error when closing RouterOS client", "error", err)
			}
			client, err := CreateRouterOSClient(bridge.RouterOSClientConfig)
			if err != nil {
				slog.Error("Error when recreating RouterOS client", "error", err)
			}
			bridge.RouterOSClient = client
		}
	}
}
