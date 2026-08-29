//go:build windows
// +build windows

package service

import (
	stdlog "log"
	"log/slog"

	"golang.org/x/sys/windows/svc"
)

type beatExporterService struct {
	stopCh chan<- bool
}

func (s *beatExporterService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s.stopCh <- true
				break loop
			default:
				slog.Error("unexpected control request", "code", c.Cmd)
			}
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}

// SetupServiceListener setups service handler for windows
func SetupServiceListener(stopCh chan<- bool, serviceName string, logger *stdlog.Logger) error {
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return err
	}

	if !isInteractive {
		go func() {
			err = svc.Run(serviceName, &beatExporterService{stopCh: stopCh})
			if err != nil {
				logger.Printf("Failed to start service: %v", err)
			}
		}()
	}

	return nil
}
