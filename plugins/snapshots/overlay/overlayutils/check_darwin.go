package overlayutils

import "errors"

func NeedsUserXAttr(d string) (bool, error) {
	return false, nil
}

func Supported(root string) error {
	return errors.New("overlayfs is not supported on darwin")
}
