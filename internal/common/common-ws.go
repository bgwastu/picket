package common

import (
	"github.com/fxamacker/cbor/v2"
)

type WebSocketAction = uint8

const (
	// Request system data from agent
	GetData WebSocketAction = iota
	// Request container logs from agent
	GetContainerLogs
	// Request container info from agent
	GetContainerInfo
	// UninstallAgent removes a Picket systemd installation from the host.
	UninstallAgent
	// SSHStream carries a framed bidirectional SSH tunnel.
	SSHStream
)

const (
	SSHStreamOpen uint8 = iota
	SSHStreamOpenOK
	SSHStreamOpenError
	SSHStreamData
	SSHStreamEOF
	SSHStreamClose
)

const SSHStreamMagic uint8 = 0x53

// SSHStreamMessage is used for the raw byte stream. Data is always bounded by
// the transport implementation and must not be accumulated without a limit.
type SSHStreamMessage struct {
	Magic    uint8  `cbor:"4,keyasint"`
	StreamID uint32 `cbor:"0,keyasint"`
	Type     uint8  `cbor:"1,keyasint"`
	Data     []byte `cbor:"2,keyasint,omitempty"`
	Error    string `cbor:"3,keyasint,omitempty"`
}

// HubRequest defines the structure for requests sent from hub to agent.
type HubRequest[T any] struct {
	Action WebSocketAction `cbor:"0,keyasint"`
	Data   T               `cbor:"1,keyasint,omitempty,omitzero"`
	Id     *uint32         `cbor:"2,keyasint,omitempty"`
}

// AgentResponse defines the structure for responses sent from agent to hub.
type AgentResponse struct {
	Id    *uint32         `cbor:"0,keyasint,omitempty"`
	Error string          `cbor:"1,keyasint,omitempty,omitzero"`
	Data  cbor.RawMessage `cbor:"2,keyasint,omitempty,omitzero"`
}

type DataRequestOptions struct {
	CacheTimeMs    uint16 `cbor:"0,keyasint"`
	IncludeDetails bool   `cbor:"1,keyasint"`
}

type ContainerLogsRequest struct {
	ContainerID string `cbor:"0,keyasint"`
}

type ContainerInfoRequest struct {
	ContainerID string `cbor:"0,keyasint"`
}

type UninstallAgentResponse struct {
	Uninstalled bool   `cbor:"0,keyasint"`
	Message     string `cbor:"1,keyasint,omitempty"`
}
