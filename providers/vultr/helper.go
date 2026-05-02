package vultr

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/vultr/govultr/v3"
)

func (p *provider) resolveRegion(ctx context.Context, region string) error {
	regions, _, _, err := p.client.Region.List(ctx, &govultr.ListOptions{})
	if err != nil {
		return fmt.Errorf("could not fetch regions: %w", err)
	}
	for _, r := range regions {
		if region == r.ID {
			// we have an exact match
			p.region = r
			return nil
		}
		if strings.EqualFold(region, r.City) {
			log.Info().Msgf("update region to %q based on city match", r.ID)
			p.region = r
		}
	}
	return ErrInvalidRegion
}

func (p *provider) resolvePlan(ctx context.Context, plan string) error {
	plans, _, _, err := p.client.Plan.List(ctx, "", &govultr.ListOptions{})
	if err != nil {
		return fmt.Errorf("could not fetch plans: %w", err)
	}
	for _, pl := range plans {
		if pl.ID == plan {
			if slices.Contains(pl.Locations, p.region.ID) {
				p.plan = pl
				return nil
			}
			return fmt.Errorf("selected plan exist but not for region %q", p.region)
		}
	}

	return ErrInvalidPlan
}

func (p *provider) resolveImage(ctx context.Context, image string) error {
	ose, _, _, err := p.client.OS.List(ctx, &govultr.ListOptions{})
	if err != nil {
		return fmt.Errorf("could not fetch images: %w", err)
	}
	var matches []govultr.OS
	want := strings.ReplaceAll(strings.ToLower(image), " ", "")
	for _, os := range ose {
		got := strings.ReplaceAll(strings.ToLower(os.Name), " ", "")
		if strings.HasPrefix(got, want) {
			matches = append(matches, os)
			log.Trace().Msgf("resolve image got match: %q", os.Name)
		}
	}

	switch len(matches) {
	case 0:
		return ErrInvalidImage
	case 1:
		p.image = matches[0]
	default:
		// we first sort matches
		slices.SortFunc(matches, func(a, b govultr.OS) int { return strings.Compare(a.Name, b.Name) })
		p.image = matches[0]
		log.Info().Msgf("image selector had %d matches, choose %q", len(matches), matches[0].Name)
	}
	return nil
}

func (p *provider) printResolvedConfig() {
	log.Info().
		Str("city", p.region.City).
		Str("country", p.region.Country).
		Str("continent", p.region.Continent).
		Msg("deploy region")

	log.Info().
		Float32("monthly_cost", p.plan.MonthlyCost).
		Str("type", p.plan.Type).
		Int("cpu_count", p.plan.VCPUCount).
		Int("gpu_vram", p.plan.GPUVRAM).
		Int("ram", p.plan.RAM).
		Int("storage", p.plan.Disk).
		Msg("deploy with plan")

	log.Info().
		Str("name", p.image.Name).
		Str("family", p.image.Family).
		Str("arch", p.image.Arch).
		Msg("deploy with image")
}

func (p *provider) setupKeyPair(ctx context.Context) error {
	res, _, _, err := p.client.SSHKey.List(ctx, nil)
	if err != nil {
		return err
	}

	index := map[string]string{}
	for key := range res {
		index[res[key].Name] = res[key].ID
	}

	// if the account has multiple keys configured try to
	// use an existing key based on naming convention.
	for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
		fingerprint, ok := index[name]
		if !ok {
			continue
		}
		p.sshKeys = append(p.sshKeys, fingerprint)

		return nil
	}

	// if there were no matches but the account has at least
	// one key-pair already created we will select the first
	// in the list.
	if len(res) > 0 {
		p.sshKeys = append(p.sshKeys, res[0].ID)
		return nil
	}

	return ErrSSHKeyNotFound
}

// imageToGoArch maps architecture based on image to Go GOARCH strings.
func imageToGoArch(i govultr.OS) string {
	switch i.Arch {
	case "x64":
		return "arm64"
	default:
		return i.Arch
	}
}
