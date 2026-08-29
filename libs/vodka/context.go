// Package vodka is a high productive and modular web framework in Golang.

package vodka

import (
	"bytes"
	ktx "context"
	"dsc/libs/vodka/libraries/i18n"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/zeebo/blake3"
	"golang.org/x/time/rate"
)

const (
	indexPage     = "index.html"
	defaultMemory = 32 << 20 // 32 MB
)

type (
	// Map is a shortcut for map[string]interface{}
	Map map[string]interface{}

	// Context represents the contextual data and environment while processing an incoming HTTP request.
	Context struct {
		Request  *http.Request // the current request
		Response *Response     // the response writer
		ktx      ktx.Context   // standard context
		Localer
		Flash      *Flash
		Session    Sessioner
		vodka      *Vodka
		pnames     []string  // list of route parameter names
		pvalues    []string  // list of parameter values corresponding to pnames
		data       *sync.Map // data items managed by Get and Set
		FiltersMap *sync.Map //map[string][]byte      // Not Global Filters, only in Context
		index      int       // the index of the currently executing handler in handlers
		handlers   []Handler // the handlers associated with the current route
		writer     DataWriter
	}

	// Localer reprents a localization interface.
	Localer interface {
		Language() string
		Tr(string, ...interface{}) string
	}

	localer struct {
		i18n.Locale
	}
)

// Reset sets the request and response of the context and resets all other properties.
func (c *Context) Reset(w http.ResponseWriter, r *http.Request) {
	c.Response.reset(w)
	c.Request = r
	c.ktx = ktx.Background()
	c.data = nil
	c.FiltersMap = new(sync.Map)
	c.index = -1
	c.writer = DefaultDataWriter
}

/*
var data struct {
    ID    uint   `json:"id" query:"id" path:"id" header:"X-User-ID" cookie:"user_id"`
    Name  string `json:"name" query:"name" path:"name" header:"X-User-Name" cookie:"user_name" validate:"required"`
    Email string `json:"email" query:"email" path:"email" header:"X-User-Email" cookie:"user_email"`
}

if err := c.BindAndValidate(&data); err != nil {
    return err
}
*/
// BindAndValidate 绑定与验证数据
func (c *Context) BindAndValidate(ptr interface{}) error {
	if err := c.Bind(ptr); err != nil {
		return fmt.Errorf("bind error: %w", err)
	}
	if err := c.Validator().Struct(ptr); err != nil {
		return fmt.Errorf("validate error: %w", err)
	}
	return nil
}

func (c *Context) Validator() *validator.Validate {
	return validator.New()
}

func (c *Context) Map(key string, value interface{}) Map {
	return Map{key: value}
}

// Vodka returns the Vodka that is handling the incoming HTTP request.
func (c *Context) Vodka() *Vodka {
	return c.vodka
}

// Shutdown 优雅停止HTTP服务 不超过特定时长
func (c *Context) Shutdown(times ...int64) error {
	return c.vodka.Shutdown(times...)
}

// Close 立即关闭HTTP服务
func (c *Context) Close() error {
	return c.vodka.Server.Close()
}

func (c *Context) Kontext() ktx.Context {
	return c.ktx
}

func (c *Context) SetKontext(ktx ktx.Context) {
	c.ktx = ktx
}

func (c *Context) Handler() Handler {
	return c.handlers[c.index]
}

func (c *Context) SetHandler(h Handler) {
	c.handlers[c.index] = h
}

func (c *Context) NewCookie() *http.Cookie {
	return new(http.Cookie)
}

func (c *Context) GetCookie(name string) (*http.Cookie, error) {
	return c.Request.Cookie(name)
}

func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Response, cookie)
}

func (c *Context) GetCookies() []*http.Cookie {
	return c.Request.Cookies()
}

// NewHTTPError creates a new HTTPError instance.
func (c *Context) NewHTTPError(status int, message ...interface{}) *HTTPError {
	return c.vodka.NewHTTPError(status, message...)
}

func (c *Context) Error(status int, message ...interface{}) {
	herr := NewHTTPError(status, message...)
	c.HandleError(herr)
}

func (c *Context) HandleError(err error) {
	c.vodka.HandleError(c, err)
}

func (c *Context) IsWebSocket() bool {
	upgrade := c.Request.Header.Get(HeaderUpgrade)
	return upgrade == "websocket" || upgrade == "Websocket"
}

// RealIP implements `Context#RealIP` function.
func (c *Context) RealIP() string {
	ra := c.Request.RemoteAddr
	if ip := c.Request.Header.Get(HeaderXForwardedFor); len(ip) > 0 {
		ra = ip
	} else if ip := c.Request.Header.Get(HeaderXRealIP); len(ip) > 0 {
		ra = ip
	} else {
		ra, _, _ = net.SplitHostPort(ra)
	}
	return ra
}

// Get returns the named data item previously registered with the context by calling Set.
// If the named data item cannot be found, nil will be returned.
func (c *Context) Get(key string) any {
	c.touchData()
	value, _ := c.data.Load(key)
	return value
}

// Set stores the named data item in the context so that it can be retrieved later.
func (c *Context) Set(key string, value any) {
	c.touchData()

	// 如果 value 为 nil，并且 key 存在，则删除该 key
	if value == nil {
		if _, exists := c.data.Load(key); exists {
			c.data.Delete(key)
			return
		}
	}

	// 否则，存储 key-value
	c.data.Store(key, value)
}

func (c *Context) SetStore(data map[string]any) {
	c.touchData()
	for k, v := range data {
		c.data.Store(k, v)
	}
}

func (c *Context) GetStore() map[string]any {
	c.touchData()
	result := make(map[string]any)
	c.data.Range(func(key, value any) bool {
		result[key.(string)] = value
		return true
	})
	return result
}

func (c *Context) touchData() {
	if c.data == nil {
		c.data = &sync.Map{}
	}
}

