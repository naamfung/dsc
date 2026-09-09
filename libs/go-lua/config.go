package lua

import (
	"os"
)

var FieldsPerFlush = 50
var RegistrySize = 256
var RegistryGrowStep = 32
var RegistryMaxSize = 256 * 256
var CallStackSize = 128
var MaxTableGetLoop = 100
var MaxArrayIndex = 67108864

type LNumber float64
type LInteger int64

const LNumberBit = 64
const Version = "Lua 5.3 - Wippy Modification"

var PathEnvVar = "LUA_PATH"
var LDir string // todo: drop it
var PathDefault string
var DirSep string
var PathSep = ";"
var PathMark = "?"
var ExecDir = "!"
var IgMark = "-"

func init() {
	if os.PathSeparator == '/' { // unix-like
		LDir = "/usr/local/share/lua/5.1"
		DirSep = "/"
		PathDefault = "./?.lua;" + LDir + "/?.lua;" + LDir + "/?/init.lua"
	} else { // windows
		LDir = "!\\lua"
		DirSep = "\\"
		PathDefault = ".\\?.lua;" + LDir + "\\?.lua;" + LDir + "\\?\\init.lua"
	}
}
