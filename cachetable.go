/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"log"
	"sort"
	"sync"
	"time"
)

// CacheTable is a table within the cache
// 描述缓存中的一个表项
type CacheTable struct {
	sync.RWMutex

	// The table's name.
	name string
	// All cached items.
	// 一个表中所有条目都存放在这个map里，map的key是任意类型，但值必须是CacheItem指针类型
	items map[interface{}]*CacheItem

	// Timer responsible for triggering cleanup.
	// 负责触发清除操作的计时器
	cleanupTimer *time.Timer

	// Current timer duration.
	// 清除操作触发的时间间隔
	cleanupInterval time.Duration

	// The logger used for this table.
	logger *log.Logger

	// Callback method triggered when trying to load a non-existing key.
	// 需要提取一个不存在的key时触发的回调函数
	loadData func(key interface{}, args ...interface{}) *CacheItem

	// Callback method triggered when adding a new item to the cache.
	// 增加一个缓存条目触发的回调函数
	addedItem []func(item *CacheItem)

	// Callback method triggered before deleting an item from the cache.
	// 删除前触发的回调函数
	aboutToDeleteItem []func(item *CacheItem)
}

// Count returns how many items are currently stored in the cache.
// 返回指定CacheTable中item的条目数，这里的table.items是一个map类型，len函数返回的是map中的元素数量
func (table *CacheTable) Count() int {
	table.RLock()
	defer table.RUnlock()
	return len(table.items)
}

// Foreach all items
// 接收一个函数参数，方法内遍历了items，把每个key和value都丢给trans函数处理
func (table *CacheTable) Foreach(trans func(key interface{}, item *CacheItem)) {
	table.RLock()
	defer table.RUnlock()

	for k, v := range table.items {
		trans(k, v)
	}
}

// SetDataLoader configures a data-loader callback, which will be called when
// trying to access a non-existing key. The key and 0...n additional arguments
// are passed to the callback function.
// 形参名：f 形参类型：func(interface{}, ...interface{}) *CacheItem
// 在SetDataLoader中将这个函数f丢给table的loadData属性。loadData所指向的方法当访问一个不存在的key时，需要调用一个方法，
// 这个方法通过SetDataLoader设定，方法的实现由用户自定义
func (table *CacheTable) SetDataLoader(f func(interface{}, ...interface{}) *CacheItem) {
	table.Lock()
	defer table.Unlock()
	table.loadData = f
}

// SetAddedItemCallback configures a callback, which will be called every time
// a new item is added to the cache.
// 当添加一个item到缓存表时会被调用的一个方法，绑定到CacheTable.addedItem上
// 设置了一个回调函数，当添加一个CacheItem的时候，同时会回调这个回调函数，这个函数可以选择对CacheItem做一些处理，比如打个日志等
func (table *CacheTable) SetAddedItemCallback(f func(*CacheItem)) {
	if len(table.addedItem) > 0 {
		table.RemoveAddedItemCallbacks()
	}
	table.Lock()
	defer table.Unlock()
	table.addedItem = append(table.addedItem, f)
}

//AddAddedItemCallback appends a new callback to the addedItem queue
func (table *CacheTable) AddAddedItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.addedItem = append(table.addedItem, f)
}

// RemoveAddedItemCallbacks empties the added item callback queue
func (table *CacheTable) RemoveAddedItemCallbacks() {
	table.Lock()
	defer table.Unlock()
	table.addedItem = nil
}

// SetAboutToDeleteItemCallback configures a callback, which will be called
// every time an item is about to be removed from the cache.
// 设删除时调用的一个方法
func (table *CacheTable) SetAboutToDeleteItemCallback(f func(*CacheItem)) {
	if len(table.aboutToDeleteItem) > 0 {
		table.RemoveAboutToDeleteItemCallback()
	}
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

// AddAboutToDeleteItemCallback appends a new callback to the AboutToDeleteItem queue
func (table *CacheTable) AddAboutToDeleteItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

// RemoveAboutToDeleteItemCallback empties the about to delete item callback queue
func (table *CacheTable) RemoveAboutToDeleteItemCallback() {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = nil
}

// SetLogger sets the logger to be used by this cache table.
// 把一个logger实例丢给table的logger属性
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.Lock()
	defer table.Unlock()
	table.logger = logger
}

