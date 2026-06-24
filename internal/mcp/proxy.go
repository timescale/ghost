package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/config"
)

// isMethodNotFoundError checks if the error is a JSON-RPC "Method not found" error.
//
// This implementation uses string matching as a pragmatic workaround for several limitations:
//
//  1. The actual error type is github.com/modelcontextprotocol/go-sdk/internal/jsonrpc2.WireError,
//     which is in an internal package that we cannot import.
//
//  2. The WireError type has an Is() method that only matches against other WireError instances
//     with the same code, so we can't create our own type that will match via errors.Is().
//
//  3. The MCP SDK doesn't export ErrMethodNotFound or provide a way to create WireError
//     instances with arbitrary codes. It only exports specific constructors like
//     ResourceNotFoundError() and a few error codes, but not the -32601 code we need.
//
//  4. Using errors.As() with a local struct that has the same shape as WireError doesn't work
//     because errors.As requires type identity, not structural compatibility.
//
// 5. Using reflection to access the Code field would work but adds complexity and runtime overhead.
//
// Therefore, we use string matching on the error message. This is brittle but works for our
// specific use case where we're checking for unsupported resources/resource_templates methods.
// The error chain looks like:
//   - "failed to list resources from remote server: calling \"resources/list\": Method not found"
//   - "failed to list resource templates from remote server: calling \"resources/templates/list\": Method not found"
func isMethodNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for the specific "Method not found" JSON-RPC error
	return strings.Contains(errStr, "Method not found")
}

// registerDocsProxy establishes a connection to the remote docs MCP server and
// registers all tools, resources, resource templates, and prompts exposed by
// the server. Does not connect if the docs MCP server is disabled in the
// config or there is no URL in the config.
func (s *Server) registerDocsProxy(ctx context.Context) {
	cfg := s.app.GetConfig()
	if cfg == nil || cfg.DocsMCPURL == "" {
		return
	}

	// Create timeout for establishing proxy
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	proxyClient, err := NewProxyClient(ctx, cfg.DocsMCPURL, s.logger)
	if err != nil {
		return
	}
	s.docsProxyClient = proxyClient

	if err := proxyClient.RegisterTools(ctx, s.mcpServer); err != nil && !isMethodNotFoundError(err) {
		s.logger.Error("Failed to register tools from docs MCP server",
			slog.String("error", err.Error()),
		)
	}

	if err := proxyClient.RegisterResources(ctx, s.mcpServer); err != nil && !isMethodNotFoundError(err) {
		s.logger.Error("Failed to register resources from docs MCP server",
			slog.String("error", err.Error()),
		)
	}

	if err := proxyClient.RegisterResourceTemplates(ctx, s.mcpServer); err != nil && !isMethodNotFoundError(err) {
		s.logger.Error("Failed to register resource templates from docs MCP server",
			slog.String("error", err.Error()),
		)
	}

	if err := proxyClient.RegisterPrompts(ctx, s.mcpServer); err != nil && !isMethodNotFoundError(err) {
		s.logger.Error("Failed to register resource prompts from docs MCP server",
			slog.String("error", err.Error()),
		)
	}
}

// ProxyClient manages connection to a remote MCP server and forwards requests
type ProxyClient struct {
	url     string
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewProxyClient creates a new proxy client for the given remote server configuration
func NewProxyClient(ctx context.Context, url string, logger *slog.Logger) (*ProxyClient, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: api.HTTPClient,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "ghost-mcp-proxy-client",
		Title:   "Ghost MCP Proxy Client",
		Version: config.Version,
	}, &mcp.ClientOptions{
		Logger: logger,
	})

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote MCP server: %w", err)
	}

	return &ProxyClient{
		url:     url,
		client:  client,
		session: session,
	}, nil
}

// RegisterTools discovers tools from remote server and registers them as proxy tools
func (p *ProxyClient) RegisterTools(ctx context.Context, server *mcp.Server) error {
	if p.session == nil {
		return errors.New("not connected to remote server")
	}

	// List tools from remote server
	toolsResp, err := p.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("failed to list tools from remote server: %w", err)
	}

	if toolsResp == nil || len(toolsResp.Tools) == 0 {
		return nil
	}

	// Register each remote tool as a proxy tool
	for _, tool := range toolsResp.Tools {
		if tool.Name == "" {
			continue
		}

		// Create handler that forwards tool calls to remote server
		handler := p.createProxyToolHandler()

		// Register the proxy tool with our MCP server
		server.AddTool(tool, handler)
	}

	return nil
}

