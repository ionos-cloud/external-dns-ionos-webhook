package main

import (
	"fmt"

	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/dnsprovider"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/logging"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/server"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

const banner = `
  ___ ___  _  _  ___  ___  
 |_ _/ _ \| \| |/ _ \/ __| 
  | | (_) | .  | (_) \__ \
 |___\___/|_|\_|\___/|___/
 external-dns-ionos-webhook
 version: %s (%s)

`

var (
	Version = "local"
	Gitsha  = "?"
)

func main() {
	fmt.Printf(banner, Version, Gitsha)
	logging.Init()
	config := configuration.Init()
	provider, err := dnsprovider.Init(config)
	if err != nil {
		log.Fatalf("Failed to initialize DNS provider: %v", err)
	}
	srv := server.Init(config, api.WebhookServer{Provider: provider})
	server.ShutdownGracefully(srv)
}
