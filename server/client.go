package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/proxy"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Client interface {
	woodpecker.Client
}

// NewClient returns a new client from the CLI context.
func NewClient(ctx context.Context, c *cli.Command) (Client, error) {
	var (
		skip        = c.Bool("skip-verify")
		socks       = c.String("socks-proxy")
		socksoff    = c.Bool("socks-proxy-off")
		serverToken = c.String("server-token")
		serverURL   = c.String("server-url")
	)
	serverURL = strings.TrimRight(serverURL, "/")

	if len(serverURL) == 0 {
		return nil, fmt.Errorf("please provide the woodpecker server address")
	}
	if len(serverToken) == 0 {
		return nil, fmt.Errorf("please provide a woodpecker access token")
	}

	// attempt to find system CA certs
	certs, err := x509.SystemCertPool()
	if err != nil {
		log.Error().Err(err).Msg("ca certs not found")
	}
	tlsConfig := &tls.Config{
		RootCAs:            certs,
		InsecureSkipVerify: skip,
	}

	config := new(oauth2.Config)
	client := config.Client(
		ctx,
		&oauth2.Token{
			AccessToken: serverToken,
		},
	)

	trans, _ := client.Transport.(*oauth2.Transport)

	if len(socks) != 0 && !socksoff {
		dialer, err := proxy.SOCKS5("tcp", socks, nil, proxy.Direct)
		if err != nil {
			return nil, err
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer with context support")
		}
		trans.Base = &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
			DialContext:     contextDialer.DialContext,
		}
	} else {
		trans.Base = &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
		}
	}

	return woodpecker.NewClient(serverURL, client), nil
}
