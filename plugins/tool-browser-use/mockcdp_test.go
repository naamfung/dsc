package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// mockCDPServer 是一个假 chromium：用最小 WebSocket 实现（RFC 6455）+ 只覆盖
// rod 建页/求值所需的最小 CDP 命令，供集成测试在不依赖本机 chromium 的情况下
// 跑通「插件 → rod → 浏览器 → 结果 → ViewFn → gRPC」完整链路。
type mockCDPServer struct {
	ln        net.Listener
	wsURL     string
	innerText string
}

// startMockCDP 启动 mock CDP 服务器并返回 ws 端点地址。
func startMockCDP(t *testing.T, innerText string) *mockCDPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock cdp listen: %v", err)
	}
	s := &mockCDPServer{ln: ln, wsURL: "ws://" + ln.Addr().String(), innerText: innerText}
	go s.acceptLoop()
	return s
}

func (s *mockCDPServer) close() { _ = s.ln.Close() }

func (s *mockCDPServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *mockCDPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	if err := wsHandshake(br, conn); err != nil {
		return
	}
	for {
		payload, opcode, err := wsReadFrame(br)
		if err != nil {
			return
		}
		switch opcode {
		case 0x8: // close
			_ = wsWriteFrame(conn, 0x8, payload)
			return
		case 0x9: // ping → pong
			_ = wsWriteFrame(conn, 0xA, payload)
		case 0x1: // text
			if resp := s.dispatch(payload); resp != nil {
				_ = wsWriteFrame(conn, 0x1, resp)
			}
		}
	}
}

