//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func launchAudioWorker(ctx context.Context) (<-chan error, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe, "audio-worker")
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return done, nil
}

func localAudioWorker(ctx context.Context) error {
	c, err := readConfig()
	if err != nil {
		return err
	}
	if !c.Audio.Enabled || c.Audio.Source != "linux" {
		return nil
	}
	parec, err := exec.LookPath("parec")
	if err != nil {
		return errors.New("Linux system-output capture requires parec")
	}
	cmd := exec.CommandContext(ctx, parec,
		"--device=@DEFAULT_MONITOR@",
		"--raw",
		"--format=s16le",
		fmt.Sprintf("--rate=%d", audioRate),
		fmt.Sprintf("--channels=%d", audioChannels),
		fmt.Sprintf("--latency-msec=%d", audioFrameMS),
		fmt.Sprintf("--process-time-msec=%d", audioFrameMS),
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	streamErr := hubAudioStream(ctx, c, "linux", stdout)
	if ctx.Err() == nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil
	}
	if streamErr != nil {
		return streamErr
	}
	if waitErr != nil {
		return waitErr
	}
	return errors.New("Linux system-output capture stopped")
}
