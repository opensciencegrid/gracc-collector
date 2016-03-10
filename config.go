package main

import (
	"github.com/BurntSushi/toml"
)

type CollectorConfig struct {
	Address       string
	Port          string
	LogLevel      string
	File          FileConfig
	Elasticsearch ElasticsearchConfig
	Kafka         KafkaConfig
	AMQP          AMQPConfig
}

func ReadConfig(file string) (*CollectorConfig, error) {
	var conf = CollectorConfig{
		Address:  "",
		Port:     "8080",
		LogLevel: "info",
		File: FileConfig{
			Enabled: false,
			Path:    ".",
			Format:  "xml",
		},
		Elasticsearch: ElasticsearchConfig{
			Enabled: false,
			Host:    "localhost",
			Index:   "gracc",
		},
		Kafka: KafkaConfig{
			Enabled: false,
			Brokers: []string{"localhost:9092"},
			Topic:   "gracc",
		},
		AMQP: AMQPConfig{
			Enabled: false,
			Host:    "localhost",
			Port:    "5672",
			Format:  "rawxml",
		},
	}
	if _, err := toml.DecodeFile(file, &conf); err != nil {
		return nil, err
	}
	return &conf, nil
}
