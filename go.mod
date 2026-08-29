module dsc

go 1.26.0

require (
	charm.land/bubbles/v2 v2.1.0
	charm.land/bubbletea/v2 v2.0.7
	charm.land/lipgloss/v2 v2.0.4
	github.com/atotto/clipboard v0.1.4
	github.com/casbin/casbin v1.9.1
	github.com/charmbracelet/x/ansi v0.11.7
	github.com/flosch/pongo2/v6 v6.1.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/garyburd/redigo v1.6.4
	github.com/go-playground/validator/v10 v10.30.3
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-plugin v1.8.0
	github.com/hashicorp/go-version v1.9.0
	github.com/mattn/go-colorable v0.1.14
	github.com/mattn/go-isatty v0.0.20
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/robfig/cron/v3 v3.0.1
	github.com/smartystreets/goconvey v1.8.1
	github.com/stretchr/testify v1.11.1
	github.com/toon-format/toon-go v0.0.0
	github.com/valyala/fasttemplate v1.2.2
	github.com/wippyai/go-lua v1.5.17
	github.com/yuin/goldmark v1.8.5
	github.com/zeebo/blake3 v0.2.4
	golang.org/x/text v0.37.0
	golang.org/x/time v0.15.0
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/vmihailenco/msgpack.v2 v2.9.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Knetic/govaluate v3.0.1-0.20171022003610-9aa49832a739+incompatible // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260525132238-948f4557a654 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-runewidth v0.0.23 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/smarty/assertions v1.15.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/exp v0.0.0-20250218142911-aa4b98e5adaa // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

// 定制版 go-plugin（plugin/，含宿主挂载聚合服务必需的 Broker 扩展）
replace github.com/hashicorp/go-plugin => ./plugin

// LLM SDK 改用本地版本（libs/），避免外部破坏性修改影响稳定性
replace github.com/anthropics/anthropic-sdk-go => ./libs/anthropic-sdk-go

replace github.com/sashabaranov/go-openai => ./libs/go-openai

replace github.com/ollama/ollama => ./libs/ollama

// cron 久未维护，改放本地（libs/）便于发现问题直接改源码
replace github.com/robfig/cron/v3 => ./libs/cron

// TOON 紧凑序列化格式（libs/），用于投喂模型前把结构化工具结果从 JSON 压缩为
// 更低 token 的表式表示，以更少噪音、稳定前缀缓存
replace github.com/toon-format/toon-go => ./libs/toon-go

// vodka 的日志模板依赖 fasttemplate / bytebufferpool。原为上游第三方模块，
// 依赖模块缓存与网络；改放本地（libs/）确保离线可复现构建
replace github.com/valyala/fasttemplate => ./libs/fasttemplate

replace github.com/valyala/bytebufferpool => ./libs/bytebufferpool

replace github.com/wippyai/go-lua => ./libs/go-lua
