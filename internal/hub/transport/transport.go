// Package transport provides hub-agent communication over WebSocket.
package transport

import (
	"context"
	"errors"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
)

type Transport interface {
	Request(ctx context.Context, action common.WebSocketAction, req any, dest any) error
	IsConnected() bool
	Close()
}

func UnmarshalResponse(resp common.AgentResponse, dest any) error {
	if dest == nil {
		return errors.New("nil destination")
	}
	if len(resp.Data) == 0 {
		return errors.New("empty response data")
	}
	return cbor.Unmarshal(resp.Data, dest)
}