// createProxyToolHandler creates a handler function that forwards tool calls to the remote server
func (p *ProxyClient) createProxyToolHandler() mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if p.session == nil {
			return nil, errors.New("not connected to remote MCP server")
		}

		// Forward the request to remote server with original tool name
		params := &mcp.CallToolParams{
			Meta:      req.Params.Meta,
			Name:      req.Params.Name,
			Arguments: req.Params.Arguments,
		}

		// Call remote tool
		result, err := p.session.CallTool(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("remote tool call failed: %w", err)
		}

		return result, nil
	}
}

// RegisterResources discovers resources from remote server and registers them
// as proxy resources
func (p *ProxyClient) RegisterResources(ctx context.Context, server *mcp.Server) error {
	if p.session == nil {
		return errors.New("not connected to remote server")
	}

	// List resources from remote server
	resourcesResp, err := p.session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		return fmt.Errorf("failed to list resources from remote server: %w", err)
	}

	if resourcesResp == nil || len(resourcesResp.Resources) == 0 {
		return nil
	}

	// Register each remote resource as a proxy resource
	for _, resource := range resourcesResp.Resources {
		if resource.URI == "" {
			continue
		}

		// Create handler that forwards resource reads to remote server
		handler := p.createProxyResourceHandler()

		// Register the proxy resource with our MCP server
		server.AddResource(resource, handler)
	}

	return nil
}

// RegisterResourceTemplates discovers resource templates from remote server and registers them as proxy resource templates
func (p *ProxyClient) RegisterResourceTemplates(ctx context.Context, server *mcp.Server) error {
	if p.session == nil {
		return errors.New("not connected to remote server")
	}

	// List resource templates from remote server
	templatesResp, err := p.session.ListResourceTemplates(ctx, &mcp.ListResourceTemplatesParams{})
	if err != nil {
		return fmt.Errorf("failed to list resource templates from remote server: %w", err)
	}

	if templatesResp == nil || len(templatesResp.ResourceTemplates) == 0 {
		return nil
	}

	// Register each remote resource template as a proxy resource template
	for _, resourceTemplate := range templatesResp.ResourceTemplates {
		if resourceTemplate.URITemplate == "" {
			continue
		}

		// Create handler that forwards resource template reads to remote server
		handler := p.createProxyResourceHandler()

		// Register the proxy resource template with our MCP server
		server.AddResourceTemplate(resourceTemplate, handler)
	}

	return nil
}

// createProxyResourceHandler creates a handler function that forwards resource reads to the remote server
func (p *ProxyClient) createProxyResourceHandler() mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if p.session == nil {
			return nil, errors.New("not connected to remote MCP server")
		}

		// Call remote resource
		result, err := p.session.ReadResource(ctx, req.Params)
		if err != nil {
			return nil, fmt.Errorf("remote resource read failed: %w", err)
		}

		return result, nil
	}
}

// RegisterPrompts discovers prompts from remote server and registers them as proxy prompts
func (p *ProxyClient) RegisterPrompts(ctx context.Context, server *mcp.Server) error {
	if p.session == nil {
		return errors.New("not connected to remote server")
	}

	// List prompts from remote server
	promptsResp, err := p.session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		return fmt.Errorf("failed to list prompts from remote server: %w", err)
	}

	if promptsResp == nil || len(promptsResp.Prompts) == 0 {
		return nil
	}

	// Register each remote prompt as a proxy prompt
	for _, prompt := range promptsResp.Prompts {
		if prompt.Name == "" {
			continue
		}

		// Create handler that forwards prompt requests to remote server
		handler := p.createProxyPromptHandler()

		// Register the proxy prompt with our MCP server
		server.AddPrompt(prompt, handler)
	}

	return nil
}

// createProxyPromptHandler creates a handler function that forwards prompt requests to the remote server
func (p *ProxyClient) createProxyPromptHandler() mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		if p.session == nil {
			return nil, errors.New("not connected to remote MCP server")
		}

		// Call remote prompt
		result, err := p.session.GetPrompt(ctx, req.Params)
		if err != nil {
			return nil, fmt.Errorf("remote prompt request failed: %w", err)
		}

		return result, nil
	}
}

// Close closes the connection to the remote MCP server
func (p *ProxyClient) Close() error {
	if p != nil && p.session != nil {
		return p.session.Close()
	}
	return nil
}
