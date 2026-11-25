package main

import "errors"

func platformInstallUpdate(_ updateOffer, _, _ string) (string, error) {
	return "", errors.New("Windows app updates are distributed through Microsoft Store")
}

func applyUpdate([]string) error {
	return errors.New("Windows app updates are distributed through Microsoft Store")
}
