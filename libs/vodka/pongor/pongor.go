package pongor

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dsc/libs/vodka"

	"github.com/flosch/pongo2/v6"
	"github.com/microcosm-cc/bluemonday"
)

/*
此中间件配置Option.Directory的值时有几种情况

第1种如下:
v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "/path/to/tpldir/", Reload: false, Filter: true}))
此种情况默认使用 default 作为集合名

调用时 ctx.Render("index")//匹配 default 集合名下指定的 index.html路径

第2种如下:
v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "cool,/path/to/tpldir1/;fire,/path/to/tpldir2/", Reload: false, Filter: true}))

调用时 ctx.Render("fire,index")//匹配 fire 集合名下指定的 /path/to/tpldir2/index.html路径

第2种情况的配置下,如果调用时无指定集合名，即如 ctx.Render("index"), 会返回默认集合名下指定的 index.html路径的模板内容

第3种如下:
v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "wind,/path/to/tpldir1/", Reload: false, Filter: true}))

调用时 必须显式指定集合名，即如 ctx.Render("wind,index")//匹配 wind 集合名下指定的 /path/to/tpldir1/index.html路径, 因为未指定默认集合名，所以无法匹配到默认集合名下的模板文件, 此时可以放弃wind集合名，指定为 default 集合名,或者不设置集合名就是默认集合名, 又或者可以增加额外的模板目录配置, 如 v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "default,/path/to/tpldir1/;wind,/path/to/tpldir2/", Reload: false, Filter: true}))
此时,ctx.Render("index")就是默认集合下的模板,此时可以省去default集合名,但wind集合下的模板调用时仍然须要显式指定集合名,即如 ctx.Render("wind,index")
*/

// Option 结构体配置多个模板目录
type Option struct {
	// Directory to load templates. 可以配置多个目录，使用分号分隔
	Directory string
	// Reload to reload templates everytime.
	Reload bool
	// Filter to do Filter for templates
	Filter bool
}

// Renderer 结构体用于模版渲染
type Renderer struct {
	Option
	templates   map[string]map[string]*pongo2.Template // 存储模板缓存
	lock        sync.RWMutex
	directories map[string]string // 集合名到目录路径的映射
}

// perparOption 处理 Option，确保有默认配置
func perparOption(options []Option) Option {
	var opt Option
	if len(options) > 0 {
		opt = options[0]
	}
	if len(opt.Directory) == 0 {
		opt.Directory = "templates" // 设置默认目录, 即指定程序当前工作目录下的 ./templates 作为默认模板目录
	}
	return opt
}

// Renderor 创建一个新的 Renderer
func Renderor(opt ...Option) *Renderer {
	o := perparOption(opt)
	r := &Renderer{
		Option: o,
	}
	r.parseDirectory() // 调用解析方法
	return r
}

// parseDirectory 解析 Directory 字段，构建集合到路径的映射
func (r *Renderer) parseDirectory() {
	r.templates = make(map[string]map[string]*pongo2.Template)
	r.directories = make(map[string]string)

	if r.Directory == "" { //此时经已被perparOption处理过, 所以正常情况下一定会有值. 此处纯属防御性范式.
		return
	}

	if !strings.Contains(r.Directory, ";") { //单个目录配置时
		var name, dir string
		if !strings.Contains(r.Directory, ",") { //纯粹的路径时
			name = "default"
			dir = r.Directory
		} else { //用户显式指定了集合名及其路径时
			parts := strings.SplitN(r.Directory, ",", 2)
			if len(parts) != 2 {
				panic("模板目录配置错误, 请按照 `集合名,路径` 的格式指定")
			}
			name = strings.TrimSpace(parts[0]) // 集合名
			dir = strings.TrimSpace(parts[1])  // 目录
		}
		r.directories[name] = dir                             // 存储集合名及路径的对应关系
		r.templates[name] = make(map[string]*pongo2.Template) // 初始化集合的模板映射
	} else {
		// 以分号为界分割多个集合
		collections := strings.Split(r.Directory, ";")
		for _, collection := range collections {
			if strings.TrimSpace(collection) == "" {
				continue
			}

			parts := strings.SplitN(collection, ",", 2)
			if len(parts) != 2 {
				continue // 如果格式不对, 跳过此条配置
			}
			name := strings.TrimSpace(parts[0]) // 集合名
			dir := strings.TrimSpace(parts[1])  // 目录

			// 存储集合名与目录路径的对应关系
			r.directories[name] = dir
			r.templates[name] = make(map[string]*pongo2.Template) // 初始化集合的模板映射
		}
	}

}

