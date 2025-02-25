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
	rWebhook := chi.NewRouter()
	rWebhook.Get("/", p.Negotiate)
	rWebhook.Get("/records", p.Records)
	rWebhook.Post("/records", p.ApplyChanges)
	rWebhook.Post("/adjustendpoints", p.AdjustEndpoints)

	srvWebhook := createHTTPServer(fmt.Sprintf("%s:%d", config.ServerHost, config.ServerPort), rWebhook, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Infof("starting webhook server on addr: '%s' ", srvWebhook.Addr)
		if err := srvWebhook.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("can't start webhook server on addr: '%s', error: %v", srvWebhook.Addr, err)
		}
	}()

	// Minor note: (Emilija) According to docs, the liveness probe EP should be "/healthz"
	// https://kubernetes-sigs.github.io/external-dns/latest/docs/tutorials/webhook-provider/#exposed-endpoints
	rExposed := chi.NewRouter()
	rExposed.Get("/health", healthCheckHandler)
	rExposed.Get("/metrics", promhttp.Handler().ServeHTTP)

	srvExposed := createHTTPServer(fmt.Sprintf("%s:%d", config.MetricsHost, config.MetricsPort), rExposed, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Infof("starting server for exposed endpoints on addr: '%s'", srvExposed.Addr)
		if err := srvExposed.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("can't start exposed server on addr: '%s', error: %v", srvExposed.Addr, err)
		}
	}()

	return srvWebhook
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

func healthCheckHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
