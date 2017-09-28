package main

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	log "github.com/Sirupsen/logrus"
)

type CollectorConfig struct {
	Address         string        `env:"GRACC_ADDRESS"`
	Port            string        `env:"GRACC_PORT"`
	Timeout         string        `env:"GRACC_TIMEOUT"`
	TimeoutDuration time.Duration `env:"-"`
	LogLevel        string        `env:"GRACC_LOGLEVEL"`
	AMQP            AMQPConfig    `env:"GRACC_AMQP_"`
	StartBufferSize int           `env:"GRACC_STARTBUFFERSIZE"`
	MaxBufferSize   int           `env:"GRACC_MAXBUFFERSIZE"`
}

func DefaultConfig() *CollectorConfig {
	var conf = CollectorConfig{
		Address:  "",
		Port:     "8080",
		Timeout:  "60s",
		LogLevel: "info",
		AMQP: AMQPConfig{
			Host:         "localhost",
			Port:         "5672",
			Scheme:       "amqp",
			Format:       "json",
			User:         "guest",
			Password:     "guest",
			Exchange:     "gracc",
			ExchangeType: "fanout",
			Durable:      false,
			AutoDelete:   true,
			Internal:     false,
			RoutingKey:   "",
			Retry:        "1s",
			MaxRetry:     "1h",
			SkipVerify:   false,
		},
		StartBufferSize: 4096,
		MaxBufferSize:   512 * 1024,
	}
	if err := conf.Validate(); err != nil {
		log.Fatalf("Error in default config: %s", err)
	}
	return &conf
}

// Validate checks that the config is valid, and performs
// any necessary conversions (e.g. time durations).
func (c *CollectorConfig) Validate() error {
	var err error
	c.TimeoutDuration, err = time.ParseDuration(c.Timeout)
	if err != nil {
		return fmt.Errorf("error parsing Timeout: %s", err)
	}
	return c.AMQP.Validate()
}

// ReadConfig reads the configuration from a TOML file.
// Defaults should already be set.
func (c *CollectorConfig) ReadConfig(file string) error {
	if _, err := toml.DecodeFile(file, c); err != nil {
		return err

	}
	return c.Validate()
}

// GetEnv will check the environment for config values, and
// set them if defined.
func (c *CollectorConfig) GetEnv() error {
	setEnvByTag(c, "")
	return c.Validate()
}

func setEnvByTag(d interface{}, prefix string) {
	t := reflect.ValueOf(d).Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		tfield := t.Field(i)
		if tfield.Tag.Get("env") == "-" {
			continue
		}
		envVar := prefix + tfield.Tag.Get("env")
		field := reflect.ValueOf(d).Elem().Field(i)
		if tfield.Type.Kind() == reflect.Struct {
			setEnvByTag(field.Addr().Interface(), tfield.Tag.Get("env"))
		} else if val := os.Getenv(envVar); val != "" {
			log.WithField("var", envVar).Info("using environment variable")
			var err error
			switch tfield.Type.Kind() {
			case reflect.String:
				field.SetString(val)
			case reflect.Int, reflect.Int32:
				var v int64
				if v, err = strconv.ParseInt(val, 10, 32); err == nil {
					field.SetInt(v)
				}
			case reflect.Int64:
				var v int64
				if v, err = strconv.ParseInt(val, 10, 64); err == nil {
					field.SetInt(v)
				}
			case reflect.Bool:
				var v bool
				if v, err = strconv.ParseBool(val); err == nil {
					field.SetBool(v)
				}
			default:
				err = fmt.Errorf("unhandled type")
			}
			if err != nil {
				log.WithFields(log.Fields{
					"var":  envVar,
					"type": tfield.Type.String(),
					"err":  err,
				}).Fatal("unable to set config val from env")
			}
		}
	}
}
