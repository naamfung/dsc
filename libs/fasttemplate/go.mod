module github.com/valyala/fasttemplate

go 1.12

require github.com/valyala/bytebufferpool v1.0.0

// 本地维护的 bytebufferpool（libs/），避免依赖上游模块缓存/网络
replace github.com/valyala/bytebufferpool => ../bytebufferpool
