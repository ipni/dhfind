package main

import (
	"context"
	"flag"
	"os"
	"os/signal"

	logging "github.com/ipfs/go-log/v2"
	"github.com/ischasny/dhfind/metrics"
	"github.com/ischasny/dhfind/server"
)

var (
	log = logging.Logger("cmd/daemon")
)

func main() {
	listenAddr := flag.String("listenAddr", "0.0.0.0:40080", "The dhfind HTTP server listen address.")
	dhstoreAddr := flag.String("dhstoreAddr", "", "The dhstore HTTP address.")
	metricsAddr := flag.String("metricsAddr", "0.0.0.0:40082", "Prometheus metrics HTTP address.")
	simulation := flag.Bool("simulation", false, "Whether dhfind runs in simulation mode.")
	llvl := flag.String("logLevel", "info", "The logging level. Only applied if GOLOG_LOG_LEVEL environment variable is unset.")

	flag.Parse()

	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("*", *llvl)
	}

	if *listenAddr == "" || *dhstoreAddr == "" || *metricsAddr == "" {
		panic("listen, dhstore and metrics addresses must be provided")
	}

	m, err := metrics.New(*metricsAddr)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	server, err := server.New(*listenAddr, *dhstoreAddr, m, *simulation)
	if err != nil {
		panic(err)
	}

	if err = m.Start(ctx); err != nil {
		panic(err)
	}

	if err = server.Start(ctx); err != nil {
		panic(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	log.Info("Terminating...")
	if err = m.Shutdown(ctx); err != nil {
		log.Warnw("Failure occurred while shutting down metrics server.", "err", err)
	}
	if err := server.Shutdown(ctx); err != nil {
		log.Warnw("Failure occurred while shutting down server.", "err", err)
	} else {
		log.Info("Shut down server successfully.")
	}
}
