module dsc-plugin-llm-anthropic

go 1.26.0

require (
	dsc v0.0.0
	dsc-sdk v0.0.0
	github.com/anthropics/anthropic-sdk-go v1.63.1
)

require github.com/wippyai/go-lua v1.5.17 // indirect

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.8.0
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/toon-format/toon-go v0.0.0 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace dsc-sdk => ../../sdk

replace dsc => ../..

replace github.com/hashicorp/go-plugin => ../../plugin

replace github.com/anthropics/anthropic-sdk-go => ../../libs/anthropic-sdk-go

replace github.com/toon-format/toon-go => ../../libs/toon-go

replace github.com/wippyai/go-lua => ../../libs/go-lua
