//go:build !windows && !linux && !darwin

package keyboard

import (
	"errors"
	"time"

	"ggosu/internal/clock"
)

type Source struct{}

func Start(*clock.Counter, func(Event)) (*Source, error) {
	return nil, errors.New("the raw keyboard POC requires Windows")
}

func (*Source) Errors() <-chan error    { return nil }
func (*Source) SetActive(bool)          {}
func (*Source) Backend() string         { return "unsupported OS" }
func (*Source) Notice() string          { return "native keyboard input is unavailable" }
func (*Source) Now() time.Duration      { return 0 }
func (*Source) Close() error            { return nil }
func (*Source) Post(uint16, bool) error { return errors.New("requires Windows") }