// Expiration check loop, triggered by a self-adjusting timer.
// 由计时器触发的到期检查
func (table *CacheTable) expirationCheck() {
	table.Lock()
	// 计时器暂停
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
	// 计时器时间间隔
	if table.cleanupInterval > 0 {
		table.log("Expiration check triggered after", table.cleanupInterval, "for table", table.name)
	} else {
		table.log("Expiration check installed for table", table.name)
	}

	// To be more accurate with timers, we would need to update 'now' on every
	// loop iteration. Not sure it's really efficient though.
	// 当前时间
	now := time.Now()
	// 最小时间间隔此时暂定为0，后面的代码会更新
	smallestDuration := 0 * time.Second
	// 遍历一个table中的items
	for key, item := range table.items {
		// Cache values so we don't keep blocking the mutex.
		item.RLock()
		// 设置好存活时间
		lifeSpan := item.lifeSpan
		// 最后一次访问时间
		accessedOn := item.accessedOn
		item.RUnlock()

		// 存活时间为0的item不做处理，也就是一直存活
		if lifeSpan == 0 {
			continue
		}
		// 减法算出来这个item已经没有被访问的时间，如果比存活时间长，说明过期了，可以删了
		if now.Sub(accessedOn) >= lifeSpan {
			// Item has excessed its lifespan.
			// 删除操作
			table.deleteInternal(key)
		} else {
			// Find the item chronologically closest to its end-of-lifespan.
			// 按照时间顺序找到最接近过期的条目
			// 如果最后一次访问的时间到当前的时间间隔小于smallestDuration，则更新smallestDuration
			if smallestDuration == 0 || lifeSpan-now.Sub(accessedOn) < smallestDuration {
				smallestDuration = lifeSpan - now.Sub(accessedOn)
			}
		}
	}

	// Setup the interval for the next cleanup run.cleanupInterval
	// 上面已经找到了最接近过期时间的条目，这里将时间丢给
	table.cleanupInterval = smallestDuration
	if smallestDuration > 0 {
		// 计时器设置为smallestDuration，时间到则调用func函数
		table.cleanupTimer = time.AfterFunc(smallestDuration, func() {
			// 这里并不是循环启动goroutine，启动一个新的goroutine后当前goroutine会退出，这里不会引起goroutine泄露
			go table.expirationCheck()
		})
	}
	table.Unlock()
}

func (table *CacheTable) addInternal(item *CacheItem) {
	// Careful: do not run this method unless the table-mutex is locked!
	// It will unlock it for the caller before running the callbacks and checks
	table.log("Adding item with key", item.key, "and lifespan of", item.lifeSpan, "to table", table.name)
	table.items[item.key] = item

	// Cache values so we don't keep blocking the mutex.
	expDur := table.cleanupInterval
	addedItem := table.addedItem
	table.Unlock()

	// Trigger callback after adding an item to cache.
	// 添加一个item时需要调用的函数
	if addedItem != nil {
		for _, callback := range addedItem {
			callback(item)
		}
	}

	// If we haven't set up any expiration check timer or found a more imminent item.
	// item.lifeSpan > 0 当前item设置的存活时间是正数 expDur == 0 还没开始设置检查时间间隔
	// item.lifeSpan < expDur 设置了，但是当前新增的item的lifeSpan要更小
	if item.lifeSpan > 0 && (expDur == 0 || item.lifeSpan < expDur) {
		table.expirationCheck()
	}
}

// Add adds a key/value pair to the cache.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
// Add方法添加一个key/value到cache
func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := NewCacheItem(key, lifeSpan, data)

	// Add item to cache.
	table.Lock()
	table.addInternal(item)

	return item
}

func (table *CacheTable) deleteInternal(key interface{}) (*CacheItem, error) {
	// 如果table中不存在key对应的item，则返回一个error
	r, ok := table.items[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	// Cache value so we don't keep blocking the mutex.
	// 将要删除的item缓存起来
	aboutToDeleteItem := table.aboutToDeleteItem
	table.Unlock()

	// Trigger callbacks before deleting an item from cache.
	// 删除操作执行前调用的回调函数，这个函数是CacheTable的属性
	if aboutToDeleteItem != nil {
		for _, callback := range aboutToDeleteItem {
			callback(r)
		}
	}

	r.RLock()
	defer r.RUnlock()
	// 对item加了一个读锁，然后执行可aboutToExpire回调函数，这个函数需要在item刚好要前删除
	if r.aboutToExpire != nil {
		for _, callback := range r.aboutToExpire {
			callback(key)
		}
	}

	table.Lock()
	// 这里对表加了锁，上面已经对item加了读锁，然后这里执行delete函数删除了这个item
	table.log("Deleting item with key", key, "created on", r.createdOn, "and hit", r.accessCount, "times from table", table.name)
	delete(table.items, key)

	return r, nil
}

// Delete an item from the cache.
func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
	table.Lock()
	defer table.Unlock()

	return table.deleteInternal(key)
}

