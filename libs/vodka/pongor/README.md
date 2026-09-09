# pongor

Package pongor is a middleware that provides pongo2 template engine support for vodka. 

## Document

此中间件配置Option.Directory的值时有几种情况

### 第1种如下:

v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "/path/to/tpldir/", Reload: false, Filter: true}))
此种情况默认使用 default 作为集合名

调用时 ctx.Render("index")//匹配 default 集合名下指定的 index.html路径

### 第2种如下:

v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "cool,/path/to/tpldir1/;fire,/path/to/tpldir2/", Reload: false, Filter: true}))

调用时 ctx.Render("fire,index")//匹配 fire 集合名下指定的 /path/to/tpldir2/index.html路径

第2种情况的配置下,如果调用时无指定集合名，即如 ctx.Render("index"), 会返回默认集合名下指定的 index.html路径的模板内容

### 第3种如下:

v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "wind,/path/to/tpldir1/", Reload: false, Filter: true}))

调用时 必须显式指定集合名，即如 ctx.Render("wind,index")//匹配 wind 集合名下指定的 /path/to/tpldir1/index.html路径, 因为未指定默认集合名，所以无法匹配到默认集合名下的模板文件, 此时可以放弃wind集合名，指定为 default 集合名,或者不设置集合名就是默认集合名, 又或者可以增加额外的模板目录配置, 如 v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "default,/path/to/tpldir1/;wind,/path/to/tpldir2/", Reload: false, Filter: true}))

此时,ctx.Render("index")就是默认集合下的模板,此时可以省去default集合名,但wind集合下的模板调用时仍然须要显式指定集合名,即如 ctx.Render("wind,index")
