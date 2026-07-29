package agent

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
)

func newAgentResponse(data any, requestID *uint32) common.AgentResponse {
	response := common.AgentResponse{Id: requestID}
	response.Data, _ = cbor.Marshal(data)
	return response
}
