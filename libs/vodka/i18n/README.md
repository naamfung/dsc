# i18n

Middleware i18n provides app Internationalization and Localization for [Vodka](https://gitcode.com/jorsion/vodka).

### Installation

	vodka/i18n

## Example

```go
package main

import (
	"fmt"
	"garpress/library/vodka"
	"garpress/library/vodka/i18n"
)

func main() {
	m := macross.Classic()
	m.Use(i18n.I18n(i18n.Options{
		Directory:   "locale",
		DefaultLang: "zh-CN",
		Langs:       []string{"en-US", "zh-CN"},
		Names:       []string{"English", "简体中文"},
		Redirect:    true,
	}))

	m.Get("/", func(self *macross.Context) error {
		fmt.Println("Header>", self.Request.Header.String())
		return self.String("current language is " + self.Language())
	})

	// Use in handler.
	m.Get("/trans", func(self *macross.Context) error {
		fmt.Println("Header>", self.Request.Header.String())
		return self.String(fmt.Sprintf("hello %s", self.Tr("world")))
	})

	fmt.Println("Listen on 9999")
	m.Listen(9999)
}
```

## Getting Help

- [API Reference](https://gowalker.org/gitcode.com/jorsion/vodka/i18n)

## License

This project is under the Apache License, Version 2.0. See the [LICENSE](LICENSE) file for the full license text.
