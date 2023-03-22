package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
	"golang.org/x/net/proxy"
	"golang.org/x/oauth2"
)

// NewClient returns a new client from the CLI context.
func NewClient(c *cli.Context) (woodpecker.Client, error) {
	var (
		skip     = c.Bool("skip-verify")
		socks    = c.String("socks-proxy")
		socksoff = c.Bool("socks-proxy-off")
		token    = c.String("token")
		server   = c.String("server")
	)
	server = strings.TrimRight(server, "/")

	// if no server url is provided we can default
	// to the hosted Woodpecker service.
	if len(server) == 0 {
		return nil, fmt.Errorf("Error: you must provide the Woodpecker server address")
	}
	if len(token) == 0 {
		return nil, fmt.Errorf("Error: you must provide your Woodpecker access token")
	}

	// attempt to find system CA certs
	certs, err := x509.SystemCertPool()
	if err != nil {
		log.Error().Msgf("failed to find system CA certs: %v", err)
	}
	tlsConfig := &tls.Config{
		RootCAs:            certs,
		InsecureSkipVerify: skip,
	}

	config := new(oauth2.Config)
	client := config.Client(
		c.Context,
		&oauth2.Token{
			AccessToken: token,
		},
	)

	trans, _ := client.Transport.(*oauth2.Transport)

	if len(socks) != 0 && !socksoff {
		dialer, err := proxy.SOCKS5("tcp", socks, nil, proxy.Direct)
		if err != nil {
			return nil, err
		}
		trans.Base = &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
			Dial:            dialer.Dial,
		}
	} else {
		trans.Base = &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
		}
	}

	return woodpecker.NewClient(server, client), nil
}
