package cache

type TagCache struct {
	store  CacheStore
	tagSet *TagSet
}

var _ Cache = new(TagCache)

func NewTagCache(store CacheStore, names ...string) Cache {
	return &TagCache{store: store, tagSet: NewTagSet(store, names)}
}

func (t *TagCache) TaggedItemKey(key string) string {
	return EncodeSha1(t.tagSet.GetNamespace()) + ":" + key
}

func (t *TagCache) Get(key string, _val interface{}) error {
	return t.store.Get(t.TaggedItemKey(key), _val)
}

func (t *TagCache) IsExist(key string) bool {
	return t.store.IsExist(t.TaggedItemKey(key))
}

// 更新过期时间
func (t *TagCache) Touch(key string, expire int64) error {
	return t.store.Touch(t.TaggedItemKey(key), expire)
}

func (t *TagCache) Set(key string, value interface{}, expire int64) error {
	return t.store.Set(t.TaggedItemKey(key), value, expire)
}

func (t *TagCache) Incr(key string) (int64, error) {
	return t.store.Incr(t.TaggedItemKey(key))
}

func (t *TagCache) Decr(key string) (int64, error) {
	return t.store.Decr(t.TaggedItemKey(key))
}

func (t *TagCache) Delete(key string) error {
	return t.store.Delete(t.TaggedItemKey(key))
}

func (t *TagCache) Flush() error {
	return t.tagSet.Reset()
}

// add Tags
func (t *TagCache) Tags(tags []string) Cache {
	t.tagSet.AddNames(tags)
	return t
}

func (t *TagCache) StartAndGC(opt Options) error {
	return t.store.StartAndGC(opt)
}
