//go:build !windows

package main

import "os/exec"

// trackChild is a no-op off Windows; only Windows needs a job object.
func trackChild(*exec.Cmd) {}
