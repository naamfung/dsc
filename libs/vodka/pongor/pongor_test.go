package pongor_test

import (
	"fmt"
	"testing"

	"dsc/libs/vodka"
	"dsc/libs/vodka/pongor"

	"github.com/flosch/pongo2/v6"
)

func TestRender(t *testing.T) {
	tpl, err := pongo2.FromString(`Hello {{ name|capfirst }}!
		{% for item in ss %}
			{% if forloop.Last %}
				这是最后一个元素: {{ item }},
				for: All the forloop fields (like forloop.counter) are written with a capital letter at the beginning. For example, the counter can be accessed by forloop.Counter and the parentloop by forloop.Parentloop.
			{% else %}
				{{ item }}
			{% endif %}
    	{% endfor %}`)
	if err != nil {
		panic(err)
	}
	ss := []string{"a", "b", "c"}
	out, err := tpl.Execute(pongo2.Context{"name": "florian", "ss": ss})
	if err != nil {
		panic(err)
	}
	fmt.Println(out)

	e := vodka.New()
	e.SetRenderer(pongor.Renderor())
	e.Get("/", func() vodka.Handler {
		return func(ctx *vodka.Context) error {
			ctx.Set("title", "你好，世界")
			// render ./template/index file.
			return ctx.Render("index")
		}
	}())
}
