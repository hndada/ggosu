//go:build !windows

package main

import "errors"

var errForeground = errors.New("OS input check requires Windows")

func sendPair(uint16) error { return errForeground }
