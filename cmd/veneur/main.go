package main

import (
	"flag"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/profile"
	"github.com/stripe/veneur"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func main() {
	defer profile.Start(profile.CPUProfile).Stop()
	flag.Parse()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	conf, err := veneur.ReadConfig(*configFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error reading config file")
	}
	server, err := veneur.NewFromConfig(conf)
	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize server")
	}
	defer func() {
		server.ConsumePanic(recover())
	}()

	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, conf.MetricMaxLength)
		},
	}

	// Read forever!
	for i := 0; i < conf.NumReaders; i++ {
		go func() {
			defer func() {
				server.ConsumePanic(recover())
			}()
			server.ReadSocket(packetPool, conf.NumReaders != 1)
		}()
	}

	go func() {
		defer func() {
			server.ConsumePanic(recover())
		}()
		ticker := time.NewTicker(conf.Interval)
		for range ticker.C {
			server.Flush(conf.Interval, conf.FlushLimit)
		}
	}()

	server.HTTPServe()
}
