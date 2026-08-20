package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent/nsconcurrent"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/lib/coreextented/nscoreextended"
	"github.com/jig/lisp/types"
)

// ============================================================
// Lisp/Scheme 精確計算工具實現
// ============================================================

// LispEvalTool Lisp 計算工具實現
type LispEvalTool struct {
	name        string
	description string
	schema      json.RawMessage
	handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

func (l *LispEvalTool) Name() string {
	return l.name
}

func (l *LispEvalTool) Description() string {
	return l.description
}

func (l *LispEvalTool) ParametersSchema() json.RawMessage {
	return l.schema
}

func (l *LispEvalTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return l.handler(ctx, args)
}

// schemeEval 執行 Clojure/Lisp 表達式並返回結果字符串
// 使用 jig/lisp 庫（純 Go 實現，Clojure 方言）
func schemeEval(ctx context.Context, expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("expression is empty")
	}

	// 創建 Lisp 環境並加載核心函數
	e := env.NewEnv()
	if err := nscore.Load(e); err != nil {
		return "", fmt.Errorf("failed to load core: %v", err)
	}

	// 加載並發原語 (atom, swap!, reset! 等，coreextented 依賴)
	if err := nsconcurrent.Load(e); err != nil {
		return "", fmt.Errorf("failed to load concurrent: %v", err)
	}

	// 加載擴展庫 (reduce, inc, dec, zero?, identity, gensym 等)
	if err := nscoreextended.Load(e); err != nil {
		return "", fmt.Errorf("failed to load core-ext: %v", err)
	}

	// 註冊擴展數學函數和常量（覆蓋核心 +, -, *, / 為可變參數版本）
	registerMathFuncs(e)

	// 加載常用 Lisp 函數擴展（filter, range, even?, odd? 等）
	if _, err := lisp.REPL(context.Background(), e, lispPreamble(), nil); err != nil {
		return "", fmt.Errorf("failed to load preamble: %v", err)
	}

	// 始終包裝在 (do ...) 中以支持多表達式輸入
	// REPL 的 READ 只讀一個 form，do 塊可順序執行多個 form 並返回最後一個結果
	wrappedExpr := "(do\n" + expr + "\n)"

	// 60 秒超時
	evalCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	startTime := time.Now()
	result, err := lisp.REPL(evalCtx, e, wrappedExpr, nil)
	elapsed := time.Since(startTime)

	if err != nil {
		return "", err
	}

	// REPL 內部調用 PRINT 返回 string 類型的 MalType
	resultStr, _ := result.(string)
	output := fmt.Sprintf("%s\n\n[耗時 %v]", resultStr, elapsed.Round(time.Millisecond))
	return output, nil
}

// lispPreamble 返回常用 Lisp 擴展函數定義
func lispPreamble() string {
	return `(do
  ;; 謂詞函數
  (defn even? [n] (= 0 (mod n 2)))
  (defn odd? [n] (not (= 0 (mod n 2))))
  (defn pos? [n] (> n 0))
  (defn neg? [n] (< n 0))
  (defn zero? [n] (= 0 n))

  ;; 列表函數
  ;; 注意: jig/lisp 的 cons 不接受 nil 作為第二個參數，需使用 '() 代替
  (defn filter [pred xs]
    (if (empty? xs) '()
      (if (pred (first xs))
        (cons (first xs) (filter pred (rest xs)))
        (filter pred (rest xs)))))

  (defn range [n]
    (if (<= n 0) '()
      (let [r (range (dec n))]
        (cons (dec n) (if (empty? r) '() r)))))

  (defn reverse [xs]
    (defn rev-helper [xs acc]
      (if (empty? xs) acc
        (rev-helper (rest xs) (cons (first xs) (if (empty? acc) '() acc)))))
    (rev-helper xs '()))

  (defn last [xs]
    (if (empty? (rest xs)) (first xs)
      (last (rest xs))))

  (defn butlast [xs]
    (reverse (rest (reverse xs))))

  (defn take-while [pred xs]
    (if (or (empty? xs) (not (pred (first xs)))) '()
      (cons (first xs) (take-while pred (rest xs)))))

  (defn drop-while [pred xs]
    (if (or (empty? xs) (not (pred (first xs)))) xs
      (drop-while pred (rest xs))))

  ;; 求和 / 求積
  (defn sum [xs] (reduce + 0 xs))
  (defn product [xs] (reduce * 1 xs))

  ;; 字符串處理
  (defn str-join [sep xs]
    (if (empty? xs) ""
      (if (= 1 (count xs)) (str (first xs))
        (str (first xs) sep (str-join sep (rest xs))))))

  (defn apply-fn [f args] (apply f args))
)`
}

