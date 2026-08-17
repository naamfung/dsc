package static

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dsc/libs/vodka"
	"dsc/libs/vodka/skipper"
)

type (
	// StaticConfig defines the config for Static middleware.
	StaticConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper skipper.Skipper

		// Root directory from where the static content is served.
		// Required.
		Root string `json:"root"`

		// Index file for serving a directory.
		// Optional. Default value "index.html".
		Index string `json:"index"`

		// Enable HTML5 mode by forwarding all not-found requests to root so that
		// SPA (single-page application) can handle the routing.
		// Optional. Default value false.
		HTML5 bool `json:"html5"`

		// Enable directory browsing.
		// Optional. Default value false.
		Browse bool `json:"browse"`
	}
)

var (
	// DefaultStaticConfig is the default Static middleware config.
	DefaultStaticConfig = StaticConfig{
		Skipper: skipper.DefaultSkipper,
		Index:   "index.html",
	}
)

// Static returns a Static middleware to serves static content from the provided
// root directory.
func Static(root string) vodka.Handler {
	c := DefaultStaticConfig
	c.Root = root
	return StaticWithConfig(c)
}

// StaticWithConfig returns a Static middleware with config.
// See `Static()`.
func StaticWithConfig(config StaticConfig) vodka.Handler {
	// Defaults
	if config.Root == "" {
		config.Root = "." // For security we want to restrict to CWD.
	}
	if config.Skipper == nil {
		config.Skipper = DefaultStaticConfig.Skipper
	}
	if config.Index == "" {
		config.Index = DefaultStaticConfig.Index
	}

	return func(c *vodka.Context) error {
		if config.Skipper(c) {
			return c.Next()
		}

		p := c.Request.URL.Path
		if strings.HasSuffix(c.Request.URL.Path, "*") { // When serving from a group, e.g. `/static*`.
			//p = c.Param("*").String()
			p = c.Parameter(0)
		}
		name := filepath.Join(config.Root, path.Clean("/"+p)) // "/"+ for security

		fi, err := os.Stat(name)
		if err != nil {
			if os.IsNotExist(err) {
				// 只有当HTML5模式开启且路径未有扩展名时，至会返回index.html
				// 确保CSS、JS等资源文件不会被错误处理
				ext := path.Ext(p)
				if config.HTML5 && len(ext) == 0 {
					return c.ServeFile(filepath.Join(config.Root, config.Index))
				}
				// 文件不存在时返回404错误，避免继续处理导致异常响应
				c.Response.Header().Set("Content-Type", "text/plain")
				c.Response.WriteHeader(404)
				c.Response.Write([]byte("404 Not Found"))
				return c.Abort()
			}
			return err
		}

		if fi.IsDir() {
			index := filepath.Join(name, config.Index)
			fi, err = os.Stat(index)

			if err != nil {
				if config.Browse {
					return listDir(name, c.Response)
				}
				if os.IsNotExist(err) {
					return c.Next()
				}
				return err
			}

			return c.ServeFile(index)
		}

		return c.ServeFile(name)
	}

}

func listDir(name string, res *vodka.Response) error {
	dir, err := os.Open(name)
	if err != nil {
		return err
	}
	dirs, err := dir.Readdir(-1)
	if err != nil {
		return err
	}

	// Create a directory index
	res.Header().Set(vodka.HeaderContentType, vodka.MIMETextHTMLCharsetUTF8)
	if _, err = fmt.Fprintf(res, "<pre>\n"); err != nil {
		return err
	}
	for _, d := range dirs {
		name := d.Name()
		color := "#212121"
		if d.IsDir() {
			color = "#e91e63"
			name += "/"
		}
		if _, err = fmt.Fprintf(res, "<a href=\"%s\" style=\"color: %s;\">%s</a>\n", name, color, name); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(res, "</pre>\n")
	return err
}