func (c *Context) Pull(key any) any {
	c.vodka.touchData()
	value, _ := c.vodka.data.Load(key)
	return value
}

func (c *Context) Push(key, value any) {
	c.vodka.touchData()

	// 如果 value 为 nil，并且 key 存在，则删除该 key
	if value == nil {
		if _, exists := c.vodka.data.Load(key); exists {
			c.vodka.data.Delete(key)
			return
		}
	}

	// 否则，存储 key-value
	c.vodka.data.Store(key, value)
}

func (c *Context) PullStore() map[any]any {
	c.vodka.touchData()
	result := make(map[any]any)
	c.vodka.data.Range(func(key, value any) bool {
		result[key] = value
		return true
	})
	return result
}

func (c *Context) PushStore(data map[any]any) {
	c.vodka.touchData()
	for k, v := range data {
		c.vodka.data.Store(k, v)
	}
}

func (c *Context) Bind(i interface{}) error {
	return c.vodka.binder.Bind(i, c)
}

func (c *Context) UserAgent() string {
	return c.Request.UserAgent()
}

func (c *Context) RequestURI() string {
	return c.Request.RequestURI
}

func (c *Context) RequestBody() io.Reader {
	rb, _ := c.Request.GetBody()
	return rb
}

func (c *Context) MultipartForm() (*multipart.Form, error) {
	err := c.Request.ParseMultipartForm(defaultMemory)
	return c.Request.MultipartForm, err
}

func (c *Context) QueryString() string {
	return c.Request.URL.RawQuery
}

func (c *Context) QueryParam(name string) string {
	return c.Request.URL.Query().Get(name)
}

func (c *Context) QueryParams() url.Values {
	return c.Request.URL.Query()
}

func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	_, fh, err := c.Request.FormFile(name)
	return fh, err
}

func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

func (c *Context) FormParams() (url.Values, error) {
	if strings.HasPrefix(c.Request.Header.Get(HeaderContentType), MIMEMultipartForm) {
		if err := c.Request.ParseMultipartForm(defaultMemory); err != nil {
			return nil, err
		}
	} else {
		if err := c.Request.ParseForm(); err != nil {
			return nil, err
		}
	}
	return c.Request.Form, nil
}

