package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
)

// RemoteTool 實現了 ToolDefinition，通過 gRPC 調用遠程工具
type RemoteTool struct {
	name        string
	description string
	schema      json.RawMessage
	client      proto.ToolServiceClient
}

func (r *RemoteTool) Name() string {
	return r.name
}

func (r *RemoteTool) Description() string {
	return r.description
}

func (r *RemoteTool) ParametersSchema() json.RawMessage {
	return r.schema
}

func (r *RemoteTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	resp, err := r.client.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName:      r.name,
		ArgumentsJson: string(args),
	})
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Content, nil
}
