package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type mqttConfig struct {
	Broker string `yaml:"broker"`

	Password string `yaml:"password"`
	UserName string `yaml:"username"`
}

type scrapingConfig struct {
	Simple []string `yaml:"simple"`
}

type httpConfig struct {
	Port string `yaml:"port"`
}

type config struct {
	LogLevel string         `yaml:"logLevel"`
	Http     httpConfig     `yaml:"http"`
	MQTT     mqttConfig     `yaml:"mqtt"`
	Scraping scrapingConfig `yaml:"scraping"`
}

func readConfig(path string) (config, error) {
	cfgData, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("cant read config: %w", err)
	}

	cfg := config{
		Http: httpConfig{
			Port: "2112",
		},
		MQTT: mqttConfig{
			Broker:   "tcp://localhost:1883",
			Password: "",
			UserName: "",
		},
		Scraping: scrapingConfig{},
	}

	err = yaml.Unmarshal(cfgData, &cfg)
	if err != nil {
		return config{}, fmt.Errorf("cant decode config: %w", err)
	}

	return cfg, nil
}
