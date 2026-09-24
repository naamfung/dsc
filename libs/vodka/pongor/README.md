# pongor

Package pongor is a middleware that provides pongo2 template engine support for vodka. 

## Document

此中間件配置Option.Directory的值時有幾種情況

### 第1種如下:

v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "/path/to/tpldir/", Reload: false, Filter: true}))
此種情況默認使用 default 作為集合名

調用時 ctx.Render("index")//匹配 default 集合名下指定的 index.html路徑

### 第2種如下:

v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "cool,/path/to/tpldir1/;fire,/path/to/tpldir2/", Reload: false, Filter: true}))

調用時 ctx.Render("fire,index")//匹配 fire 集合名下指定的 /path/to/tpldir2/index.html路徑

第2種情況的配置下,如果調用時無指定集合名，即如 ctx.Render("index"), 會返回默認集合名下指定的 index.html路徑的模板內容

### 第3種如下:

v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "wind,/path/to/tpldir1/", Reload: false, Filter: true}))

調用時 必須顯式指定集合名，即如 ctx.Render("wind,index")//匹配 wind 集合名下指定的 /path/to/tpldir1/index.html路徑, 因為未指定默認集合名，所以無法匹配到默認集合名下的模板文件, 此時可以放棄wind集合名，指定為 default 集合名,或者不設置集合名就是默認集合名, 又或者可以增加額外的模板目錄配置, 如 v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "default,/path/to/tpldir1/;wind,/path/to/tpldir2/", Reload: false, Filter: true}))

此時,ctx.Render("index")就是默認集合下的模板,此時可以省去default集合名,但wind集合下的模板調用時仍然須要顯式指定集合名,即如 ctx.Render("wind,index")
