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
	MqttClient     mqtt.Client
	RouterOSClient *routeros.Client
	TopicPrefix    string
	//sendMutex      sync.Mutex
}

func CreateRouterOSClient(address, username, password string) (*routeros.Client, error) {
	return routeros.DialTLS(address, username, password, &tls.Config{
		InsecureSkipVerify: true,
	})
}

func CreateMQTTClient(mqttBroker string) (mqtt.Client, error) {
	slog.Info("Creating MQTT client", "broker", mqttBroker)
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("Could not connect to broker", "mqttBroker", mqttBroker, "error", token.Error())
		return nil, token.Error()
	}
	slog.Info("Connected to MQTT broker", "mqttBroker", mqttBroker)
	return client, nil
}

func NewRouterOSMQTTBridge(routerOSClient *routeros.Client, mqttClient mqtt.Client, topicPrefix string) *RouterOSMQTTBridge {

	bridge := &RouterOSMQTTBridge{
		MqttClient:     mqttClient,
		RouterOSClient: routerOSClient,
		TopicPrefix:    topicPrefix,
	}
	return bridge
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
	go func() {
		for {
			reply, err := bridge.RouterOSClient.Run("/interface/wireless/registration-table/print")
			if err != nil {
				slog.Error("Could not retrieve registration table", "error", err)
			}

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

			time.Sleep(30 * time.Second)
			bridge.reconnectIfNeeded()
		}
	}()
}

func (bridge *RouterOSMQTTBridge) reconnectIfNeeded() {
}