// wsHandshake 完成 RFC 6455 服务端握手。
func wsHandshake(br *bufio.Reader, conn net.Conn) error {
	req, err := http.ReadRequest(br)
	if err != nil {
		return err
	}
	h := sha1.New()
	io.WriteString(h, req.Header.Get("Sec-WebSocket-Key")+"258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(h.Sum(nil)) + "\r\n\r\n"
	_, err = io.WriteString(conn, resp)
	return err
}

// wsReadFrame 读取一个 WebSocket 帧（客户端帧带掩码）。
func wsReadFrame(r io.Reader) (payload []byte, opcode byte, err error) {
	var b [2]byte
	if _, err = io.ReadFull(r, b[:]); err != nil {
		return nil, 0, err
	}
	opcode = b[0] & 0x0F
	length := uint64(b[1] & 0x7F)
	if length == 126 {
		var ext [2]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return nil, 0, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	} else if length == 127 {
		var ext [8]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return nil, 0, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	var mask [4]byte
	if b[1]&0x80 != 0 {
		if _, err = io.ReadFull(r, mask[:]); err != nil {
			return nil, 0, err
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return nil, 0, err
	}
	if b[1]&0x80 != 0 {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return payload, opcode, nil
}

// wsWriteFrame 写出一个服务端帧（服务端帧不掩码）。
func wsWriteFrame(conn net.Conn, opcode byte, payload []byte) error {
	n := len(payload)
	var hdr []byte
	hdr = append(hdr, 0x80|opcode)
	switch {
	case n <= 125:
		hdr = append(hdr, byte(n))
	case n <= 65535:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		hdr = append(hdr, l[:]...)
	}
	if _, err := conn.Write(hdr); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// dispatch 处理一条 CDP 命令并返回 JSON-RPC 响应（未知命令回空结果）。
func (s *mockCDPServer) dispatch(payload []byte) []byte {
	var req struct {
		ID     int            `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil
	}
	out, _ := json.Marshal(map[string]any{"id": req.ID, "result": s.cdpResult(req.Method, req.Params)})
	return out
}

func (s *mockCDPServer) cdpResult(method string, params map[string]any) map[string]any {
	switch method {
	case "Target.createTarget":
		return map[string]any{"targetId": "T-1"}
	case "Target.attachToTarget":
		return map[string]any{"sessionId": "S-1"}
	case "Page.navigate":
		return map[string]any{"frameId": "F-1", "loaderId": "L-1"}
	case "Runtime.evaluate":
		// rod 用 Runtime.evaluate{expression:"window"} 获取 JS 上下文
		return map[string]any{"result": map[string]any{"type": "object", "objectId": "O-1", "className": "Object"}}
	case "Runtime.callFunctionOn":
		fn, _ := params["functionDeclaration"].(string)
		switch {
		case strings.Contains(fn, "waitLoad"):
			return map[string]any{"result": map[string]any{"type": "undefined"}}
		case strings.Contains(fn, "innerText"):
			return map[string]any{"result": map[string]any{"type": "string", "value": s.innerText}}
		default:
			return map[string]any{"result": map[string]any{"type": "undefined"}}
		}
	default:
		// setDiscoverTargets / stopLoading / 各类 enable 等无结果命令
		return map[string]any{}
	}
}

// assertView 校验工具结果经完整 gRPC 链路透传的 ViewJson。
func assertView(t *testing.T, resp *proto.ExecuteToolResponse) core.ToolView {
	t.Helper()
	if resp.ViewJson == "" {
		t.Fatalf("ViewJson 为空（ViewFn 未生效或 gRPC 透传缺失）: %+v", resp)
	}
	var v core.ToolView
	if err := json.Unmarshal([]byte(resp.ViewJson), &v); err != nil {
		t.Fatalf("ViewJson 非法: %v", err)
	}
	if v.Kind == "" {
		t.Fatalf("ViewJson 缺 kind: %q", resp.ViewJson)
	}
	return v
}

// TestE2EWithMockChromium 以 mock CDP 服务器替代真实 chromium，验证
// fetch_url 全链路：插件经 rod 连接假浏览器 → 读取 innerText → 结果与
// ViewJson 穿透 gRPC 返回，全程不依赖本机 chromium / 网络。
func TestE2EWithMockChromium(t *testing.T) {
	// 1. 启动 mock CDP 服务器（假 chromium，innerText 恒定）
	mock := startMockCDP(t, "MOCK_PAGE_BODY_TEXT")
	defer mock.close()

	// 2. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-browser-use.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 3. 以 DSC_BROWSER_CDP_URL 指向 mock 拉起插件（跳过本地 chromium 启动）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "DSC_BROWSER_CDP_URL="+mock.wsURL)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  core.Handshake,
		Plugins:          map[string]plugin.Plugin{},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
	if !ok {
		t.Fatalf("unexpected client type %T", rpcClient)
	}
	conn := grpcClient.Conn
	ctx := context.Background()

	// 4. 元数据与工具目录
	meta := metadata.NewPluginMetadataClient(conn)
	if info, err := meta.GetInfo(ctx, &metadata.Empty{}); err != nil || info.Type != "tool" || info.Name != "browser-use" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}
	tc := proto.NewToolServiceClient(conn)
	if list, err := tc.ListTools(ctx, &proto.ListToolsRequest{}); err != nil || len(list.Tools) != 5 {
		t.Fatalf("ListTools = %+v, err %v", list, err)
	}

	// 5. 真实执行 fetch_url（user_mode=false 走隔离路径，经 mock 浏览器）
	resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName:      "fetch_url",
		ArgumentsJson: `{"url":"https://example.com","user_mode":false}`,
	})
	if err != nil {
		t.Fatalf("ExecuteTool(fetch_url): %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("fetch_url 应成功: %+v", resp)
	}
	if !strings.Contains(resp.Content, "MOCK_PAGE_BODY_TEXT") {
		t.Fatalf("fetch_url 应返回 mock innerText: %+v", resp)
	}
	// ViewJson 穿透完整链路：plain 视图 + ok 徽标
	if v := assertView(t, resp); v.Kind != "plain" || v.Title != "Fetch" || v.Badge == nil || v.Badge.Text != "ok" || !strings.Contains(v.Body, "MOCK_PAGE_BODY_TEXT") {
		t.Fatalf("fetch view = %+v", v)
	}
}