// Exists returns whether an item exists in the cache. Unlike the Value method
// Exists neither tries to fetch data via the loadData callback nor does it
// keep the item alive in the cache.
func (table *CacheTable) Exists(key interface{}) bool {
	table.RLock()
	defer table.RUnlock()
	_, ok := table.items[key]

	return ok
}

// NotFoundAdd checks whether an item is not yet cached. Unlike the Exists
// method this also adds data if the key could not be found.
func (table *CacheTable) NotFoundAdd(key interface{}, lifeSpan time.Duration, data interface{}) bool {
	table.Lock()

	// 检查items中是否有这个key，存在则返回false
	if _, ok := table.items[key]; ok {
		table.Unlock()
		return false
	}

	// 不存在时则创建一个CacheItem的实例，然后调用addInternal添加item，再返回true
	item := NewCacheItem(key, lifeSpan, data)
	table.addInternal(item)

	return true
}

// Value returns an item from the cache and marks it to be kept alive. You can
// pass additional arguments to your DataLoader callback function.
func (table *CacheTable) Value(key interface{}, args ...interface{}) (*CacheItem, error) {
	table.RLock()
	r, ok := table.items[key]
	// loadData在load一个不存在的数据时被调用的回调函数
	loadData := table.loadData
	table.RUnlock()

	// 如果值存在，执行下面的操作
	if ok {
		// Update access counter and timestamp.
		// 更新accessOn为当前时间
		r.KeepAlive()
		return r, nil
	}

	// Item doesn't exist in cache. Try and fetch it with a data-loader.
	// 值不存在的时候
	if loadData != nil {
		// loadData这个回调函数需要返回CacheItem类型的指针数据
		item := loadData(key, args...)
		if item != nil {
			// 当loadData返回了item的时候，万事大吉，执行Add
			table.Add(key, item.lifeSpan, item.data)
			return item, nil
		}

		// 当item没有拿到的时候，只能返回nil+错误信息
		return nil, ErrKeyNotFoundOrLoadable
	}

	// 当loadData为nil时，直接返回key找不到
	return nil, ErrKeyNotFound
}

// Flush deletes all items from this cache table.
// 清空数据 简单粗暴，让table的items的属性指向一个空map
func (table *CacheTable) Flush() {
	table.Lock()
	defer table.Unlock()

	table.log("Flushing table", table.name)

	table.items = make(map[interface{}]*CacheItem)
	table.cleanupInterval = 0
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
}

// CacheItemPair maps key to access counter
// 用来映射key到访问计数的
type CacheItemPair struct {
	Key         interface{}
	AccessCount int64
}

// CacheItemPairList is a slice of CacheIemPairs that implements sort.
// Interface to sort by AccessCount.
// 是一个CacheItemPair的列表，对应的就是切片
type CacheItemPairList []CacheItemPair

// 交换
func (p CacheItemPairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
// 取长度
func (p CacheItemPairList) Len() int           { return len(p) }
// 判断CacheItemPairList列表中第i个CacheItemPair和第j个CacheItemPair的AccessCount的大小
func (p CacheItemPairList) Less(i, j int) bool { return p[i].AccessCount > p[j].AccessCount }

// MostAccessed returns the most accessed items in this cache table
// 访问高频的count条的item全部返回
func (table *CacheTable) MostAccessed(count int64) []*CacheItem {
	table.RLock()
	defer table.RUnlock()

	// 这里的CacheItemPairList是[]CacheItemPair类型，是类型不是实例
	// 因此p是一个长度为len(table.item)的一个CacheItemPair类型数据存入p切片
	p := make(CacheItemPairList, len(table.items))
	i := 0
	// 遍历items，将key和AccessCount够造成的CacheItemPair类型数据存入p切片
	for k, v := range table.items {
		p[i] = CacheItemPair{k, v.accessCount}
		i++
	}

	// 这里可以直接使用sort方法进行排序是因为CacheItemPairList实现了sort.Interface接口，也就是Swap，Len，Less三个方法
	sort.Sort(p)

	var r []*CacheItem
	c := int64(0)
	for _, v := range p {
		// 控制返回值的数目
		if c >= count {
			break
		}

		item, ok := table.items[v.Key]
		if ok {
			// 如果数据是按照访问频率从高到底排序的，所以可以从第一条数据开始加
			r = append(r, item)
		}
		c++
	}

	return r
}

// Internal logging method for convenience.
func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}

	table.logger.Println(v...)
}
