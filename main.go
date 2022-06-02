package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func main() {
	cfgData, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Println("cant read config: ", err.Error())
		os.Exit(1)
	}

	cfg := config{}

	err = yaml.Unmarshal(cfgData, &cfg)
	if err != nil {
		fmt.Println("cant decode config:", err.Error())
		os.Exit(1)
	}

	logger, _ := zap.NewDevelopmentConfig().Build()

	err = run(cfg, logger)
	if err != nil {
		logger.Error("Application failed", zap.Error(err))
		os.Exit(1)
	}
}

type mqttConfig struct {
	Password string `yaml:"password"`
	UserName string `yaml:"username"`
	Broker   string `yaml:"broker"`
}

type scrapingConfig struct {
	Simple []string `yaml:"simple"`
}

type config struct {
	MQTT     mqttConfig     `yaml:"mqtt"`
	Scraping scrapingConfig `yaml:"scraping"`
}

func run(cfg config, logger *zap.Logger) error {
	mqttOpts := mqtt.NewClientOptions()

	mqttOpts.Password = cfg.MQTT.Password
	mqttOpts.Username = cfg.MQTT.UserName

	mqttOpts.AddBroker(cfg.MQTT.Broker)

	mq := mqtt.NewClient(mqttOpts)

	token := mq.Connect()
	if !token.WaitTimeout(time.Second) {
		logger.Error("Can't connect to mqtt broker")
	}

	defer mq.Disconnect(1000)

	logger.Info("Connected to mqtt broker")

	metrics := prometheus.NewRegistry()

	err := registerGauges(cfg.Scraping.Simple, mq, metrics, logger)
	if err != nil {
		return err
	}

	logger.Info("Starting metrics server")

	http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{}))

	err = http.ListenAndServe(":2112", nil)
	if err != nil {
		return fmt.Errorf("ListenAndServe error: %w", err)
	}

	return nil
}

func registerGauges(gauges []string, mq mqtt.Client, metrics prometheus.Registerer, logger *zap.Logger) error {
	for _, subject := range gauges {
		name := strings.ReplaceAll(subject, "/", "_")

		metric := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: name,
		})

		err := metrics.Register(metric)
		if err != nil {
			return fmt.Errorf("registering metric %q for subject %q: %w", name, subject, err)
		}

		token := mq.Subscribe(subject, 0, func(c mqtt.Client, m mqtt.Message) {
			value, err := strconv.ParseFloat(string(m.Payload()), 64)
			if err != nil {
				logger.Error("Invalid value", zap.String("subject", subject), zap.Error(err))
				return
			}

			metric.Set(value)
			logger.Debug("Received value", zap.String("subject", subject), zap.String("message", string(m.Payload())))
		})

		if !token.WaitTimeout(time.Second) {
			return fmt.Errorf("can't subscribe to subject %q: %w", subject, token.Error())
		}

		logger.Info("Registered new gauge for subject", zap.String("subject", subject))
	}

	return nil
}