// Query returns the first value for the named component of the URL query parameters.
// If key is not present, it returns the specified default value or an empty string.
func (c *Context) Query(name string, defaultValue ...string) string {
	vs := c.Request.URL.Query()[name] // 直接赋值给 vs
	if len(vs) > 0 {
		return vs[0]
	}

	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// Form returns the first value for the named component of the query.
// Form reads the value from POST and PUT body parameters as well as URL query parameters.
// The form takes precedence over the latter.
// If key is not present, it returns the specified default value or an empty string.
func (c *Context) Form(key string, defaultValue ...string) string {
	r := c.Request
	r.ParseMultipartForm(32 << 20)
	if vs := r.Form[key]; len(vs) > 0 {
		return vs[0]
	}

	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// PostForm returns the first value for the named component from POST and PUT body parameters.
// If key is not present, it returns the specified default value or an empty string.
func (c *Context) PostForm(key string, defaultValue ...string) string {
	r := c.Request
	r.ParseMultipartForm(32 << 20)
	if vs := r.PostForm[key]; len(vs) > 0 {
		return vs[0]
	}

	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// Next calls the rest of the handlers associated with the current route.
// If any of these handlers returns an error, Next will return the error and skip the following handlers.
// Next is normally used when a handler needs to do some postprocessing after the rest of the handlers
// are executed.
func (c *Context) Next() error {
	c.index++
	for n := len(c.handlers); c.index < n; c.index++ {
		if err := c.handlers[c.index](c); err != nil {
			return err
		}
	}
	return nil
}

// Abort skips the rest of the handlers associated with the current route.
// Abort is normally used when a handler handles the request normally and wants to skip the rest of the handlers.
// If a handler wants to indicate an error condition, it should simply return the error without calling Abort.
func (c *Context) Abort() error {
	c.index = len(c.handlers)
	return nil
}

// Break 中断继续执行后续动作，返回指定状态及错误，不设置错误亦可.
func (c *Context) Break(status int, err ...error) error {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	c.Response.WriteHeader(status)
	c.HandleError(e)
	return c.Abort()
}

// URL creates a URL using the named route and the parameter values.
// The parameters should be given in the sequence of name1, value1, name2, value2, and so on.
// If a parameter in the route is not provided a value, the parameter token will remain in the resulting URL.
// Parameter values will be properly URL encoded.
// The method returns an empty string if the URL creation fails.
func (c *Context) URL(route string, pairs ...interface{}) string {
	if r := c.vodka.namedRoutes[route]; r != nil {
		return r.URL(pairs...)
	}
	return ""
}

// Read populates the given struct variable with the data from the current request.
// If the request is NOT a GET request, it will check the "Content-Type" header
// and find a matching reader from DataReaders to read the request data.
// If there is no match or if the request is a GET request, it will use DefaultFormDataReader
// to read the request data.
func (c *Context) Read(data interface{}) error {
	if c.Request.Method != "GET" {
		t := getContentType(c.Request)
		if reader, ok := DataReaders[t]; ok {
			return reader.Read(c.Request, data)
		}
	}

	return DefaultFormDataReader.Read(c.Request, data)
}

// --------------------------------------------------------------------------------------------
/*
使用示例：
// 示例1：启用哈希计算及动态限速（基础速率5MB/s）
form, hashes, err := c.ReadFormPlus(-5*1024*1024, true, "uploads")

// 示例2：固定速率模式（10MB/s），不计算哈希
form, _, err := c.ReadFormPlus(10*1024*1024, false, "uploads")

// 示例3：无限制模式，计算哈希
form, hashes, err := c.ReadFormPlus(0, true, "uploads")

// 示例4：动态限速（基础速率20MB/s），不计算哈希
form, _, err := c.ReadFormPlus(-20*1024*1024, false, "uploads")

参数说明：
limit int64 - 速率控制：

> 0: 固定速率（字节/秒）

< 0: 动态速率（绝对值作为基础速率）

= 0: 无限制

enableHash bool - 哈希计算开关：

true: 计算BLAKE3文件哈希

false: 跳过哈希计算

args ...string - 额外参数：

第一个有效目录路径：指定文件存储位置

第一个有效整数字符串：最大内存使用量
*/
// ReadFormPlus 增强版文件上传处理，支持限速、动态速率调整及可选哈希计算
func (c *Context) ReadFormPlus(limit int64, enableHash bool, args ...string) (_ *multipart.Form, fileHashes map[string][]byte, err error) {
	c.Request.MultipartForm = nil
	r, e := c.Request.MultipartReader()
	if nil != e {
		return nil, nil, e
	}

	form := new(multipart.Form)
	form.Value = make(map[string][]string)
	form.File = make(map[string][]*multipart.FileHeader)
	fileHashes = make(map[string][]byte)

	defer func() {
		if err != nil {
			form.RemoveAll()
		}
	}()

	var output string
	var maxMemory int64
	if len(args) > 0 {
		for _, v := range args {
			i, e := strconv.ParseInt(v, 10, 64)
			if nil != e {
				if info, err := os.Stat(v); err == nil && info.IsDir() {
					output = v
				} else {
					continue
				}
			} else {
				maxMemory = i
			}
		}
	}

	// Reserve an additional 10 MB for non-file parts.
	maxValueBytes := maxMemory + int64(10<<20)
	if maxValueBytes <= 0 {
		if maxMemory < 0 {
			maxValueBytes = 0
		} else {
			maxValueBytes = math.MaxInt64
		}
	}

	// 创建全局限速器（所有文件共享）
	var (
		diskLimiter    *rate.Limiter
		adaptiveWriter *adaptiveRateWriter
		isDynamic      = false
	)

	switch {
	case limit > 0: // 固定速率模式
		diskLimiter = rate.NewLimiter(rate.Limit(limit), int(limit*2))
	case limit < 0: // 动态速率模式
		baseRate := -limit // 使用绝对值作为基础速率
		diskLimiter = rate.NewLimiter(rate.Limit(baseRate), int(baseRate*2))
		adaptiveWriter = &adaptiveRateWriter{
			limiter:  diskLimiter,
			baseRate: rate.Limit(baseRate),
			lastTime: time.Now(),
		}
		isDynamic = true
	}

	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		name := p.FormName()
		if name == "" {
			continue
		}
		filename := p.FileName()

		var b bytes.Buffer

		if filename == "" {
			// 处理非文件字段
			n, err := io.CopyN(&b, p, maxValueBytes+1)
			if err != nil && err != io.EOF {
				return nil, nil, fmt.Errorf("value, store as string in memory, Error: %v", err)
			}
			maxValueBytes -= n
			if maxValueBytes < 0 {
				return nil, nil, multipart.ErrMessageTooLarge
			}
			form.Value[name] = append(form.Value[name], b.String())
			continue
		}

		// 文件处理部分
		fh := &multipart.FileHeader{
			Filename: filename,
			Header:   p.Header,
		}

		var file *os.File
		var full string
		var writers []io.Writer

		// 创建目标文件
		if len(output) > 0 {
			filename = sanitizeFilename(filename)
			full = path.Join(output, filename)
			file, err = os.Create(full)
			if err != nil {
				return nil, nil, err
			}
		} else {
			file, err = os.CreateTemp(os.TempDir(), "multipart-")
			if err != nil {
				return nil, nil, err
			}
			full = file.Name() // 直接获取完整路径
		}

		// 添加文件写入器
		writers = append(writers, file)

		// 添加哈希计算器（如果启用）
		var hasher *blake3.Hasher
		if enableHash {
			hasher = blake3.New()
			writers = append(writers, hasher)
		}

		// 创建组合写入器
		combinedWriter := io.MultiWriter(writers...)
		var writer io.Writer = combinedWriter

		// 启用限速
		if limit != 0 {
			if isDynamic {
				// 动态速率模式
				adaptiveWriter.writer = combinedWriter
				writer = adaptiveWriter
			} else {
				// 固定速率模式
				writer = &rateLimitedWriter{
					writer:  combinedWriter,
					limiter: diskLimiter,
				}
			}
		}

		// 先读取 maxMemory+1 字节
		n, err := io.CopyN(writer, p, maxMemory+1)

		// 处理大文件（超过 maxMemory）
		if err == nil && n > maxMemory {
			// 继续读取剩余部分
			additional, err := io.Copy(writer, p)
			if err != nil && err != io.EOF {
				os.Remove(full)
				return nil, nil, fmt.Errorf("additional copy error: %v", err)
			}
			n += additional
		} else if err != nil && err != io.EOF {
			os.Remove(full)
			return nil, nil, fmt.Errorf("initial copy error: %v", err)
		}

		if cerr := file.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(full)
			return nil, nil, err
		}

		fh.Filename = file.Name() // 服务器端文件名
		fh.Size = n
		form.File[name] = append(form.File[name], fh)

		// 保存哈希值（如果启用）
		if enableHash {
			fileHashes[filename] = hasher.Sum(nil)
		}
	}

	c.Request.MultipartForm = form
	return form, fileHashes, nil
}

// 辅助函数：清理文件名中的特殊字符
func sanitizeFilename(filename string) string {
	replacer := strings.NewReplacer(
		" ", "_", "#", "_", "%", "_",
		"+", "_", "/", "_", "\\", "_",
		"_-_", "-", "..", "", "\x00", "",
	)
	return replacer.Replace(filename)
}

// 固定速率写入器
type rateLimitedWriter struct {
	writer  io.Writer
	limiter *rate.Limiter
}

func (w *rateLimitedWriter) Write(p []byte) (int, error) {
	// 分批处理避免大块数据卡死
	chunkSize := 32 * 1024 // 32KB
	total := len(p)

	for i := 0; i < total; i += chunkSize {
		end := i + chunkSize
		if end > total {
			end = total
		}
		chunk := p[i:end]

		// 等待速率令牌
		if err := w.limiter.WaitN(ktx.Background(), len(chunk)); err != nil {
			return i, err
		}

		// 写入实际数据
		if n, err := w.writer.Write(chunk); err != nil {
			return i + n, err
		}
	}
	return total, nil
}

// 动态速率写入器（根据磁盘负载自动调整）
type adaptiveRateWriter struct {
	writer   io.Writer
	limiter  *rate.Limiter
	baseRate rate.Limit // 基础速率
	lastTime time.Time  // 上次写入时间
}

func (w *adaptiveRateWriter) Write(p []byte) (int, error) {
	start := time.Now()

	// 如果是第一次写入，初始化时间
	if w.lastTime.IsZero() {
		w.lastTime = start
	}

	// 计算实际写入延迟
	actualDelay := time.Since(w.lastTime)
	w.lastTime = start

	// 动态调整速率
	const targetDelay = 20 * time.Millisecond // 目标延迟
	const adjustmentFactor = 0.1              // 调整因子

	switch {
	case actualDelay > targetDelay*2: // 延迟过高，大幅降速
		newRate := w.baseRate * 0.5
		w.limiter.SetLimit(newRate)
	case actualDelay > targetDelay: // 延迟偏高，小幅降速
		newRate := w.limiter.Limit() * (1 - adjustmentFactor)
		w.limiter.SetLimit(newRate)
	case actualDelay < targetDelay/2: // 延迟过低，提速
		newRate := w.limiter.Limit() * (1 + adjustmentFactor)
		if newRate > w.baseRate*2 {
			newRate = w.baseRate * 2
		}
		w.limiter.SetLimit(newRate)
	}

	// 使用固定限速器写入
	return (&rateLimitedWriter{
		writer:  w.writer,
		limiter: w.limiter,
	}).Write(p)
}

// --------------------------------------------------------------------------------------------
// // 固定速率写入器
// type rateLimitedWriter struct {
// 	writer  io.Writer
// 	limiter *rate.Limiter
// }

// func (w *rateLimitedWriter) Write(p []byte) (int, error) {
// 	// 分批处理避免大块数据卡死
// 	chunkSize := 32 * 1024 // 32KB
// 	total := len(p)

// 	for i := 0; i < total; i += chunkSize {
// 		end := i + chunkSize
// 		if end > total {
// 			end = total
// 		}
// 		chunk := p[i:end]

// 		// 等待速率令牌
// 		if err := w.limiter.WaitN(context.Background(), len(chunk)); err != nil {
// 			return i, err
// 		}

// 		// 写入实际数据
// 		if n, err := w.writer.Write(chunk); err != nil {
// 			return i + n, err
// 		}
// 	}
// 	return total, nil
// }

// // 动态速率写入器（根据磁盘负载自动调整）
// type adaptiveRateWriter struct {
// 	writer   io.Writer
// 	limiter  *rate.Limiter
// 	baseRate rate.Limit // 基础速率
// 	lastTime time.Time  // 上次写入时间
// }

// func (w *adaptiveRateWriter) Write(p []byte) (int, error) {
// 	start := time.Now()

// 	// 如果是第一次写入，初始化时间
// 	if w.lastTime.IsZero() {
// 		w.lastTime = start
// 	}

// 	// 计算实际写入延迟
// 	actualDelay := time.Since(w.lastTime)
// 	w.lastTime = start

// 	// 动态调整速率
// 	const targetDelay = 20 * time.Millisecond // 目标延迟
// 	const adjustmentFactor = 0.1              // 调整因子

// 	switch {
// 	case actualDelay > targetDelay*2: // 延迟过高，大幅降速
// 		newRate := w.baseRate * 0.5
// 		w.limiter.SetLimit(newRate)
// 	case actualDelay > targetDelay: // 延迟偏高，小幅降速
// 		newRate := w.limiter.Limit() * (1 - adjustmentFactor)
// 		w.limiter.SetLimit(newRate)
// 	case actualDelay < targetDelay/2: // 延迟过低，提速
// 		newRate := w.limiter.Limit() * (1 + adjustmentFactor)
// 		if newRate > w.baseRate*2 {
// 			newRate = w.baseRate * 2
// 		}
// 		w.limiter.SetLimit(newRate)
// 	}

// 	// 使用固定限速器写入
// 	return (&rateLimitedWriter{
// 		writer:  w.writer,
// 		limiter: w.limiter,
// 	}).Write(p)
// }

// // ReadFormPlus 增强版文件上传处理，支持限速及哈希计算
// func (c *Context) ReadFormPlus(limit int64, args ...string) (_ *multipart.Form, fileHashes map[string][]byte, err error) {
// 	c.Request.MultipartForm = nil
// 	r, e := c.Request.MultipartReader()
// 	if nil != e {
// 		return nil, nil, e
// 	}

// 	form := new(multipart.Form)
// 	form.Value = make(map[string][]string)
// 	form.File = make(map[string][]*multipart.FileHeader)
// 	fileHashes = make(map[string][]byte)

// 	defer func() {
// 		if err != nil {
// 			form.RemoveAll()
// 		}
// 	}()

// 	var output string
// 	var maxMemory int64
// 	if len(args) > 0 {
// 		for _, v := range args {
// 			i, e := strconv.ParseInt(v, 10, 64)
// 			if nil != e {
// 				if info, err := os.Stat(v); err == nil && info.IsDir() {
// 					output = v
// 				} else {
// 					continue
// 				}
// 			} else {
// 				maxMemory = i
// 			}
// 		}
// 	}

// 	// Reserve an additional 10 MB for non-file parts.
// 	maxValueBytes := maxMemory + int64(10<<20)
// 	if maxValueBytes <= 0 {
// 		if maxMemory < 0 {
// 			maxValueBytes = 0
// 		} else {
// 			maxValueBytes = math.MaxInt64
// 		}
// 	}

// 	// 创建全局限速器（所有文件共享）
// 	var (
// 		diskLimiter    *rate.Limiter
// 		adaptiveWriter *adaptiveRateWriter
// 		isDynamic      = false
// 	)

// 	switch {
// 	case limit > 0: // 固定速率模式
// 		diskLimiter = rate.NewLimiter(rate.Limit(limit), int(limit*2))
// 	case limit < 0: // 动态速率模式
// 		baseRate := -limit // 使用绝对值作为基础速率
// 		diskLimiter = rate.NewLimiter(rate.Limit(baseRate), int(baseRate*2))
// 		adaptiveWriter = &adaptiveRateWriter{
// 			limiter:  diskLimiter,
// 			baseRate: rate.Limit(baseRate),
// 			lastTime: time.Now(),
// 		}
// 		isDynamic = true
// 	}

// 	for {
// 		p, err := r.NextPart()
// 		if err == io.EOF {
// 			break
// 		}
// 		if err != nil {
// 			return nil, nil, err
// 		}

// 		name := p.FormName()
// 		if name == "" {
// 			continue
// 		}
// 		filename := p.FileName()

// 		var b bytes.Buffer

// 		if filename == "" {
// 			// value, store as string in memory
// 			n, err := io.CopyN(&b, p, maxValueBytes+1)
// 			if err != nil && err != io.EOF {
// 				return nil, nil, fmt.Errorf("value, store as string in memory, Error: %v", err)
// 			}
// 			maxValueBytes -= n
// 			if maxValueBytes < 0 {
// 				return nil, nil, multipart.ErrMessageTooLarge
// 			}
// 			form.Value[name] = append(form.Value[name], b.String())
// 			continue
// 		}

// 		// 初始化BLAKE3哈希计算器
// 		hasher := blake3.New()

// 		// file, store in memory or on disk
// 		fh := &multipart.FileHeader{
// 			Filename: filename,
// 			Header:   p.Header,
// 		}

// 		var file *os.File
// 		var full string

// 		if len(output) > 0 {
// 			filename = strings.Replace(filename, " ", "_", -1)
// 			filename = strings.Replace(filename, "#", "_", -1)
// 			filename = strings.Replace(filename, "%", "_", -1)
// 			filename = strings.Replace(filename, "+", "_", -1)
// 			filename = strings.Replace(filename, "/", "_", -1)
// 			filename = strings.Replace(filename, "\\", "_", -1)
// 			filename = strings.Replace(filename, "_-_", "-", -1)
// 			full = path.Join(output, filename)
// 			file, err = os.Create(full)
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 		} else {
// 			file, err = os.CreateTemp(os.TempDir(), "multipart-")
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 			full = path.Join(os.TempDir(), file.Name())
// 		}

// 		// 创建组合写入器：文件 + 哈希
// 		combinedWriter := io.MultiWriter(file, hasher)
// 		var writer io.Writer = combinedWriter

// 		// 启用限速
// 		if limit != 0 {
// 			if isDynamic {
// 				// 动态速率模式
// 				adaptiveWriter.writer = combinedWriter
// 				writer = adaptiveWriter
// 			} else {
// 				// 固定速率模式
// 				writer = &rateLimitedWriter{
// 					writer:  combinedWriter,
// 					limiter: diskLimiter,
// 				}
// 			}
// 		}

// 		// 先读取 maxMemory+1 字节
// 		n, err := io.CopyN(writer, p, maxMemory+1)

// 		// 处理大文件（超过 maxMemory）
// 		if err == nil && n > maxMemory {
// 			// 继续读取剩余部分
// 			additional, err := io.Copy(writer, p)
// 			if err != nil && err != io.EOF {
// 				os.Remove(full)
// 				return nil, nil, fmt.Errorf("additional copy error: %v", err)
// 			}
// 			n += additional
// 		} else if err != nil && err != io.EOF {
// 			os.Remove(full)
// 			return nil, nil, fmt.Errorf("initial copy error: %v", err)
// 		}

// 		if cerr := file.Close(); err == nil {
// 			err = cerr
// 		}
// 		if err != nil {
// 			os.Remove(full)
// 			return nil, nil, err
// 		}

// 		fh.Filename = file.Name() // server file
// 		fh.Size = n
// 		form.File[name] = append(form.File[name], fh)
// 		fileHashes[filename] = hasher.Sum(nil)
// 	}

// 	c.Request.MultipartForm = form
// 	return form, fileHashes, nil
// }

// func (c *Context) ReadFormPlus(limit int64, args ...string) (_ *multipart.Form, fileHashes map[string][]byte, err error) {
// 	c.Request.MultipartForm = nil
// 	r, e := c.Request.MultipartReader()
// 	if nil != e {
// 		return nil, nil, e
// 	}

// 	form := new(multipart.Form)
// 	form.Value = make(map[string][]string)
// 	form.File = make(map[string][]*multipart.FileHeader)
// 	fileHashes = make(map[string][]byte)

// 	defer func() {
// 		if err != nil {
// 			form.RemoveAll()
// 		}
// 	}()

// 	var output string
// 	var maxMemory int64
// 	if len(args) > 0 {
// 		for _, v := range args {
// 			i, e := strconv.ParseInt(v, 10, 64)
// 			if nil != e {
// 				if info, err := os.Stat(v); err == nil && info.IsDir() {
// 					output = v
// 				} else {
// 					continue
// 				}
// 			} else {
// 				maxMemory = i
// 			}
// 		}
// 	}

// 	// Reserve an additional 10 MB for non-file parts.
// 	maxValueBytes := maxMemory + int64(10<<20)
// 	if maxValueBytes <= 0 {
// 		if maxMemory < 0 {
// 			maxValueBytes = 0
// 		} else {
// 			maxValueBytes = math.MaxInt64
// 		}
// 	}

// 	var diskLimiter *rate.Limiter
// 	if limit > 0 {
// 		// 创建速率限制器，突发设置为速率的2倍
// 		diskLimiter = rate.NewLimiter(rate.Limit(limit), int(limit*2))
// 	}

// 	for {
// 		p, err := r.NextPart()
// 		if err == io.EOF {
// 			break
// 		}
// 		if err != nil {
// 			return nil, nil, err
// 		}

// 		name := p.FormName()
// 		if name == "" {
// 			continue
// 		}
// 		filename := p.FileName()

// 		var b bytes.Buffer

// 		if filename == "" {
// 			// value, store as string in memory
// 			n, err := io.CopyN(&b, p, maxValueBytes+1)
// 			if err != nil && err != io.EOF {
// 				return nil, nil, fmt.Errorf("value, store as string in memory, Error: %v", err)
// 			}
// 			maxValueBytes -= n
// 			if maxValueBytes < 0 {
// 				return nil, nil, multipart.ErrMessageTooLarge
// 			}
// 			form.Value[name] = append(form.Value[name], b.String())
// 			continue
// 		}

// 		// 初始化BLAKE3哈希计算器
// 		hasher := blake3.New()

// 		// file, store in memory or on disk
// 		fh := &multipart.FileHeader{
// 			Filename: filename,
// 			Header:   p.Header,
// 		}

// 		var file *os.File
// 		var full string

// 		if len(output) > 0 {
// 			filename = strings.Replace(filename, " ", "_", -1)
// 			filename = strings.Replace(filename, "#", "_", -1)
// 			filename = strings.Replace(filename, "%", "_", -1)
// 			filename = strings.Replace(filename, "+", "_", -1)
// 			filename = strings.Replace(filename, "/", "_", -1)
// 			filename = strings.Replace(filename, "\\", "_", -1)
// 			filename = strings.Replace(filename, "_-_", "-", -1)
// 			full = path.Join(output, filename)
// 			file, err = os.Create(full)
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 		} else {
// 			file, err = os.CreateTemp(os.TempDir(), "multipart-")
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 			full = path.Join(os.TempDir(), file.Name())
// 		}

// 		// 创建组合写入器：文件 + 哈希计算
// 		combinedWriter := io.MultiWriter(file, hasher)
// 		var writer io.Writer = combinedWriter

// 		// 如果启用限速，包装写入器
// 		if diskLimiter != nil {
// 			writer = &rateLimitedWriter{
// 				writer:  combinedWriter,
// 				limiter: diskLimiter,
// 			}
// 		}

// 		// 先读取 maxMemory+1 字节
// 		n, err := io.CopyN(writer, p, maxMemory+1)

// 		// 处理大文件（超过 maxMemory）
// 		if err == nil && n > maxMemory {
// 			// 继续读取剩余部分
// 			additional, err := io.Copy(writer, p)
// 			if err != nil && err != io.EOF {
// 				os.Remove(full)
// 				return nil, nil, fmt.Errorf("additional copy error: %v", err)
// 			}
// 			n += additional
// 		} else if err != nil && err != io.EOF {
// 			os.Remove(full)
// 			return nil, nil, fmt.Errorf("initial copy error: %v", err)
// 		}

// 		if cerr := file.Close(); err == nil {
// 			err = cerr
// 		}
// 		if err != nil {
// 			os.Remove(full)
// 			return nil, nil, err
// 		}

// 		fh.Filename = file.Name() // server file
// 		fh.Size = n
// 		form.File[name] = append(form.File[name], fh)
// 		fileHashes[filename] = hasher.Sum(nil)
// 	}

// 	c.Request.MultipartForm = form
// 	return form, fileHashes, nil
// }

// ReadFormPlus 使用ReadFormPlus接收文件上传时，ReadFormPlus必须执行在处理器的最前边, 或不耗时的操作之后,
// 否则可能会超时, 继而导致ReadFormPlus接收失败.
// func (c *Context) ReadFormPlus(args ...string) (_ *multipart.Form, err error) {
// 	c.Request.MultipartForm = nil
// 	r, e := c.Request.MultipartReader()
// 	if nil != e {
// 		return nil, e
// 	}

// 	form := new(multipart.Form)
// 	form.Value = make(map[string][]string)
// 	form.File = make(map[string][]*multipart.FileHeader)

// 	defer func() {
// 		if err != nil {
// 			form.RemoveAll()
// 		}
// 	}()

// 	var output string
// 	var maxMemory int64
// 	if len(args) > 0 {
// 		for _, v := range args {
// 			i, e := strconv.ParseInt(v, 10, 64)
// 			if nil != e {
// 				if info, err := os.Stat(v); err == nil && info.IsDir() {
// 					output = v
// 				} else {
// 					continue
// 				}
// 			} else {
// 				maxMemory = i
// 			}
// 		}
// 	}

// 	// Reserve an additional 10 MB for non-file parts.
// 	maxValueBytes := maxMemory + int64(10<<20)
// 	if maxValueBytes <= 0 {
// 		if maxMemory < 0 {
// 			maxValueBytes = 0
// 		} else {
// 			maxValueBytes = math.MaxInt64
// 		}
// 	}
// 	for {
// 		p, err := r.NextPart()
// 		if err == io.EOF {
// 			break
// 		}
// 		if err != nil {
// 			return nil, err
// 		}

// 		name := p.FormName()
// 		if name == "" {
// 			continue
// 		}
// 		filename := p.FileName()

// 		var b bytes.Buffer

// 		if filename == "" {
// 			// value, store as string in memory
// 			n, err := io.CopyN(&b, p, maxValueBytes+1)
// 			if err != nil && err != io.EOF {
// 				return nil, fmt.Errorf("value, store as string in memory, Error: %v", err)
// 			}
// 			maxValueBytes -= n
// 			if maxValueBytes < 0 {
// 				return nil, multipart.ErrMessageTooLarge
// 			}
// 			form.Value[name] = append(form.Value[name], b.String())
// 			continue
// 		}

// 		var content []byte
// 		// file, store in memory or on disk
// 		fh := &multipart.FileHeader{
// 			Filename: filename,
// 			Header:   p.Header,
// 		}
// 		n, err := io.CopyN(&b, p, maxMemory+1)
// 		if err != nil && err != io.EOF {
// 			return nil, fmt.Errorf("io.CopyN Error: %v", err)
// 		}
// 		if n > maxMemory { // too big, write to disk and flush buffer
// 			var file *os.File
// 			var full string
// 			if len(output) > 0 {
// 				filename = strings.Replace(filename, " ", "_", -1)
// 				filename = strings.Replace(filename, "#", "_", -1)
// 				filename = strings.Replace(filename, "%", "_", -1)
// 				filename = strings.Replace(filename, "+", "_", -1)
// 				filename = strings.Replace(filename, "/", "_", -1)
// 				filename = strings.Replace(filename, "\\", "_", -1)
// 				filename = strings.Replace(filename, "_-_", "-", -1)
// 				full = path.Join(output, filename)
// 				file, err = os.Create(full)
// 				if err != nil {
// 					return nil, err
// 				}
// 			} else {
// 				file, err = os.CreateTemp(os.TempDir(), "multipart-")
// 				if err != nil {
// 					return nil, err
// 				}
// 				full = path.Join(os.TempDir(), file.Name())
// 			}
// 			size, err := io.Copy(file, io.MultiReader(&b, p))
// 			if cerr := file.Close(); err == nil {
// 				err = cerr
// 			}
// 			if err != nil {
// 				os.Remove(full)
// 				return nil, err
// 			}
// 			fh.Filename = file.Name() //server file
// 			fh.Size = size
// 		} else {
// 			content = b.Bytes()
// 			fh.Size = int64(len(content))
// 			maxMemory -= n
// 			maxValueBytes -= n
// 		}
// 		form.File[name] = append(form.File[name], fh)
// 	}

// 	c.Request.MultipartForm = form
// 	return form, nil
// }
// --------------------------------------------------------------------------------------------

// Write writes the given data of arbitrary type to the response.
// The method calls the data writer set via SetDataWriter() to do the actual writing.
// By default, the DefaultDataWriter will be used.
func (c *Context) Write(data interface{}) error {
	return c.writer.Write(c.Response, data)
}

// WriteWithStatus sends the HTTP status code and writes the given data of arbitrary type to the response.
// See Write() for details on how data is written to response.
func (c *Context) WriteWithStatus(data interface{}, statusCode int) error {
	c.Response.WriteHeader(statusCode)
	return c.Write(data)
}

func (c *Context) Redirect(url string, status ...int) error {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusFound
	}
	if code < StatusMultipleChoices || code > StatusPermanentRedirect {
		return ErrInvalidRedirectCode
	}

	c.Response.Header().Set(HeaderLocation, url)
	c.Response.WriteHeader(code)
	c.Abort()
	return nil
}

