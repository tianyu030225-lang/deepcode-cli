package main

import (
	"context"
	"os"
)

func startEscWatcher(cancel context.CancelFunc) *escWatcher {
	w := &escWatcher{
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	go func() {
		defer close(w.stopped)
		_ = withRawModeTimed(func() error {
			buf := make([]byte, 1)
			for {
				select {
				case <-w.done:
					return nil
				default:
				}
				n, err := os.Stdin.Read(buf)
				if err != nil {
					return nil
				}
				if n == 0 {
					continue
				}
				if buf[0] == 27 {
					w.once.Do(func() {
						close(w.canceled)
						cancel()
					})
					return nil
				}
			}
		})
	}()
	return w
}

func (w *escWatcher) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.done)
		<-w.stopped
	})
}

func (w *escWatcher) WasCanceled() bool {
	if w == nil {
		return false
	}
	select {
	case <-w.canceled:
		return true
	default:
		return false
	}
}
