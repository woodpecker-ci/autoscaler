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

func (p *Provider) resolveRegion(ctx context.Context, r string) error {
	region, err := p.client.GetRegion(ctx, r)
	if err != nil {
		// we try to resolve by name
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

		return fmt.Errorf("could not resolve region: %w", err)
	}
	p.region = region
	return nil
}
