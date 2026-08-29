package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"dsc-sdk"
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

	// ---- 精確整數運算 mod/quot/rem ----
	// 整數參數（int 或分母為 1 的 Rat）走 big.Int 精確計算；浮點參數回退 float64。
	_ = e.Set(types.Symbol{Val: "mod"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("mod expects 2 arguments, got %d", len(args))
		}
		if a, ok := toInt64(args[0]); ok {
			if b, ok := toInt64(args[1]); ok {
				if b == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				// Clojure mod：結果符號跟隨除數（floor 語義）
				ba, bb := big.NewInt(a), big.NewInt(b)
				m := new(big.Int).Rem(ba, bb)
				if m.Sign() != 0 && ((m.Sign() < 0) != (b < 0)) {
					m.Add(m, bb)
				}
				return int(m.Int64()), nil
			}
		}
		return math.Mod(toFloat64(args[0]), toFloat64(args[1])), nil
	}})
	_ = e.Set(types.Symbol{Val: "quot"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("quot expects 2 arguments, got %d", len(args))
		}
		if a, ok := toInt64(args[0]); ok {
			if b, ok := toInt64(args[1]); ok {
				if b == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				return int(new(big.Int).Quo(big.NewInt(a), big.NewInt(b)).Int64()), nil
			}
		}
		return math.Trunc(toFloat64(args[0]) / toFloat64(args[1])), nil
	}})
	_ = e.Set(types.Symbol{Val: "rem"}, types.Func{Fn: func(_ context.Context, args []types.MalType) (types.MalType, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("rem expects 2 arguments, got %d", len(args))
		}
		if a, ok := toInt64(args[0]); ok {
			if b, ok := toInt64(args[1]); ok {
				if b == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				return int(new(big.Int).Rem(big.NewInt(a), big.NewInt(b)).Int64()), nil
			}
		}
		return math.Remainder(toFloat64(args[0]), toFloat64(args[1])), nil
	}})

	// ---- 雙參數數學函數 ----
	_ = e.Set(types.Symbol{Val: "pow"}, types.Func{Fn: makeMathFunc2(math.Pow)})
	_ = e.Set(types.Symbol{Val: "atan2"}, types.Func{Fn: makeMathFunc2(math.Atan2)})
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
// 支持 int, float64, float32（jig/lisp reader 將浮點字面量解析為 float32）與精確 Rat
func toFloat64(v types.MalType) float64 {
	switch v := v.(type) {
	case int:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	case types.Rat:
		f, _ := v.R.Float64()
		return f
	case *types.Rat:
		f, _ := v.R.Float64()
		return f
	case bool:
		if v {
			return 1.0
		}
		return 0.0
	default:
		return 0.0
	}
}

// toInt64 將 MalType 轉換為 int64；僅接受原生 int 或分母為 1 的 Rat（即整數值）
func toInt64(v types.MalType) (int64, bool) {
	switch v := v.(type) {
	case int:
		return int64(v), true
	case types.Rat:
		if v.R.IsInt() {
			if i := v.R.Num(); i.IsInt64() {
				return i.Int64(), true
			}
		}
	case *types.Rat:
		if v.R.IsInt() {
			if i := v.R.Num(); i.IsInt64() {
				return i.Int64(), true
			}
		}
	}
	return 0, false
}

// ============================================================
// 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService /
// PluginMetadata / PluginHookService 与 go-core 组装（重写自旧的
// ToolServiceServer/MetadataServer/ToolMetadataGRPCPlugin 样板）。
// ============================================================
func main() {
	// 定義 lisp_eval 工具的元数据与处理器
	name := "lisp_eval"
	description := "Evaluate a Lisp/Scheme expression with exact rational arithmetic (Clojure-dialect interpreter). + - * / are exact and variadic: (/ 10 3) = 10/3, 3/4 literals (no spaces) are exact fractions, integral results print as ints, arbitrary-precision integers supported. Supported: exact + - * /, comparisons < <= > >= =, mod/quot/rem, list helpers filter range sum product reverse last; float escape hatch f+ f- f* f/ and math functions sqrt sin cos tan exp log pow min max (floats are NOT accepted by the exact + - * /). Do NOT quote the expression: pass (+ 1 2), not '(+ 1 2)."
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"expression": {
				"type": "string",
				"description": "Lisp/Scheme expression to evaluate, e.g. (+ 1 2), (* 3 4), (/ 10 3), (sqrt 2), (sum (range 100))"
			}
		},
		"required": ["expression"]
	}`)
	handler := func(ctx context.Context, args json.RawMessage) (string, error) {
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
	}

	sdk := dsc.New(dsc.Config{Name: "lisp-eval", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{Name: name, Description: description, Schema: schema, Handler: handler})
	sdk.Serve()
}
