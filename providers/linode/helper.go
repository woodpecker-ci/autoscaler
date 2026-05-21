package linode

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

func generatePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

func (p *provider) resolveRegion(ctx context.Context, r string) error {
	// let linode decide
	if r == "" {
		return nil
	}

	// set by region id
	region, err := p.client.GetRegion(ctx, r)
	if err == nil {
		p.region = region
		return nil
	}

	// of if no valid id, we try to resolve by country
	list, err2 := p.client.ListRegions(ctx, nil)
	if err2 != nil {
		err = errors.Join(err, err2)
	} else if len(list) != 0 {
		for _, li := range list {
			if strings.EqualFold(li.Country, r) {
				log.Info().Msgf("region found by country match: %q", li.ID)
				p.region = &li
				return nil
			}
		}
	}

	return fmt.Errorf("could not resolve region %q: %w", r, err)
}

func (p *provider) resolveInstanceType(ctx context.Context, it string) error {
	lt, err := p.client.GetType(ctx, it)
	if err != nil {
		return fmt.Errorf("could not resolve instance type %q: %w", it, err)
	}
	p.instanceType = lt
	return nil
}

func (p *provider) resolveImage(ctx context.Context, i string) error {
	img, err := p.client.GetImage(ctx, i)
	if err != nil {
		return fmt.Errorf("could not resolve image %q: %w", i, err)
	}
	p.image = img
	return nil
}
