//go:build linux || darwin

package service

import (
	stdlog "log"
	"os"
	"os/signal"
	"syscall"
)

// SetupServiceListener setups singal handler
func SetupServiceListener(stopCh chan<- bool, serviceName string, logger *stdlog.Logger) error {
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGHUP)
		logger.Printf("Signal received: %v", <-sigs)
		stopCh <- true
		close(stopCh)
	}()

	return nil
}
