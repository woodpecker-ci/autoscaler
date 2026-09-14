package yandexcloud

import "errors"

var (
	ErrFolderIDRequired        = errors.New("folder ID is required")
	ErrSubnetIDRequired        = errors.New("subnet ID is required")
	ErrCredentialsRequired     = errors.New("one Yandex Cloud credential mode is required")
	ErrCredentialConflict      = errors.New("yandex cloud credential modes are mutually exclusive")
	ErrImageSelectorRequired   = errors.New("image ID or image family is required")
	ErrImageSelectorConflict   = errors.New("image ID and image family are mutually exclusive")
	ErrImageNotReady           = errors.New("image is not ready")
	ErrPoolIDInvalid           = errors.New("pool ID cannot be used in a Yandex Cloud VM name")
	ErrReservedLabelPrefix     = errors.New("reserved label prefix")
	ErrInvalidLabel            = errors.New("invalid Yandex Cloud label")
	ErrDuplicateInstance       = errors.New("multiple Yandex Cloud instances found")
	ErrSubnetFolderMismatch    = errors.New("subnet belongs to another folder")
	ErrOperationTimeoutInvalid = errors.New("operation timeout must be positive")
)
