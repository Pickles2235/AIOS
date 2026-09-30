package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

type testReadCloser struct{ io.Reader }

func (testReadCloser) Close() error { return nil }

type testWriteCloser struct{ bytes.Buffer }

func (*testWriteCloser) Close() error { return nil }

func TestRecoveringConnectionPreflightsOutboundFrame(t *testing.T) {
	writer := &testWriteCloser{}
	connection := newRecoveringConnection(testReadCloser{strings.NewReader("")}, writer, 64)
	defer connection.Close()
	id, err := jsonrpc.MakeID("1")
	if err != nil {
		t.Fatal(err)
	}
	message := &jsonrpc.Request{ID: id, Method: "x", Params: []byte(`{"payload":"` + strings.Repeat("x", 128) + `"}`)}
	if err := connection.Write(context.Background(), message); err == nil || !strings.Contains(err.Error(), "frame limit") {
		t.Fatalf("oversized write error=%v", err)
	}
	if writer.Len() != 0 {
		t.Fatalf("oversized response was partially written: %q", writer.String())
	}
}

func TestRecoveringConnectionReportsMalformedAndContinues(t *testing.T) {
	valid := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	writer := &testWriteCloser{}
	connection := newRecoveringConnection(testReadCloser{strings.NewReader("not-json\n" + valid + "\n")}, writer, 256)
	defer connection.Close()
	message, err := connection.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if request, ok := message.(*jsonrpc.Request); !ok || request.Method != "ping" {
		t.Fatalf("message=%#v", message)
	}
	line, err := bufio.NewReader(strings.NewReader(writer.String())).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(bytes.TrimSpace(line)) || !bytes.Contains(line, []byte(`"code":-32700`)) {
		t.Fatalf("protocol output=%q", line)
	}
}

func TestRecoveringConnectionRejectsOversizeAndRecovers(t *testing.T) {
	valid := `{"jsonrpc":"2.0","id":1,"method":"after"}`
	writer := &testWriteCloser{}
	input := strings.Repeat("x", 257) + "\n" + valid + "\n"
	connection := newRecoveringConnection(testReadCloser{strings.NewReader(input)}, writer, 256)
	defer connection.Close()
	message, err := connection.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if request, ok := message.(*jsonrpc.Request); !ok || request.Method != "after" {
		t.Fatalf("message=%#v", message)
	}
	if !strings.Contains(writer.String(), `"code":-32700`) || !strings.HasSuffix(writer.String(), "\n") {
		t.Fatalf("protocol output=%q", writer.String())
	}
}

func TestRecoveringConnectionRejectsInvalidRequestAndAcceptsExactLimit(t *testing.T) {
	valid := `{"jsonrpc":"2.0","id":1,"method":"exact"}`
	padding := 256 - len(valid)
	frame := valid[:len(valid)-1] + strings.Repeat(" ", padding) + "}"
	if len(frame) != 256 {
		t.Fatalf("frame bytes=%d", len(frame))
	}
	writer := &testWriteCloser{}
	connection := newRecoveringConnection(testReadCloser{strings.NewReader("{}\n" + frame + "\n")}, writer, 256)
	defer connection.Close()
	message, err := connection.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if request, ok := message.(*jsonrpc.Request); !ok || request.Method != "exact" {
		t.Fatalf("message=%#v", message)
	}
	if !strings.Contains(writer.String(), `"code":-32600`) {
		t.Fatalf("protocol output=%q", writer.String())
	}
}

func TestRecoveringConnectionCleanEOF(t *testing.T) {
	connection := newRecoveringConnection(testReadCloser{strings.NewReader("")}, &testWriteCloser{}, 256)
	defer connection.Close()
	if message, err := connection.Read(context.Background()); message != nil || err != io.EOF {
		t.Fatalf("message=%#v err=%v", message, err)
	}
}
