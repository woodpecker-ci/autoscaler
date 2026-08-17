package azure

import "errors"

var (
	ErrSubscriptionIDNotSet = errors.New("no azure subscription id provided")
	ErrResourceGroupNotSet  = errors.New("no azure resource group provided")
	ErrSubnetIDNotSet       = errors.New("no azure subnet id provided")
	ErrSSHPublicKeyNotSet   = errors.New("no azure ssh public key provided")
)

// imageReference holds an Azure marketplace image reference.
type imageReference struct {
	publisher string
	offer     string
	sku       string
	version   string
}
