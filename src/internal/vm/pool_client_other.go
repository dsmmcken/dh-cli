//go:build !linux

package vm

import "errors"

// ErrPoolNotRunning is returned when the pool daemon socket cannot be reached.
var ErrPoolNotRunning = errors.New("pool daemon not running")

func PoolSocketPath() string { return "" }

func PoolProbe() bool { return false }

func PoolExec(req *PoolRequest) (*PoolResponse, error) {
	return nil, ErrPoolNotRunning
}

func PoolCheckout() (*CheckoutInfo, error) {
	return nil, ErrPoolNotRunning
}

func DestroyCheckedOutVM(_ *CheckoutInfo) {}

func PoolCommand(req *PoolRequest) (*PoolResponse, error) {
	return nil, ErrPoolNotRunning
}
