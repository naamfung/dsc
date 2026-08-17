package jsplugin

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dsc/libs/vodka"

	"github.com/fsnotify/fsnotify"
)

var (
	g  = make(map[string]string)
	jk *fsnotify.Watcher
)

func GetPlugins(pathname string) (paths []string) {
	rd, err := os.ReadDir(pathname)
	if err != nil {
		log.Println(err)
		return
	}
	for _, fi := range rd {
		if fi.IsDir() {
			paths = append(paths, (pathname + "/" + fi.Name() + "/" + fi.Name() + ".js"))
		}
	}
	return
}

func watcher(dir string) {
	var err error
	jk, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer jk.Close()

	if err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			path, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			err = jk.Add(path)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	var n = make(map[int64]int)
	for {
		select {
		case event, ok := <-jk.Events:
			if !ok {
				return
			}
			m := time.Now().Unix()
			if n[m]++; n[m] > 1 {
				n = make(map[int64]int)
				continue
			}
			for t := range n {
				if t != m {
					n = make(map[int64]int)
					continue
				}
			}
			log.Println("event:", event)

			var sss string
			g = make(map[string]string)
			for _, jsp := range GetPlugins(dir) {
				if len(sss) == 0 {
					sss = jsp
					continue
				}
				sss = fmt.Sprintf("%v;%v", sss, jsp)
			}
			g[dir] = sss //update cache js files path

			for _, jsp := range strings.Split(g[dir], ";") {
				var f, e = os.OpenFile(jsp, os.O_RDONLY, os.ModePerm)
				if e != nil {
					log.Println(e)
					continue
				}
				defer f.Close()
				var b, erro = io.ReadAll(f)
				if erro != nil {
					log.Println(erro)
					continue
				}
				var jsc = string(b)
				g[jsp] = jsc //update cache for javascript code
			}

		case err, ok := <-jk.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}

func JSPlugin(dir ...string) vodka.Handler {
	var dirPath = "./plugin"
	if len(dir) > 0 {
		dirPath = dir[0]
	}
	if jk == nil {
		go watcher(dirPath)
	}
	return func(c *vodka.Context) (err error) {
		c.JSR.Set("ctx", c)
		if _, okay := g[dirPath]; !okay {
			var sss string
			for _, jsp := range GetPlugins(dirPath) {
				if len(sss) == 0 {
					sss = jsp
					continue
				}
				sss = fmt.Sprintf("%v;%v", sss, jsp)
			}
			g[dirPath] = sss //cache js files path
		}
		for _, jsp := range strings.Split(g[dirPath], ";") {
			ss := strings.Split(jsp, "/")
			var jsf string
			for _, s := range ss {
				if strings.Contains(s, ".js") {
					jsf = strings.Split(s, ".")[0]
				}
			}
			if len(jsf) > 0 {
				if v, okay := g[jsp]; okay {
					var jsv, err = c.JSR.RunString(v)
					if err != nil {
						return fmt.Errorf("JSPlugin c.JSR.RunString[cache] Error => %v", err.Error())
					}
					if jsv != nil {
						v := c.JSR.Get("ctx")
						if ctx, okay := v.Export().(*vodka.Context); okay {
							c = ctx //return ctx.Next()
							continue
						} else {
							return fmt.Errorf("<(*vodka.Context) == (%v)> at Plugin:[%v]", v.ExportType(), jsf)
						}
					}
				} else {
					var f, e = os.OpenFile(jsp, os.O_RDONLY, os.ModePerm)
					if e != nil {
						return e
					}
					defer f.Close()
					var b, erro = io.ReadAll(f)
					if erro != nil {
						return erro
					}
					var jsc = string(b)
					var jsv, err = c.JSR.RunString(jsc)
					if err != nil {
						return fmt.Errorf("JSPlugin c.JSR.RunString[file] Error => %v", err)
					}
					if jsv != nil {
						v := c.JSR.Get("ctx")
						if ctx, okay := v.Export().(*vodka.Context); okay {
							g[jsp] = jsc //cache for javascript code
							c = ctx      //return ctx.Next()
						} else {
							return fmt.Errorf("<(*vodka.Context) == (%v)> at Plugin:[%v]", v.ExportType(), jsf)
						}
					}
				}
			}
		}
		return c.Next()
	}
}