func (c *Context) Render(name string, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	if c.vodka.renderer == nil {
		return ErrRendererNotRegistered
	}
	buf := new(bytes.Buffer)
	if err = c.vodka.renderer.Render(buf, name, c); err != nil {
		return
	}
	c.Response.Header().Set(HeaderContentType, MIMETextHTMLCharsetUTF8)
	err = c.WriteWithStatus(buf.Bytes(), code)
	c.Abort()
	return
}

func (c *Context) String(s string, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	c.Response.Header().Set(HeaderContentType, MIMETextPlainCharsetUTF8)
	err = c.WriteWithStatus(s, code)
	c.Abort()
	return
}

func (c *Context) JSON(i interface{}, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return c.JSONBlob(b, code)
}

func (c *Context) JSONPretty(i interface{}, indent string, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	b, err := json.MarshalIndent(i, "", indent)
	if err != nil {
		return
	}
	return c.JSONBlob(b, code)
}

func (c *Context) JSONBlob(b []byte, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	return c.Blob(MIMEApplicationJSONCharsetUTF8, b, code)
}

func (c *Context) JSONP(callback string, i interface{}, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return c.JSONPBlob(callback, b, code)
}

func (c *Context) JSONPBlob(callback string, b []byte, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	c.Response.Header().Set(HeaderContentType, MIMEApplicationJavaScriptCharsetUTF8)
	c.Response.WriteHeader(code)
	if err = c.Write([]byte(callback + "(")); err != nil {
		return
	}
	if err = c.Write(b); err != nil {
		return
	}
	err = c.Write([]byte(");"))
	c.Abort()
	return
}

