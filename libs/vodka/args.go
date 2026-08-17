package vodka

import (
	"fmt"
	"strconv"
	"time"

	"dsc/libs/vodka/libraries/com"
)

type (
	Args struct {
		s string
	}
)

func (a *Args) Int() (int, error) {
	return com.StrTo(a.s).Int()
}

func (a *Args) Int8() (int8, error) {
	i, err := strconv.ParseInt(a.s, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("failed to convert to int8: %v", err)
	}
	return int8(i), nil
}

func (a *Args) Int16() (int16, error) {
	i, err := strconv.ParseInt(a.s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("failed to convert to int16: %v", err)
	}
	return int16(i), nil
}

func (a *Args) Int32() (int32, error) {
	i, err := strconv.ParseInt(a.s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to convert to int32: %v", err)
	}
	return int32(i), nil
}

func (a *Args) Int64() (int64, error) {
	return com.StrTo(a.s).Int64()
}

func (a *Args) MustInt() int {
	return com.StrTo(a.s).MustInt()
}

func (a *Args) MustInt8() int8 {
	i, _ := a.Int8()
	return i
}

func (a *Args) MustInt16() int16 {
	i, _ := a.Int16()
	return i
}

func (a *Args) MustInt32() int32 {
	i, _ := a.Int32()
	return i
}

func (a *Args) MustInt64() int64 {
	return com.StrTo(a.s).MustInt64()
}

func (a *Args) MustUint() uint {
	u, _ := strconv.ParseUint(a.s, 10, 0) // 0 表示要使用 uint 的默认宽度
	return uint(u)
}

func (a *Args) MustUint8() uint8 {
	return com.StrTo(a.s).MustUint8()
}

func (a *Args) MustUint16() uint16 {
	u, _ := strconv.ParseUint(a.s, 10, 16)
	return uint16(u)
}

func (a *Args) MustUint32() uint32 {
	u, _ := strconv.ParseUint(a.s, 10, 32)
	return uint32(u)
}

func (a *Args) MustUint64() uint64 {
	u, _ := strconv.ParseUint(a.s, 10, 64)
	return u
}

func (a *Args) Float32() (f float32, e error) {
	var _f float64
	_f, e = strconv.ParseFloat(a.s, 32)
	f = float32(_f)
	return
}

func (a *Args) MustFloat32() (f float32) {
	_f, _ := strconv.ParseFloat(a.s, 32)
	f = float32(_f)
	return
}

func (a *Args) Float64() (f float64, e error) {
	f, e = strconv.ParseFloat(a.s, 64)
	return
}

func (a *Args) MustFloat64() (f float64) {
	f, _ = strconv.ParseFloat(a.s, 64)
	return
}

func (a *Args) String() string {
	return a.s
}

func (a *Args) Bytes() []byte {
	return []byte(a.s)
}

func (a *Args) Time() (time.Time, error) {
	tme, err := time.Parse("2006-01-02 03:04:05 PM", a.s)
	return tme, err
}

func (a *Args) MustTime() time.Time {
	tme, _ := time.Parse("2006-01-02 03:04:05 PM", a.s)
	return tme
}

func (a *Args) TimeByISO8601() (time.Time, error) {
	tme, err := time.Parse(time.RFC3339, a.s)
	return tme, err
}

func (a *Args) MustTimeByISO8601() time.Time {
	tme, _ := time.Parse(time.RFC3339, a.s)
	return tme
}

func (a *Args) Exist() bool {
	return com.StrTo(a.s).Exist()
}

func (a *Args) ToStr(args ...int) (s string) {
	return com.ToStr(a.s, args...)
}

func (a *Args) ToSnakeCase(str ...string) string {
	var s string
	if len(str) > 0 {
		s = str[0]
	} else {
		if len(a.s) != 0 {
			s = a.s
		}
	}
	return com.ToSnakeCase(s)
}

// Param returns the named parameter value that is found in the URL path matching the current route.
// If the named parameter cannot be found, an empty string will be returned.
func (c *Context) Param(name string) *Args {
	var a = new(Args)
	for i, n := range c.pnames {
		if n == name {
			a.s = c.pvalues[i]
		}
	}
	return a
}

// FormArgs returns the named parameter value that is found in the form data.
// If the named parameter cannot be found, an empty string will be returned.
func (c *Context) FormArgs(key ...string) *Args {
	var a = new(Args)
	var k string
	if len(key) > 0 {
		k = key[0]
		a.s = c.Form(k)
	}
	return a
}

// Args 先从URL获取参数，若无则再尝试从from获取参数
func (c *Context) Args(Key ...string) *Args {
	var arg = new(Args)
	var key string
	if len(Key) > 0 {
		key = Key[0]
		// 尝试从 Param 获取值
		arg = c.Param(key)
		if arg.s != "" {
			return arg
		}
		// 尝试从 Query 获取值
		arg.s = c.Query(key)
		if arg.s != "" {
			return arg
		}
		// 尝试从 PostForm 获取值
		arg.s = c.PostForm(key)
		if arg.s != "" {
			return arg
		}
		// 尝试从 Form 获取值
		arg.s = c.Form(key)
		if arg.s != "" {
			return arg
		}
	}
	return arg
	// 如果所有方式都没有获得值，返回空值
}

// Parameter returns the i-th parameter value in the URL path matching the current route.
// If the i-th parameter value cannot be found, an empty string will be returned.
func (c *Context) Parameter(i int) (value string) {
	l := len(c.pnames)
	if i < l {
		value = c.pvalues[i]
	}
	return
}