// registerMathFuncs 註冊擴展數學常量和函數到 Lisp 環境
// 覆蓋核心的二元 +, -, *, / 為可變參數版本，並補充浮點運算和數學函數
func registerMathFuncs(e types.EnvType) {
	// ---- 數學常量 ----
	_ = e.Set(types.Symbol{Val: "PI"}, 3.141592653589793)
	_ = e.Set(types.Symbol{Val: "E"}, 2.718281828459045)

	// ---- 可變參數整數算術（覆蓋核心二元版本）----
	_ = e.Set(types.Symbol{Val: "+"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		result := 0
		for _, a := range args {
			result += toInt(a)
		}
		return result, nil
	}})
	_ = e.Set(types.Symbol{Val: "-"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) == 0 {
			return 0, nil
		}
		result := toInt(args[0])
		for _, a := range args[1:] {
			result -= toInt(a)
		}
		return result, nil
	}})
	_ = e.Set(types.Symbol{Val: "*"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		result := 1
		for _, a := range args {
			result *= toInt(a)
		}
		return result, nil
	}})
	_ = e.Set(types.Symbol{Val: "/"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) == 0 {
			return 0, nil
		}
		result := toInt(args[0])
		for _, a := range args[1:] {
			d := toInt(a)
			if d == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			result /= d
		}
		return result, nil
	}})

	// ---- 浮點算術（f+ f- f* f/）----
	_ = e.Set(types.Symbol{Val: "f+"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		result := 0.0
		for _, a := range args {
			result += toFloat64(a)
		}
		return result, nil
	}})
	_ = e.Set(types.Symbol{Val: "f-"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) == 0 {
			return 0.0, nil
		}
		result := toFloat64(args[0])
		for _, a := range args[1:] {
			result -= toFloat64(a)
		}
		return result, nil
	}})
	_ = e.Set(types.Symbol{Val: "f*"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		result := 1.0
		for _, a := range args {
			result *= toFloat64(a)
		}
		return result, nil
	}})
	_ = e.Set(types.Symbol{Val: "f/"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) == 0 {
			return 0.0, nil
		}
		result := toFloat64(args[0])
		for _, a := range args[1:] {
			d := toFloat64(a)
			if d == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			result /= d
		}
		return result, nil
	}})

	// ---- 單參數數學函數 ----
	_ = e.Set(types.Symbol{Val: "sqrt"}, types.Func{Fn: makeMathFunc1(math.Sqrt)})
	_ = e.Set(types.Symbol{Val: "abs"}, types.Func{Fn: makeMathFunc1(math.Abs)})
	_ = e.Set(types.Symbol{Val: "floor"}, types.Func{Fn: makeMathFunc1(math.Floor)})
	_ = e.Set(types.Symbol{Val: "ceil"}, types.Func{Fn: makeMathFunc1(math.Ceil)})
	_ = e.Set(types.Symbol{Val: "round"}, types.Func{Fn: makeMathFunc1(math.Round)})
	_ = e.Set(types.Symbol{Val: "log"}, types.Func{Fn: makeMathFunc1(math.Log)})
	_ = e.Set(types.Symbol{Val: "log10"}, types.Func{Fn: makeMathFunc1(math.Log10)})
	_ = e.Set(types.Symbol{Val: "log2"}, types.Func{Fn: makeMathFunc1(math.Log2)})
	_ = e.Set(types.Symbol{Val: "exp"}, types.Func{Fn: makeMathFunc1(math.Exp)})
	_ = e.Set(types.Symbol{Val: "sin"}, types.Func{Fn: makeMathFunc1(math.Sin)})
	_ = e.Set(types.Symbol{Val: "cos"}, types.Func{Fn: makeMathFunc1(math.Cos)})
	_ = e.Set(types.Symbol{Val: "tan"}, types.Func{Fn: makeMathFunc1(math.Tan)})
	_ = e.Set(types.Symbol{Val: "asin"}, types.Func{Fn: makeMathFunc1(math.Asin)})
	_ = e.Set(types.Symbol{Val: "acos"}, types.Func{Fn: makeMathFunc1(math.Acos)})
	_ = e.Set(types.Symbol{Val: "atan"}, types.Func{Fn: makeMathFunc1(math.Atan)})

	// ---- 雙參數數學函數 ----
	_ = e.Set(types.Symbol{Val: "pow"}, types.Func{Fn: makeMathFunc2(math.Pow)})
	_ = e.Set(types.Symbol{Val: "atan2"}, types.Func{Fn: makeMathFunc2(math.Atan2)})
	_ = e.Set(types.Symbol{Val: "mod"}, types.Func{Fn: makeMathFunc2(math.Mod)})
	_ = e.Set(types.Symbol{Val: "remainder"}, types.Func{Fn: makeMathFunc2(math.Remainder)})

	// ---- 可變參數數學函數（至少 1 個參數）----
	// Clojure 語義：min/max 接受任意數量參數，如 (max 1 5 3) -> 5
	_ = e.Set(types.Symbol{Val: "min"}, types.Func{Fn: makeMathNaryFunc("min", math.Min)})
	_ = e.Set(types.Symbol{Val: "max"}, types.Func{Fn: makeMathNaryFunc("max", math.Max)})
}

