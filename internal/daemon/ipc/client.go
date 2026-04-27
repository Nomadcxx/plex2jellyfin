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