func (c *Context) XML(i interface{}, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	b, err := xml.Marshal(i)
	if err != nil {
		return err
	}
	return c.XMLBlob(b, code)
}

func (c *Context) XMLPretty(i interface{}, indent string, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	b, err := xml.MarshalIndent(i, "", indent)
	if err != nil {
		return
	}
	return c.XMLBlob(b, code)
}

func (c *Context) XMLBlob(b []byte, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	c.Response.Header().Set(HeaderContentType, MIMEApplicationXMLCharsetUTF8)
	c.Response.WriteHeader(code)
	if err = c.Write([]byte(xml.Header)); err != nil {
		return
	}
	err = c.Write(b)
	c.Abort()
	return
}

func (c *Context) Blob(contentType string, b []byte, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	c.Response.Header().Set(HeaderContentType, contentType)
	err = c.WriteWithStatus(b, code)
	c.Abort()
	return
}

func (c *Context) Stream(contentType string, r io.Reader, status ...int) (err error) {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	c.Response.Header().Set(HeaderContentType, contentType)
	c.Response.WriteHeader(code)
	_, err = io.Copy(c.Response, r)
	c.Abort()
	return
}

// ServeFile serves a view file, to send a file ( zip for example) to the client
// you should use the SendFile(serverfilename,clientfilename)
//
// You can define your own "Content-Type" header also, after this function call
// This function doesn't implement resuming (by range), use ctx.SendFile/fasthttp.ServeFileUncompressed(ctx.RequestCtx,path)/fasthttpServeFile(ctx.RequestCtx,path) instead
//
// Use it when you want to serve css/js/... files to the client, for bigger files and 'force-download' use the SendFile
func (c *Context) ServeFile(file string) (err error) {
	file, err = url.QueryUnescape(file) // Issue #839
	if err != nil {
		return
	}

	f, err := os.Open(file)
	if err != nil {
		return ErrNotFound
	}
	defer f.Close()
	fi, _ := f.Stat()
	if fi.IsDir() {
		file = path.Join(file, indexPage)
		f, err = os.Open(file)
		if err != nil {
			return ErrNotFound
		}
		fi, _ = f.Stat()
	}
	http.ServeContent(c.Response, c.Request, fi.Name(), fi.ModTime(), f)
	return c.Abort() //c.ServeContent(f, fi.Name(), fi.ModTime())
}

