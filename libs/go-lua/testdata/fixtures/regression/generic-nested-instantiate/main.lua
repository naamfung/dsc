type Wrapper<T> = {inner: T}
type DoubleWrap<T> = Wrapper<Wrapper<T>>
local dw: DoubleWrap<string> = {inner = {inner = "hello"}}
local s: string = dw.inner.inner
