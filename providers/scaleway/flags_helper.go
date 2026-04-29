package scaleway

import (
	"fmt"

	"github.com/scaleway/scaleway-sdk-go/scw"
)

func allZones() (zones []string) {
	for i := range scw.AllZones {
		zones = append(zones, scw.AllZones[i].String())
	}
	return zones
}

func zoneValidator(sl []string) error {
	for _, s := range sl {
		if scw.Zone(s).Exists() {
			return fmt.Errorf("%w: %q does not exist", ErrInvalidZone, s)
		}
	}
	return nil
}
