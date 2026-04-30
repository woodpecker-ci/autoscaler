package hetznercloud

import "errors"

var (
	ErrIllegalLabelPrefix   = errors.New("illegal label prefix")
	ErrImageNotFound        = errors.New("image not found")
	ErrSSHKeyNotFound       = errors.New("SSH key not found")
	ErrNetworkNotFound      = errors.New("network not found")
	ErrFirewallNotFound     = errors.New("firewall not found")
	ErrServerTypeNotFound   = errors.New("server type not found")
	ErrLocationNotSupported = errors.New("location not available for server type")
)
