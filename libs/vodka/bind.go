package vodka

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

type (
	// Binder is the interface that wraps the Bind method.
	Binder interface {
		Bind(i interface{}, c *Context) error
	}

	// DefaultBinder is the default implementation of the Binder interface.
	DefaultBinder struct{}

	// BindUnmarshaler is the interface used to wrap the UnmarshalParam method.
	BindUnmarshaler interface {
		// UnmarshalParam decodes and assigns a value from an form or query param.
		UnmarshalParam(param string) error
	}
)

/*
调用方式
用户只需要调用一次 Bind 方法即可完成所有数据来源的绑定

	var data struct {
	    ID    uint   `json:"id" query:"id" path:"id" header:"X-User-ID" cookie:"user_id"`
	    Name  string `json:"name" query:"name" path:"name" header:"X-User-Name" cookie:"user_name"`
	    Email string `json:"email" query:"email" path:"email" header:"X-User-Email" cookie:"user_email"`
	}

	if err := c.Bind(&data); err != nil {
	    return err
	}
*/
func (b *DefaultBinder) Bind(i interface{}, c *Context) (err error) {
	// 绑定请求体数据
	if err = b.bindBody(i, c); err != nil {
		return err
	}

	// 绑定查询参数
	if err = b.bindQuery(i, c); err != nil {
		return err
	}

	// 绑定路径参数
	if err = b.bindPath(i, c); err != nil {
		return err
	}

	// 绑定 Header 数据
	if err = b.bindHeader(i, c); err != nil {
		return err
	}

	// 绑定 Cookie 数据
	if err = b.bindCookie(i, c); err != nil {
		return err
	}

	return nil
}

func (b *DefaultBinder) bindBody(i interface{}, c *Context) error {
	req := c.Request
	if req.ContentLength == 0 {
		if req.Method == GET || req.Method == DELETE {
			return nil // GET 同 DELETE 请求通常没有请求体
		}
		return NewHTTPError(http.StatusBadRequest, "Request body can't be empty")
	}

	ctype := req.Header.Get(HeaderContentType)
	switch {
	case strings.HasPrefix(ctype, MIMEApplicationJSON):
		if err := json.NewDecoder(req.Body).Decode(i); err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error())
		}
	case strings.HasPrefix(ctype, MIMEApplicationXML), strings.HasPrefix(ctype, MIMETextXML):
		if err := xml.NewDecoder(req.Body).Decode(i); err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error())
		}
	case strings.HasPrefix(ctype, MIMEApplicationForm), strings.HasPrefix(ctype, MIMEMultipartForm):
		params, err := c.FormParams()
		if err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if err := b.bindData(i, params, "form"); err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error())
		}
	default:
		return ErrUnsupportedMediaType
	}
	return nil
}

func (b *DefaultBinder) bindQuery(i interface{}, c *Context) error {
	// 获取查询参数
	queryParams := c.QueryParams()

	// 使用反射将查询参数绑定到结构体
	v := reflect.ValueOf(i).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := field.Tag.Get("query")
		if key == "" {
			key = strings.ToLower(field.Name) // 如果未有 query 标签，使用字段名的蛇形命名
		}

		// 获取查询参数的值
		values, exists := queryParams[key]
		if !exists || len(values) == 0 {
			continue
		}

		// 绑定第一个非空值
		value := values[0]
		if value == "" {
			continue
		}

		fieldValue := v.Field(i)
		switch fieldValue.Kind() {
		case reflect.String:
			fieldValue.SetString(value)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if intValue, err := strconv.ParseUint(value, 10, 64); err == nil {
				fieldValue.SetUint(intValue)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
				fieldValue.SetInt(intValue)
			}
		case reflect.Float32, reflect.Float64:
			if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
				fieldValue.SetFloat(floatValue)
			}
		case reflect.Bool:
			if boolValue, err := strconv.ParseBool(value); err == nil {
				fieldValue.SetBool(boolValue)
			}
		default:
			return fmt.Errorf("unsupported field type: %v", fieldValue.Kind())
		}
	}

	return nil
}

func (b *DefaultBinder) bindPath(i interface{}, c *Context) error {
	pathParams := make(map[string][]string)
	for i, name := range c.pnames {
		pathParams[name] = []string{c.pvalues[i]}
	}
	if len(pathParams) == 0 {
		return nil
	}
	return b.bindData(i, pathParams, "path")
}

func (b *DefaultBinder) bindHeader(i interface{}, c *Context) error {
	headerParams := make(map[string][]string)
	for key, values := range c.Request.Header {
		headerParams[key] = values
	}
	if len(headerParams) == 0 {
		return nil
	}
	return b.bindData(i, headerParams, "header")
}

func (b *DefaultBinder) bindCookie(i interface{}, c *Context) error {
	cookieParams := make(map[string][]string)
	for _, cookie := range c.Request.Cookies() {
		cookieParams[cookie.Name] = []string{cookie.Value}
	}
	if len(cookieParams) == 0 {
		return nil
	}
	return b.bindData(i, cookieParams, "cookie")
}

