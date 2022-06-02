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
    mq, err := connectMqtt(cfg.MQTT)
    if err != nil {
        return err
    }
    defer mq.Disconnect(uint(time.Second.Milliseconds()))

	logger.Info("Connected to mqtt broker")

	metrics := prometheus.NewRegistry()

	err = registerGauges(cfg.Scraping.Simple, mq, metrics, logger)
	if err != nil {
		return err
	}

	http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{}))

    addr := fmt.Sprintf("127.0.0.1:%s", cfg.Http.Port)

	logger.Info("Starting metrics server", zap.String("address", addr))
    
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		return fmt.Errorf("ListenAndServe error: %w", err)
	}

	return nil
}

func connectMqtt(cfg mqttConfig) (mqtt.Client, error) {
	mqttOpts := mqtt.NewClientOptions()

	mqttOpts.Password = cfg.Password
	mqttOpts.Username = cfg.UserName

	mqttOpts.AddBroker(cfg.Broker)

	mq := mqtt.NewClient(mqttOpts)

	token := mq.Connect()
	if !token.WaitTimeout(time.Second) {
		return nil, fmt.Errorf("Can't connect to mqtt broker")
	}

    return mq, nil
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