// makeMathFunc1 創建單參數數學函數的 types.Func 包裝（float64 輸入輸出）
func makeMathFunc1(fn func(float64) float64) types.ExternalCall {
	return func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 argument, got %d", len(args))
		}
		return fn(toFloat64(args[0])), nil
	}
}

// makeMathFunc2 創建雙參數數學函數的 types.Func 包裝（float64 輸入輸出）
func makeMathFunc2(fn func(float64, float64) float64) types.ExternalCall {
	return func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("expected 2 arguments, got %d", len(args))
		}
		return fn(toFloat64(args[0]), toFloat64(args[1])), nil
	}
}

// makeMathNaryFunc 創建可變參數數學函數的 types.Func 包裝（至少 1 個參數，float64 輸入輸出）
// 用於 Clojure 語義下可接收任意數量參數的 min/max 等函數，如 (max 1 5 3) -> 5
func makeMathNaryFunc(name string, fn func(float64, float64) float64) types.ExternalCall {
	return func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("%s expects at least 1 argument, got %d", name, len(args))
		}
		result := toFloat64(args[0])
		for _, a := range args[1:] {
			result = fn(result, toFloat64(a))
		}
		return result, nil
	}
}

// toFloat64 將 MalType 轉換為 float64
// 支持 int, float64, float32（jig/lisp reader 將浮點字面量解析為 float32）
func toFloat64(v types.MalType) float64 {
	switch v := v.(type) {
	case int:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	case bool:
		if v {
			return 1.0
		}
		return 0.0
	default:
		return 0.0
	}
}

// toInt 將 MalType 轉換為 int
func toInt(v types.MalType) int {
	switch v := v.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case float32:
		return int(v)
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// ============================================================
// 插件服務實現
// ============================================================

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	tools []*LispEvalTool
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	for _, t := range s.tools {
		if t.Name() == req.ToolName {
			res, err := t.Execute(ctx, json.RawMessage(req.ArgumentsJson))
			if err != nil {
				return &proto.ExecuteToolResponse{Error: err.Error()}, nil
			}
			return &proto.ExecuteToolResponse{Content: res}, nil
		}
	}
	return &proto.ExecuteToolResponse{Error: "tool not found"}, nil
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	var tools []*proto.Tool
	for _, t := range s.tools {
		tools = append(tools, &proto.Tool{
			Name:           t.Name(),
			Description:    t.Description(),
			ParametersJson: string(t.ParametersSchema()),
		})
	}
	return &proto.ListToolsResponse{Tools: tools}, nil
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "tool",
		Name:       "lisp-eval",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

func main() {
	// 定義 lisp_eval 工具
	lispEvalTool := &LispEvalTool{
		name:        "lisp_eval",
		description: "Evaluate a Lisp/Scheme expression with exact integer arithmetic (Clojure-dialect interpreter). Supported: integer + - * /, float f+ f- f* f/, math functions sqrt abs floor ceil round log exp sin cos tan asin acos atan pow min max mod, and list helpers filter range sum product reverse last. NOT supported: rationals (e.g. 3/4) and big integers (e.g. biginteger) — use plain integers or floats instead. Do NOT quote the expression: pass (+ 1 2), not '(+ 1 2).",
		schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"expression": {
					"type": "string",
					"description": "Lisp/Scheme expression to evaluate, e.g. (+ 1 2), (* 3 4), (/ 10 3), (sqrt 2), (sum (range 100))"
				}
			},
			"required": ["expression"]
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if strings.TrimSpace(params.Expression) == "" {
				return "", fmt.Errorf("expression is required")
			}

			result, err := schemeEval(ctx, params.Expression)
			if err != nil {
				return "", err
			}
			return result, nil
		},
	}

	// 創建工具服務服務端
	toolServer := &ToolServiceServer{
		tools: []*LispEvalTool{lispEvalTool},
	}

	// 創建元數據服務服務端
	metadataServer := &MetadataServer{}

	// 啟動插件服務
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolMetadataGRPCPlugin{
				ToolImpl:     toolServer,
				MetadataImpl: metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}

// ToolMetadataGRPCPlugin 是 gRPC 插件的實現
type ToolMetadataGRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	ToolImpl     proto.ToolServiceServer
	MetadataImpl metadata.PluginMetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &ToolMetadataGRPCClient{
		ToolClient:     proto.NewToolServiceClient(c),
		MetadataClient: metadata.NewPluginMetadataClient(c),
	}, nil
}

type ToolMetadataGRPCClient struct {
	ToolClient     proto.ToolServiceClient
	MetadataClient metadata.PluginMetadataClient
}

func (c *ToolMetadataGRPCClient) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	return c.ToolClient.ExecuteTool(ctx, req, opts...)
}

func (c *ToolMetadataGRPCClient) ListTools(ctx context.Context, req *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return c.ToolClient.ListTools(ctx, req, opts...)
}

func (c *ToolMetadataGRPCClient) GetInfo(ctx context.Context, req *metadata.Empty, opts ...grpc.CallOption) (*metadata.PluginInfo, error) {
	return c.MetadataClient.GetInfo(ctx, req, opts...)
}
