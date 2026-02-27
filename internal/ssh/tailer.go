package ssh

import (
	"context"
	"fmt"
	"io"
	"sync"

	"al.essio.dev/pkg/shellescape"
	gossh "golang.org/x/crypto/ssh"
)

// Tailer streams the output of `tail -f` on a remote file to a writer.
type Tailer struct {
	cancel      context.CancelFunc
	done        chan struct{}
	mu          sync.Mutex
	err         error
	errCallback func(error)
}

// SetErrCallback sets a function to be called when the tail stream ends with an error
// (e.g. SSH disconnect). The callback is invoked from a background goroutine.
func (t *Tailer) SetErrCallback(fn func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errCallback = fn
}

// StartTail begins tailing a remote file, writing output to w.
// The returned Tailer can be stopped via Stop().
func StartTail(ctx context.Context, client *gossh.Client, path string, lines int, w io.Writer) (*Tailer, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	cmd := fmt.Sprintf("tail -n %d -f %s", lines, shellescape.Quote(path))
	if err := sess.Start(cmd); err != nil {
		sess.Close()
		return nil, fmt.Errorf("starting tail: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	t := &Tailer{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(t.done)
		defer sess.Close()

		// Copy stdout to writer until context is cancelled or stream ends
		copyDone := make(chan error, 1)
		go func() {
			_, err := io.Copy(w, stdout)
			copyDone <- err
		}()

		select {
		case <-ctx.Done():
			// Context cancelled — signal the remote process to stop
			sess.Signal(gossh.SIGTERM)
			sess.Close()
		case err := <-copyDone:
			t.mu.Lock()
			t.err = err
			cb := t.errCallback
			t.mu.Unlock()
			if err != nil && cb != nil {
				cb(err)
			}
		}
	}()

	return t, nil
}

// Stop cancels the tail and waits for the goroutine to finish.
func (t *Tailer) Stop() {
	t.cancel()
	<-t.done
}

// Err returns any error that occurred during tailing.
func (t *Tailer) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}