// SendFile sends file for force-download to the client
//
// Use this instead of ServeFile to 'force-download' bigger files to the client
func (c *Context) SendFile(filename string, destinationName string) error {
	f, err := os.Open(filename)
	if err != nil {
		return ErrNotFound
	}
	defer f.Close()

	c.Response.Header().Set(HeaderContentDisposition, "attachment;filename="+destinationName)
	_, err = io.Copy(c.Response, f)
	c.Abort()
	return err
}

// At 在Context内即时调用handlers
func (c *Context) At(handlers ...Handler) error {
	var es []string
	for k, handler := range handlers {
		e := handler(c)
		if e != nil {
			es = append(es, fmt.Sprintf("[%v]%v", k, e.Error()))
		}
	}
	if len(es) > 0 {
		return fmt.Errorf("at error:%v", es)
	}
	return nil
}

func (c *Context) Attachment(file, name string) (err error) {
	return c.contentDisposition(file, name, "attachment")
}

func (c *Context) Inline(file, name string) (err error) {
	return c.contentDisposition(file, name, "inline")
}

func (c *Context) contentDisposition(file, name, dispositionType string) (err error) {
	c.Response.Header().Set(HeaderContentDisposition, fmt.Sprintf("%s; filename=%s", dispositionType, name))
	c.ServeFile(file)
	return
}