func (b *DefaultBinder) bindData(ptr interface{}, data map[string][]string, tag string) error {
	typ := reflect.TypeOf(ptr).Elem()
	val := reflect.ValueOf(ptr).Elem()

	if typ.Kind() != reflect.Struct {
		return errors.New("binding element must be a struct")
	}

	for i := 0; i < typ.NumField(); i++ {
		typeField := typ.Field(i)
		structField := val.Field(i)
		if !structField.CanSet() {
			continue
		}
		structFieldKind := structField.Kind()
		inputFieldName := typeField.Tag.Get(tag)

		if inputFieldName == "" {
			inputFieldName = typeField.Name
			// If tag is nil, we inspect if the field is a struct.
			if _, ok := bindUnmarshaler(structField); !ok && structFieldKind == reflect.Struct {
				err := b.bindData(structField.Addr().Interface(), data, tag)
				if err != nil {
					return err
				}
				continue
			}
		}
		inputValue, exists := data[inputFieldName]
		if !exists {
			continue
		}

		// Call this first, in case we're dealing with an alias to an array type
		if ok, err := unmarshalField(typeField.Type.Kind(), inputValue[0], structField); ok {
			if err != nil {
				return err
			}
			continue
		}

		numElems := len(inputValue)
		if structFieldKind == reflect.Slice && numElems > 0 {
			sliceOf := structField.Type().Elem().Kind()
			slice := reflect.MakeSlice(structField.Type(), numElems, numElems)
			for j := 0; j < numElems; j++ {
				if err := setWithProperType(sliceOf, inputValue[j], slice.Index(j)); err != nil {
					return err
				}
			}
			val.Field(i).Set(slice)
		} else {
			if err := setWithProperType(typeField.Type.Kind(), inputValue[0], structField); err != nil {
				return err
			}
		}
	}
	return nil
}

func setWithProperType(valueKind reflect.Kind, val string, structField reflect.Value) error {
	// But also call it here, in case we're dealing with an array of BindUnmarshalers
	if ok, err := unmarshalField(valueKind, val, structField); ok {
		return err
	}

	switch valueKind {
	case reflect.Ptr:
		return setWithProperType(structField.Elem().Kind(), val, structField.Elem())
	case reflect.Int:
		return setIntField(val, 0, structField)
	case reflect.Int8:
		return setIntField(val, 8, structField)
	case reflect.Int16:
		return setIntField(val, 16, structField)
	case reflect.Int32:
		return setIntField(val, 32, structField)
	case reflect.Int64:
		return setIntField(val, 64, structField)
	case reflect.Uint:
		return setUintField(val, 0, structField)
	case reflect.Uint8:
		return setUintField(val, 8, structField)
	case reflect.Uint16:
		return setUintField(val, 16, structField)
	case reflect.Uint32:
		return setUintField(val, 32, structField)
	case reflect.Uint64:
		return setUintField(val, 64, structField)
	case reflect.Bool:
		return setBoolField(val, structField)
	case reflect.Float32:
		return setFloatField(val, 32, structField)
	case reflect.Float64:
		return setFloatField(val, 64, structField)
	case reflect.String:
		structField.SetString(val)
	default:
		return errors.New("unknown type")
	}
	return nil
}

func unmarshalField(valueKind reflect.Kind, val string, field reflect.Value) (bool, error) {
	switch valueKind {
	case reflect.Ptr:
		return unmarshalFieldPtr(val, field)
	default:
		return unmarshalFieldNonPtr(val, field)
	}
}

// bindUnmarshaler attempts to unmarshal a reflect.Value into a BindUnmarshaler
func bindUnmarshaler(field reflect.Value) (BindUnmarshaler, bool) {
	ptr := reflect.New(field.Type())
	if ptr.CanInterface() {
		iface := ptr.Interface()
		if unmarshaler, ok := iface.(BindUnmarshaler); ok {
			return unmarshaler, ok
		}
	}
	return nil, false
}

func unmarshalFieldNonPtr(value string, field reflect.Value) (bool, error) {
	if unmarshaler, ok := bindUnmarshaler(field); ok {
		err := unmarshaler.UnmarshalParam(value)
		field.Set(reflect.ValueOf(unmarshaler).Elem())
		return true, err
	}
	return false, nil
}

func unmarshalFieldPtr(value string, field reflect.Value) (bool, error) {
	if field.IsNil() {
		// Initialize the pointer to a nil value
		field.Set(reflect.New(field.Type().Elem()))
	}
	return unmarshalFieldNonPtr(value, field.Elem())
}

func setIntField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	intVal, err := strconv.ParseInt(value, 10, bitSize)
	if err == nil {
		field.SetInt(intVal)
	}
	return err
}

func setUintField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	uintVal, err := strconv.ParseUint(value, 10, bitSize)
	if err == nil {
		field.SetUint(uintVal)
	}
	return err
}

func setBoolField(value string, field reflect.Value) error {
	if value == "" {
		value = "false"
	}
	boolVal, err := strconv.ParseBool(value)
	if err == nil {
		field.SetBool(boolVal)
	}
	return err
}

func setFloatField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0.0"
	}
	floatVal, err := strconv.ParseFloat(value, bitSize)
	if err == nil {
		field.SetFloat(floatVal)
	}
	return err
}
