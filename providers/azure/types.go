package azure

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrSubscriptionIDNotSet = errors.New("no azure subscription id provided")
	ErrResourceGroupNotSet  = errors.New("no azure resource group provided")
	ErrSubnetIDNotSet       = errors.New("no azure subnet id provided")
	ErrSSHPublicKeyNotSet   = errors.New("no azure ssh public key provided")
	ErrImageInvalid         = errors.New("azure image must be in 'publisher:offer:sku:version' form")
)

// imageReference holds a parsed marketplace image URN.
type imageReference struct {
	publisher string
	offer     string
	sku       string
	version   string
}

// parseImageURN splits a 'publisher:offer:sku:version' URN into its parts.
func parseImageURN(urn string) (imageReference, error) {
	parts := strings.SplitN(urn, ":", 4)
	if len(parts) != 4 {
		return imageReference{}, fmt.Errorf("%w: got %q", ErrImageInvalid, urn)
	}
	for _, p := range parts {
		if p == "" {
			return imageReference{}, fmt.Errorf("%w: got %q", ErrImageInvalid, urn)
		}
	}
	return imageReference{
		publisher: parts[0],
		offer:     parts[1],
		sku:       parts[2],
		version:   parts[3],
	}, nil
}