// NoContent Only header
func (c *Context) NoContent(status ...int) error {
	var code int
	if len(status) > 0 {
		code = status[0]
	} else {
		code = StatusOK
	}
	c.Response.WriteHeader(code)
	return c.Abort()
}

// // SetDataWriter sets the data writer that will be used by Write().
//
//	func (c *Context) SetDataWriter(writer DataWriter) {
//		c.writer = writer
//	}
//
// SetDataWriter sets the data writer that will be used by Write().
func (c *Context) SetDataWriter(writer DataWriter) {
	c.writer = writer
	writer.SetHeader(c.Response)
}

func getContentType(req *http.Request) string {
	t := req.Header.Get("Content-Type")
	for i, c := range t {
		if c == ' ' || c == ';' {
			return t[:i]
		}
	}
	return t
}

// ContentTypeByExtension returns the MIME type associated with the file based on
// its extension. It returns `application/octet-stream` incase MIME type is not
// found.
func (c *Context) ContentTypeByExtension(name string) (t string) {
	ext := filepath.Ext(name)
	//these should be found by the windows(registry) and unix(apache) but on windows some machines have problems on this part.
	if t = mime.TypeByExtension(ext); t == "" {
		// no use of map here because we will have to lock/unlock it, by hand is better, no problem:
		if ext == ".json" {
			t = MIMEApplicationJSON
		} else if ext == ".zip" {
			t = "application/zip"
		} else if ext == ".3gp" {
			t = "video/3gpp"
		} else if ext == ".7z" {
			t = "application/x-7z-compressed"
		} else if ext == ".ace" {
			t = "application/x-ace-compressed"
		} else if ext == ".aac" {
			t = "audio/x-aac"
		} else if ext == ".ico" { // for any case
			t = "image/x-icon"
		} else if ext == ".png" {
			t = "image/png"
		} else {
			t = MIMEOctetStream
		}
	}
	return
}

