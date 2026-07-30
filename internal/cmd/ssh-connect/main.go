package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"

	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/websocket"
	"github.com/henrygd/beszel/internal/common"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "proxy" {
		fmt.Fprintln(os.Stderr, "usage: picket-connect proxy --hub URL --token TOKEN")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("proxy", flag.ExitOnError)
	hub := flags.String("hub", "", "Picket hub URL")
	token := flags.String("token", "", "one-time SSH launch token")
	_ = flags.Parse(os.Args[2:])
	if *hub == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "--hub and --token are required")
		os.Exit(2)
	}
	if err := proxy(*hub, *token); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func proxy(hub, token string) error {
	u, err := url.Parse(hub)
	if err != nil {
		return fmt.Errorf("invalid hub URL: %w", err)
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = path.Join(u.Path, "/api/picket/ssh-connect")
	query := u.Query()
	query.Set("token", token)
	u.RawQuery = query.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("connect to Picket: %w", err)
	}
	defer conn.Close()

	if err := writeFrame(conn, &common.SSHStreamMessage{Magic: common.SSHStreamMagic, StreamID: 1, Type: common.SSHStreamOpen}); err != nil {
		return err
	}
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if sendErr := writeFrame(conn, &common.SSHStreamMessage{Magic: common.SSHStreamMagic, StreamID: 1, Type: common.SSHStreamData, Data: append([]byte(nil), buf[:n]...)}); sendErr != nil {
					readErr <- sendErr
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					_ = writeFrame(conn, &common.SSHStreamMessage{Magic: common.SSHStreamMagic, StreamID: 1, Type: common.SSHStreamEOF})
				}
				readErr <- err
				return
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case inputErr := <-readErr:
				if inputErr != io.EOF {
					return inputErr
				}
			default:
			}
			return err
		}
		var request common.HubRequest[cbor.RawMessage]
		if err := cbor.Unmarshal(data, &request); err != nil || request.Action != common.SSHStream {
			continue
		}
		var msg common.SSHStreamMessage
		if err := cbor.Unmarshal(request.Data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case common.SSHStreamData:
			if _, err := os.Stdout.Write(msg.Data); err != nil {
				return err
			}
		case common.SSHStreamOpenError:
			return fmt.Errorf("Picket SSH tunnel: %s", msg.Error)
		case common.SSHStreamEOF, common.SSHStreamClose:
			return nil
		}
	}
}

func writeFrame(conn *websocket.Conn, msg *common.SSHStreamMessage) error {
	streamData, err := cbor.Marshal(msg)
	if err != nil {
		return err
	}
	data, err := cbor.Marshal(common.HubRequest[cbor.RawMessage]{Action: common.SSHStream, Data: streamData})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}
