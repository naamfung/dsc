package main

import (
	"context"
	"fmt"

	"dsc/plugin"
)

// ExamplePlugin 插件業務實現
type ExamplePlugin struct{}

func (e *ExamplePlugin) Name(ctx context.Context) string {
	return "example"
}

func (e *ExamplePlugin) Version(ctx context.Context) string {
	return "1.0.0"
}

func (e *ExamplePlugin) Execute(ctx context.Context, req *plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	output := fmt.Sprintf("Example plugin processed: %s", req.Input)
	return &plugin.ExecuteResponse{
		Output:  output,
		Status:  "success",
		Message: "done",
	}, nil
}

func (e *ExamplePlugin) HealthCheck(ctx context.Context) error {
	return nil
}