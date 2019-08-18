package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	cfg      Config
	useTLS   bool
	certPath string
	keyPath  string
	version  = "0.0.1"
)

type Config struct {
	ServePort                   int    `default:"8080"`
	MetricsPort                 int    `default:"8888"`
	InitContainerCPURequests    string `default:"100m"`
	InitContainerCPULimits      string `default:"200m"`
	InitContainerMemoryRequests string `default:"64Mi"`
	InitContainerMemoryLimits   string `default:"128Mi"`
	InitImage                   string `default:"simonmacklin/vault-cli:0.0.2"`
}

func main() {
	sigs := make(chan os.Signal)
	stop := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	flag.BoolVar(&useTLS, "UseTLS", true, "start http server with tls enabled")
	flag.StringVar(&certPath, "CertPath", "/etc/webhook/certs/cert.pem", "path to the tls certificate")
	flag.StringVar(&keyPath, "KeyPath", "/etc/webhook/certs/key.pem", "path to the tls private key")
	flag.Parse()

	if err := envconfig.Process("app", &cfg); err != nil {
		log.Fatal(err.Error())
	}

	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})

	go func() {
		log.Infof("starting http server on port %d", cfg.ServePort)
		http.HandleFunc("/mutate", serve)
		if useTLS {
			log.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%d", cfg.ServePort), certPath, keyPath, nil))
		} else {
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.ServePort), nil))
		}
	}()

	go func() {
		log.Infof("starting metrics server on port %d", cfg.MetricsPort)
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(fmt.Sprintf(":%d", cfg.MetricsPort), nil)
	}()

	go func() {
		sig := <-sigs
		fmt.Println(sig)
		stop <- true
	}()

	<-stop
	log.Error("recieved kill signal closing down")
}
