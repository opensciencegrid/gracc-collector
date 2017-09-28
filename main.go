package main

import (
	"flag"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
)

// build parameters
var (
	build_ver  = "1.1.7"
	build_date = "???"
	build_ref  = "scratch"
)

// flags
var (
	configFile string
	logFile    string
	pprofOpt   string
)

func init() {
	flag.StringVar(&configFile, "c", "", "config file")
	flag.StringVar(&logFile, "l", "stderr", "log file: stdout, stderr, or file name")
	flag.StringVar(&pprofOpt, "pprof", "", "enable pprof: \"on\" to serve on main collector address at /debug/pprof, or provide address:port. Disabled if not set.")
}

func main() {
	flag.Parse()

	// need to set log output first, since we log everything else
	var flog *os.File
	var err error
	switch logFile {
	case "stdout":
		log.SetOutput(os.Stdout)
	case "stderr":
		log.SetOutput(os.Stdout)
	default:
		flog, err = os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil {
			log.Fatal(err)
		}
		defer flog.Close()
		log.SetOutput(flog)
		//log.SetFormatter(&log.JSONFormatter{})
		log.SetFormatter(&log.TextFormatter{DisableColors: true})
	}

	log.WithFields(log.Fields{
		"version": build_ver,
		"build":   build_date,
		"ref":     build_ref,
	}).Info("GRÅCC")

	config := DefaultConfig()
	if configFile != "" {
		log.WithField("file", configFile).Info("reading config")
		err := config.ReadConfig(configFile)
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := config.GetEnv(); err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"level": config.LogLevel,
		"file":  logFile,
	}).Info("initializing logging")
	logLevel, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(logLevel)

	log.Info("initializing collector")
	g, err := NewCollector(config)
	if err != nil {
		log.Fatal(err)
	}

	prometheus.MustRegister(g)

	log.WithFields(log.Fields{
		"address": config.Address,
		"port":    config.Port,
	}).Info("starting HTTP server")
	// We don't use the DefaultServeMux since pprof registers handlers with it, which we may not want.
	mux := http.NewServeMux()
	mux.Handle("/gratia-servlets/rmi", g)
	mux.HandleFunc("/stats", g.ServeStats)
	mux.Handle("/metrics", prometheus.Handler())
	srv := &http.Server{
		Addr:         config.Address + ":" + config.Port,
		Handler:      mux,
		ReadTimeout:  config.TimeoutDuration,
		WriteTimeout: config.TimeoutDuration,
	}
	go srv.ListenAndServe()

	if pprofOpt == "on" {
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		log.Infof("pprof available at %s:%s/debug/pprof/", config.Address, config.Port)
	} else if pprofOpt != "" {
		// pprof by defaults registers handlers with the DefaultServeMux, so
		// we just need to start a server using it.
		go http.ListenAndServe(pprofOpt, nil)
		log.Infof("pprof available at %s/debug/pprof/", pprofOpt)
	}

	// loop to catch signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals)
MainLoop:
	for {
		select {
		case s := <-signals:
			log.WithField("signal", s).Debug("got signal")
			switch s {
			case os.Interrupt, syscall.SIGTERM:
				// terminate
				log.WithField("signal", s).Info("exiting")
				break MainLoop
			case syscall.SIGUSR1, syscall.SIGHUP:
				// refresh log file
				if flog != nil {
					log.WithField("signal", s).Info("closing log")
					flog.Close()
					flog, err = os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
					if err != nil {
						log.Fatal(err)
					}
					defer flog.Close()
					log.SetOutput(flog)
				}
			case syscall.SIGUSR2:
				// toggle log level between debug and config option
				if log.GetLevel() != logLevel {
					log.SetLevel(logLevel)
				} else {
					log.SetLevel(log.DebugLevel)
				}
				log.WithFields(log.Fields{
					"signal": s,
					"level":  log.GetLevel().String(),
				}).Info("changing log level")
			}
		}
	}
}
