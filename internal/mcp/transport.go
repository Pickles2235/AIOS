package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const DefaultMaxFrameBytes = 1 << 20

// RecoveringStdioTransport provides bounded newline-delimited JSON-RPC over
// stdin/stdout. Unlike the SDK's standard IO transport, it reports malformed
// frames as JSON-RPC errors and continues reading the same connection.
type RecoveringStdioTransport struct {
	MaxFrameBytes int
}

func (t *RecoveringStdioTransport) Connect(context.Context) (mcp.Connection, error) {
	return newRecoveringConnection(os.Stdin, nopWriteCloser{os.Stdout}, t.MaxFrameBytes), nil
}

type recoveringIOTransport struct {
	reader        io.ReadCloser
	writer        io.WriteCloser
	maxFrameBytes int
}

func (t *recoveringIOTransport) Connect(context.Context) (mcp.Connection, error) {
	return newRecoveringConnection(t.reader, t.writer, t.maxFrameBytes), nil
}

type recoveringConnection struct {
	reader io.ReadCloser
	writer io.WriteCloser

	incoming      <-chan messageOrError
	closed        chan struct{}
	closeOne      sync.Once
	closeErr      error
	writeMu       sync.Mutex
	maxFrameBytes int
}

type messageOrError struct {
	message jsonrpc.Message
	err     error
}

func newRecoveringConnection(reader io.ReadCloser, writer io.WriteCloser, maximum int) *recoveringConnection {
	if maximum <= 0 {
		maximum = DefaultMaxFrameBytes
	}
	incoming := make(chan messageOrError)
	connection := &recoveringConnection{reader: reader, writer: writer, incoming: incoming, closed: make(chan struct{}), maxFrameBytes: maximum}
	go connection.readLoop(maximum, incoming)
	return connection
}

func (c *recoveringConnection) readLoop(maximum int, incoming chan<- messageOrError) {
	defer close(incoming)
	reader := bufio.NewReaderSize(c.reader, min(maximum+1, 64<<10))
	for {
		frame, oversized, err := readFrame(reader, maximum)
		if len(frame) > 0 || oversized {
			if oversized {
				if writeErr := c.writeProtocolError(jsonrpc.CodeParseError, "Parse error: inbound frame exceeds configured limit"); writeErr != nil {
					c.deliver(incoming, nil, writeErr)
					return
				}
			} else if !json.Valid(frame) {
				if writeErr := c.writeProtocolError(jsonrpc.CodeParseError, "Parse error"); writeErr != nil {
					c.deliver(incoming, nil, writeErr)
					return
				}
			} else {
				message, decodeErr := jsonrpc.DecodeMessage(frame)
				if decodeErr != nil {
					if writeErr := c.writeProtocolError(jsonrpc.CodeInvalidRequest, "Invalid Request"); writeErr != nil {
						c.deliver(incoming, nil, writeErr)
						return
					}
				} else if !c.deliver(incoming, message, nil) {
					return
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.deliver(incoming, nil, io.EOF)
			} else {
				c.deliver(incoming, nil, err)
			}
			return
		}
	}
}

func readFrame(reader *bufio.Reader, maximum int) ([]byte, bool, error) {
	frame := make([]byte, 0, min(maximum, 4096))
	oversized := false
	for {
		fragment, err := reader.ReadSlice('\n')
		payload := fragment
		if len(payload) > 0 && payload[len(payload)-1] == '\n' {
			payload = payload[:len(payload)-1]
			if len(payload) > 0 && payload[len(payload)-1] == '\r' {
				payload = payload[:len(payload)-1]
			}
		}
		if !oversized {
			if len(frame)+len(payload) > maximum {
				oversized = true
				frame = nil
			} else {
				frame = append(frame, payload...)
			}
		}
		switch {
		case err == nil:
			return bytes.TrimSpace(frame), oversized, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return bytes.TrimSpace(frame), oversized, io.EOF
		default:
			return nil, oversized, err
		}
	}
}

func (c *recoveringConnection) deliver(ch chan<- messageOrError, message jsonrpc.Message, err error) bool {
	select {
	case ch <- messageOrError{message: message, err: err}:
		return true
	case <-c.closed:
		return false
	}
}

func (c *recoveringConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case item, ok := <-c.incoming:
		if !ok {
			return nil, io.EOF
		}
		return item.message, item.err
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *recoveringConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	encoded, err := jsonrpc.EncodeMessage(message)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC message: %w", err)
	}
	if len(encoded)+1 > c.maxFrameBytes {
		return fmt.Errorf("JSON-RPC response exceeds configured frame limit")
	}
	return c.writeLine(encoded)
}

func (c *recoveringConnection) writeProtocolError(code int64, message string) error {
	response := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Error   struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{JSONRPC: "2.0"}
	response.Error.Code = code
	response.Error.Message = message
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC protocol error: %w", err)
	}
	if len(encoded)+1 > c.maxFrameBytes {
		return fmt.Errorf("JSON-RPC protocol error exceeds configured frame limit")
	}
	return c.writeLine(encoded)
}

func (c *recoveringConnection) writeLine(encoded []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	encoded = append(encoded, '\n')
	_, err := c.writer.Write(encoded)
	return err
}

func (c *recoveringConnection) Close() error {
	c.closeOne.Do(func() {
		close(c.closed)
		c.closeErr = errors.Join(c.reader.Close(), c.writer.Close())
	})
	return c.closeErr
}

func (*recoveringConnection) SessionID() string { return "" }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
