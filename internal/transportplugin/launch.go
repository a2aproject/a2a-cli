// Copyright 2026 The A2A Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transportplugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
)

const handshakeTimeout = 15 * time.Second
const gracefulStopTimeout = 5 * time.Second

type execLauncher struct{}

var _ launcher = execLauncher{}

func (execLauncher) launch(ctx context.Context, binary, endpoint string) (*session, error) {
	cmd := exec.Command(binary, clitransport.SubcommandServe, "--endpoint", endpoint)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting plugin: %w", err)
	}

	sess := &session{
		closeFunc: func() error {
			_ = stdin.Close()

			done := make(chan error, 1)
			go func() {
				cause := cmd.Wait()
				var exitErr *exec.ExitError
				if errors.As(cause, &exitErr) {
					cause = nil
				}
				done <- cause
			}()

			timer := time.NewTimer(gracefulStopTimeout)
			defer timer.Stop()

			select {
			case err := <-done:
				return err

			case <-timer.C:
				_ = cmd.Process.Kill()
				return <-done
			}
		},
	}

	hs, err := readHandshake(ctx, stdout)
	if err != nil {
		_ = sess.close()
		return nil, err
	}
	sess.handshake = hs

	// drain any further stdout so the plugin never blocks on a full pipe.
	go func() { _, _ = io.Copy(io.Discard, stdout) }()

	return sess, nil
}

func readHandshake(ctx context.Context, r io.Reader) (*clitransport.Endpoint, error) {
	type result struct {
		body *clitransport.Endpoint
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		if err != nil && line == "" {
			ch <- result{err: fmt.Errorf("reading handshake: %w", err)}
			return
		}
		var hs clitransport.Handshake
		if err := json.Unmarshal([]byte(line), &hs); err != nil {
			ch <- result{err: fmt.Errorf("parsing handshake %q: %w", line, err)}
			return
		}
		if !hs.Success {
			msg := hs.Error
			if msg == "" {
				msg = "unknown error"
			}
			ch <- result{err: fmt.Errorf("plugin failed to start: %s", msg)}
			return
		}
		if hs.Endpoint == nil || hs.Endpoint.Address == "" || hs.Endpoint.Binding == "" {
			ch <- result{err: fmt.Errorf("plugin handshake missing connection details: %q", line)}
			return
		}
		ch <- result{body: hs.Endpoint}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case <-time.After(handshakeTimeout):
		return nil, fmt.Errorf("timed out waiting for plugin handshake after %s", handshakeTimeout)

	case res := <-ch:
		return res.body, res.err
	}
}
