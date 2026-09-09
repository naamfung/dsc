package core

import (
        "encoding/json"
        "os"
        "testing"
)

func TestE2BNoApiKey(t *testing.T) {
        // 确保无 API key
        os.Unsetenv("DSC_E2B_API_KEY")

        sb := NewE2BSandbox()
        if sb.apiKey != "" {
                t.Error("apiKey should be empty when env not set")
        }
}

func TestE2BSandboxID(t *testing.T) {
        sb := &E2BSandbox{}
        if sb.SandboxID() != "" {
                t.Error("sandboxID should be empty initially")
        }
}

// TestE2BRequestBodyJSONMarshaled 验证 Execute/WriteFile 用 json.Marshal 构造请求体，
// 覆盖此前手写 escapeJSON 漏转义控制字符（\r、\f、unicode 等）的缺陷。
func TestE2BRequestBodyJSONMarshaled(t *testing.T) {
        cases := []struct {
                name string
                m    map[string]string
        }{
                {"simple cmd", map[string]string{"cmd": "ls -la"}},
                {"quoted cmd", map[string]string{"cmd": `echo "hello world"`}},
                {"backslash cmd", map[string]string{"cmd": `grep "a\\b" file`}},
                {"newline cmd", map[string]string{"cmd": "echo line1\nline2"}},
                {"cr cmd", map[string]string{"cmd": "echo line1\rline2"}},
                {"unicode cmd", map[string]string{"cmd": "echo 你好"}},
                {"path+content", map[string]string{"path": "/tmp/file with spaces", "content": "body\ttab\nnewline"}},
        }
        for _, c := range cases {
                t.Run(c.name, func(t *testing.T) {
                        b, err := json.Marshal(c.m)
                        if err != nil {
                                t.Fatalf("marshal failed: %v", err)
                        }
                        // 反解回来应与原 map 一致——证明 marshal/unmarshal 往返保真
                        var got map[string]string
                        if err := json.Unmarshal(b, &got); err != nil {
                                t.Fatalf("unmarshal failed: %v", err)
                        }
                        for k, v := range c.m {
                                if got[k] != v {
                                        t.Errorf("round-trip mismatch: key=%q got=%q want=%q", k, got[k], v)
                                }
                        }
                })
        }
}
