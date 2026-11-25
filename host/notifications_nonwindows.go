//go:build !windows

package main

import "errors"

func notificationPermission() error {
	return errors.New("automatic Windows notification forwarding is available only in the Windows app")
}

func windowsNotificationWorker(bool) error {
	return errors.New("automatic Windows notification forwarding is available only in the Windows app")
}
