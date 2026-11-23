//go:build !unix

package main

import (
	"context"

	"cozycast/internal/diag"
)

func dumpOnSignal(context.Context, *diag.Watchdog) {}
