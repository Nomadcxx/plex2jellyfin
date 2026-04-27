package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
)

// Stream issues a streaming command and returns a frames channel.
// Closing the returned context aborts the stream. The errc channel
// receives a single value: nil on clean termination, or the parse/IO
// error.
func (c *Client) Stream(ctx context.Context, cmd Command, args any) (<-chan Frame, <-chan error) {
	frames := make(chan Frame, 32)
	errc := make(chan error, 1)
	go func() {
		defer close(frames)
		defer close(errc)
		conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
		if err != nil {
			errc <- err
			return
		}
		defer conn.Close()
		go func() {
			<-ctx.Done()
			conn.Close()
		}()
		id := uuid.NewString()
		req := Request{V: ProtocolVersion, ID: id, Cmd: cmd}
		if args != nil {
			b, _ := json.Marshal(args)
			req.Args = b
		}
		if err := json.NewEncoder(conn).Encode(req); err != nil {
			errc <- err
			return
		}
		dec := json.NewDecoder(bufio.NewReader(conn))
		for {
			var f Frame
			if err := dec.Decode(&f); err != nil {
				if ctx.Err() != nil {
					errc <- nil
					return
				}
				errc <- err
				return
			}
			if f.Type == FrameHeartbeat {
				continue
			}
			frames <- f
			if f.Type == FrameDone || f.Type == FrameError {
				errc <- nil
				return
			}
		}
	}()
	return frames, errc
}

type Client struct {
	path string
}

func NewClient(path string) *Client {
	return &Client{path: path}
}

func (c *Client) Call(ctx context.Context, cmd Command, args any) (json.RawMessage, error) {
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial ipc: %w", err)
	}
	defer conn.Close()

	id := uuid.NewString()
	req := Request{V: ProtocolVersion, ID: id, Cmd: cmd}
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		req.Args = b
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	_ = conn.SetReadDeadline(deadline)

	dec := json.NewDecoder(bufio.NewReader(conn))
	for {
		var f Frame
		if err := dec.Decode(&f); err != nil {
			return nil, err
		}
		if f.ID != id {
			continue
		}
		switch f.Type {
		case FrameResult:
			return f.Data, nil
		case FrameError:
			return nil, fmt.Errorf("ipc error %s: %s", f.Code, f.Msg)
		case FrameHeartbeat:
			continue
		default:
			return nil, errors.New("unexpected frame type for call")
		}
	}
}