// getDirectory 获取集合对应的目录
func (r *Renderer) getDirectory(collection string) string {
	if collection == "default" {
		// 返回默认集合的目录
		if dir, ok := r.directories["default"]; ok {
			return dir
		}
		if len(r.directories) > 1 {
			fmt.Println("未指定默认模板集合，如须要在调用时省去其模板集合名称, 请在多模板配置时设置默认集合，即使用 `default` 名称指定其中之一的路径为默认模板集合所在.")
		}
		return "" // 如果没有找到 default 集合，返回空字符串
	}
	// 返回对应集合名的目录路径
	return r.directories[collection]
}

// buildTemplatesCache 根据集合名和模板名构建模板缓存
func (r *Renderer) buildTemplatesCache(collection, name string) (t *pongo2.Template, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// 检查集合是否已存在，如果不存在则初始化
	if r.templates[collection] == nil {
		r.templates[collection] = make(map[string]*pongo2.Template)
	}

	var dir = r.getDirectory(collection)
	if len(dir) <= 0 {
		return nil, fmt.Errorf("模板目录 %s 不存在", collection)
	}

	// 使用集合对应的目录路径与模板名组合获取模板文件
	t, err = pongo2.FromFile(filepath.Join(dir, name))
	if err != nil {
		return
	}
	r.templates[collection][name] = t // 存储缓存
	return
}

// getTemplate 获取模板
func (r *Renderer) getTemplate(name string) (t *pongo2.Template, err error) {
	parts := strings.Split(name, ",")
	var collection string
	var templateName string

	if len(parts) > 1 {
		collection = parts[0]
		templateName = parts[1]
	} else {
		collection = "default" // 默认集合
		templateName = name
	}

	if !strings.HasSuffix(templateName, ".html") {
		templateName = templateName + ".html"
	}

	// 检查是否需要重新载入模板
	if r.Reload {
		return pongo2.FromFile(filepath.Join(r.getDirectory(collection), templateName))
	}

	r.lock.RLock()
	var ok bool
	if t, ok = r.templates[collection][templateName]; !ok {
		r.lock.RUnlock()
		t, err = r.buildTemplatesCache(collection, templateName)
	} else {
		r.lock.RUnlock()
	}
	return
}

func sanitize(s string) string {
	p := bluemonday.StrictPolicy()
	plainText := p.Sanitize(s)
	return plainText
}

// Render 方法渲染模板
func (r *Renderer) Render(w io.Writer, name string, c *vodka.Context) error {
	template, err := r.getTemplate(name)
	if err != nil {
		return err
	}

	c.Set("range", rangeFunction)
	c.Set("sanitize", sanitize)
	c.Set("currentYear", currentYear)

	var buffer bytes.Buffer
	err = template.ExecuteWriter(c.GetStore(), &buffer)
	if err != nil {
		return err
	}

	if b := buffer.Bytes(); r.Filter {
		_, err = fmt.Fprintf(w, "%s", c.DoFilterHook(fmt.Sprintf("%s_render", name), func() []byte {
			return b
		}))
	} else {
		_, err = fmt.Fprintf(w, "%s", b)
	}
	return err
}

func rangeFunction(start int, end int) (s []int) {
	if start > end {
		return s // 如果起始值大于结束值，返回空切片
	}

	result := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

// currentYear returns the current year as a string
func currentYear() string {
	return time.Now().Format("2006")
}