// TimeFormat is the time format to use when generating times in HTTP
// headers. It is like time.RFC1123 but hard-codes GMT as the time
// zone. The time being formatted must be in UTC for Format to
// generate the correct format.
//
// For parsing this time format, see ParseTime.
const TimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

// RequestHeader returns the request header's value
// accepts one parameter, the key of the header (string)
// returns string
func (c *Context) RequestHeader(key string) string {
	return c.Request.Header.Get(key)
}

func (c *Context) ServeContent(content io.ReadSeeker, filename string, modtime time.Time) error {
	// 设置必要的响应头
	c.Response.Header().Set(HeaderContentType, c.ContentTypeByExtension(filename))
	c.Response.Header().Set(HeaderLastModified, modtime.UTC().Format(TimeFormat))

	// 使用内置的ServeContent处理文件内容
	http.ServeContent(c.Response, c.Request, filename, modtime, content)
	return nil
}

// IsTLS implements `Context#TLS` function.
func (c *Context) IsTLS() bool {
	return c.Request.TLS != nil
}

func (c *Context) Scheme() string {

	// Can't use `r.Request.URL.Scheme`
	// See: https://groups.google.com/forum/#!topic/golang-nuts/pMUkBlQBDF0
	if c.IsTLS() {
		return "https"
	}
	if scheme := c.Request.Header.Get(HeaderXForwardedProto); scheme != "" {
		return scheme
	}
	if scheme := c.Request.Header.Get(HeaderXForwardedProtocol); scheme != "" {
		return scheme
	}
	if ssl := c.Request.Header.Get(HeaderXForwardedSsl); ssl == "on" {
		return "https"
	}
	if scheme := c.Request.Header.Get(HeaderXUrlScheme); scheme != "" {
		return scheme
	}
	return "http"
}
