package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

/* load lib {{{ */

var loLoaders = []LGoFunc{loLoaderPreload, loLoaderLua}

func loGetPath(env string, defpath string) string {
	path := os.Getenv(env)
	if len(path) == 0 {
		path = defpath
	}
	path = strings.ReplaceAll(path, ";;", ";"+defpath+";")
	if os.PathSeparator != '/' {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err == nil {
			path = strings.ReplaceAll(path, "!", dir)
		}
		// If Abs fails, leave "!" unsubstituted (paths with "!" won't match)
	}
	return path
}

func loFindFile(L *LState, name, pname string) (string, string) {
	name = strings.ReplaceAll(name, ".", string(os.PathSeparator))
	lv := L.GetField(L.GetField(L.Get(EnvironIndex), "package"), pname)
	path, ok := lv.(LString)
	if !ok {
		L.RaiseError("package.%s must be a string", pname)
	}
	var messages []string
	for _, pattern := range strings.Split(string(path), ";") {
		luapath := strings.ReplaceAll(pattern, "?", name)
		_, err := os.Stat(luapath)
		if err == nil {
			return luapath, ""
		}
		messages = append(messages, err.Error())
	}
	return "", strings.Join(messages, "\n\t")
}

func OpenPackage(L *LState) int {
	packagemod := L.RegisterGoModule(LoadLibName, loFuncs)

	L.SetField(packagemod, "preload", L.NewTable())

	loaders := L.CreateTable(len(loLoaders), 0)
	for i, loader := range loLoaders {
		L.RawSetInt(loaders, i+1, loader)
	}
	L.SetField(packagemod, "loaders", loaders)
	L.SetField(L.Get(RegistryIndex), "_LOADERS", loaders)

	loaded := L.NewTable()
	L.SetField(packagemod, "loaded", loaded)
	L.SetField(L.Get(RegistryIndex), "_LOADED", loaded)

	L.SetField(packagemod, "path", LString(loGetPath(PathEnvVar, PathDefault)))
	L.SetField(packagemod, "cpath", emptyLString)

	L.SetField(packagemod, "config", LString(DirSep+"\n"+PathSep+
		"\n"+PathMark+"\n"+ExecDir+"\n"+IgMark+"\n"))

	L.Push(packagemod)
	return 1
}

var loFuncs = map[string]LGoFunc{
	"loadlib": loLoadLib,
	"seeall":  loSeeAll,
}

func loLoaderPreload(L *LState) int {
	name := L.CheckString(1)
	preload := L.GetField(L.GetField(L.Get(EnvironIndex), "package"), "preload")
	if _, ok := preload.(*LTable); !ok {
		L.RaiseError("package.preload must be a table")
	}
	lv := L.GetField(preload, name)
	if lv == LNil {
		L.Push(LString(fmt.Sprintf("no field package.preload['%s']", name)))
		return 1
	}
	L.Push(lv)
	return 1
}

func loLoaderLua(L *LState) int {
	name := L.CheckString(1)
	path, msg := loFindFile(L, name, "path")
	if len(path) == 0 {
		L.Push(LString(msg))
		return 1
	}
	fn, err1 := L.LoadFile(path)
	if err1 != nil {
		L.RaiseError(err1.Error())
	}
	L.Push(fn)
	return 1
}

func loLoadLib(L *LState) int {
	L.RaiseError("loadlib is not supported")
	return 0
}

func loSeeAll(L *LState) int {
	mod := L.CheckTable(1)
	mt := L.GetMetatable(mod)
	if mt == LNil {
		mt = L.CreateTable(0, 1)
		L.SetMetatable(mod, mt)
	}
	L.SetField(mt, "__index", L.Get(GlobalsIndex))
	return 0
}

/* }}} */

//
