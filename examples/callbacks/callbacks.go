package main

import (
	"fmt"
	"time"

	"github.com/muesli/cache2go"
)

func main() {
	// 创建一个名为myCache的缓存表
	cache := cache2go.Cache("myCache")

	// This callback will be triggered every time a new item
	// gets added to the cache.
	// 每次有新的item被加入到这个缓存表的时候会被触发的回调函数
	cache.SetAddedItemCallback(func(entry *cache2go.CacheItem) {
		fmt.Println("Added Callback 1:", entry.Key(), entry.Data(), entry.CreatedOn())
	})
	cache.AddAddedItemCallback(func(entry *cache2go.CacheItem) {
		fmt.Println("Added Callback 2:", entry.Key(), entry.Data(), entry.CreatedOn())
	})
	// This callback will be triggered every time an item
	// is about to be removed from the cache.
	// 当一个item被删除时被触发一个回调函数
	cache.SetAboutToDeleteItemCallback(func(entry *cache2go.CacheItem) {
		fmt.Println("Deleting:", entry.Key(), entry.Data(), entry.CreatedOn())
	})

	// Caching a new item will execute the AddedItem callback.
	// 缓存中添加一条数据
	cache.Add("someKey", 0, "This is a test!")

	// Let's retrieve the item from the cache
	// 读取刚才存入的数据
	res, err := cache.Value("someKey")
	if err == nil {
		fmt.Println("Found value in cache:", res.Data())
	} else {
		fmt.Println("Error retrieving value from cache:", err)
	}

	// Deleting the item will execute the AboutToDeleteItem callback.
	// 删除someKey对应的记录
	cache.Delete("someKey")

	cache.RemoveAddedItemCallbacks()
	// Caching a new item that expires in 3 seconds
	// 设置了3s的存活时间
	res = cache.Add("anotherKey", 3*time.Second, "This is another test")

	// This callback will be triggered when the item is about to expire
	// 一旦触发了删除操作就会调用下面这个回调函数，在这里也就是3s到期时执行
	res.SetAboutToExpireCallback(func(key interface{}) {
		fmt.Println("About to expire:", key.(string))
	})

	// 等3s时间到
	time.Sleep(5 * time.Second)
}
