package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

func main() {
	zapConfig := zap.NewDevelopmentConfig()
	logger, _ := zapConfig.Build()

	cfg, err := readConfig("config.yaml")
	if err != nil {
		logger.Fatal("Can't read config", zap.Error(err))
	}

	if cfg.LogLevel != "debug" {
		zapConfig.Level.SetLevel(zap.InfoLevel)
	}

	err = run(cfg, logger)
	if err != nil {
		logger.Fatal("Application failed", zap.Error(err))
	}
}

func run(cfg config, logger *zap.Logger) error {
	metrics := prometheus.NewRegistry()

    err := registerGauges(cfg.Scraping.Simple, metrics, logger)
    if err != nil {
        logger.Fatal("Failed to register gauges on connect, %s", zap.Error(err))
    }
	mq, err := connectMqtt(cfg.MQTT, cfg.Scraping.Simple, logger)
	if err != nil {
		return err
	}
	defer mq.Disconnect(uint(time.Second.Milliseconds()))

	logger.Info("Connected to mqtt broker")

	http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{}))

	addr := fmt.Sprintf("127.0.0.1:%s", cfg.Http.Port)

	logger.Info("Starting metrics server", zap.String("address", addr))

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		return fmt.Errorf("ListenAndServe error: %w", err)
	}

	return nil
}

func connectMqtt(cfg mqttConfig, gauges []string, logger *zap.Logger) (mqtt.Client, error) {
	mqttOpts := mqtt.NewClientOptions()

	mqttOpts.Password = cfg.Password
	mqttOpts.Username = cfg.UserName

	mqttOpts.AddBroker(cfg.Broker)
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetOrderMatters(false)

	mqttOpts.SetOnConnectHandler(func(mq mqtt.Client) {
		for _, subject := range gauges {
			name := strings.ReplaceAll(subject, "/", "_")
			token := mq.Subscribe(subject, 0, func(c mqtt.Client, m mqtt.Message) {
				value, err := strconv.ParseFloat(string(m.Payload()), 64)
				if err != nil {
					logger.Error("Invalid value", zap.String("subject", subject), zap.Error(err))
					return
				}

				metric, ok := registeredMetrics[name]
				if !ok {
					logger.Error("No metrics for subject", zap.String("subject", name))
					return
				}

				metric.Set(value)
				logger.Debug("Received value", zap.String("subject", subject), zap.String("message", string(m.Payload())))

			})

			if !token.WaitTimeout(time.Second) {
				logger.Error("Failed to subscribe mqtt topic.", zap.String("subject",subject), zap.Error(token.Error()))
			}
		}
	})

	mqttOpts.SetReconnectingHandler(func(mqtt.Client, *mqtt.ClientOptions) {
		logger.Info("Attempting reconnection...")
	})

	mqttOpts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		logger.Info("Connection lost", zap.Error(err))
	})

	mq := mqtt.NewClient(mqttOpts)

	token := mq.Connect()
	if !token.WaitTimeout(time.Second) {
		return nil, fmt.Errorf("Can't connect to mqtt broker")
	}

	return mq, nil
}

var registeredMetrics map[string]prometheus.Gauge = map[string]prometheus.Gauge{}

func registerGauges(gauges []string, metrics prometheus.Registerer, logger *zap.Logger) error {
	for _, subject := range gauges {
		name := strings.ReplaceAll(subject, "/", "_")

		if _, ok := registeredMetrics[name]; !ok {
			metric := prometheus.NewGauge(prometheus.GaugeOpts{
				Name: name,
			})

			err := metrics.Register(metric)
			if err != nil {
				return fmt.Errorf("registering metric %q for subject %q: %w", name, subject, err)
			}

            registeredMetrics[name] = metric
		}

		logger.Info("Registered new gauge for subject", zap.String("subject", subject))
	}

	return nil
}
