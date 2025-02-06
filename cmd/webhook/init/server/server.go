package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"

	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"

	"github.com/ionos-cloud/external-dns-ionos-webhook/pkg/webhook"
)

// Init server initialization function
// The server will respond to the following endpoints:
// - / (GET): initialization, negotiates headers and returns the domain filter
// - /records (GET): returns the current records
// - /records (POST): applies the changes
// - /adjustendpoints (POST): executes the AdjustEndpoints method
func Init(config configuration.Config, p *webhook.Webhook) *http.Server {
	r := chi.NewRouter()
	r.Get("/", p.Negotiate)
	r.Get("/records", p.Records)
	r.Post("/records", p.ApplyChanges)
	r.Post("/adjustendpoints", p.AdjustEndpoints)

	srv := createHTTPServer(fmt.Sprintf("%s:%d", config.ServerHost, config.ServerPort), r, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Infof("starting server on addr: '%s' ", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("can't serve on addr: '%s', error: %v", srv.Addr, err)
		}
	}()

	rHealth := chi.NewRouter()
	rHealth.Get("/health", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("healthy"))
		},
	))
	srvHealth := createHTTPServer(fmt.Sprintf("%s:%d", config.HealthHost, config.HealthPort), rHealth, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Infof("starting health server on addr: '%s'", srvHealth.Addr)
		if err := srvHealth.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("can't start health server on addr: '%s', error: %v", srvHealth.Addr, err)
		}
	}()

	if config.MetricsServer && config.MetricsPort != config.ServerPort {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			log.Infof("starting metrics server on port: %d", config.MetricsPort)
			if err := http.ListenAndServe(fmt.Sprintf(":%d", config.MetricsPort), nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Errorf("can't serve metrics server on addr: ':%d', error: %v", config.MetricsPort, err)
			}
		}()
	} else {
		r.Get("/metrics", promhttp.Handler().ServeHTTP)
	}

	return srv
}

func createHTTPServer(addr string, hand http.Handler, readTimeout, writeTimeout time.Duration) *http.Server {
	return &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Addr:         addr,
		Handler:      hand,
	}
}

// ShutdownGracefully gracefully shutdown the http server
func ShutdownGracefully(srv *http.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigCh
	log.Infof("shutting down server due to received signal: %v", sig)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("error shutting down server: %v", err)
	}
	cancel()
}
