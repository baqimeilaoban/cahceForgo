package main

import (
	"fmt"
	"time"

	"github.com/muesli/cache2go"
)

// Keys & values in cache2go can be of arbitrary types, e.g. a struct.
type myStruct struct {
	text     string
	moreData []byte
}

func main() {
	// Accessing a new cache table for the first time will create it.
	// 创建缓存表
	cache := cache2go.Cache("myCache")

	// We will put a new item in the cache. It will expire after
	// not being accessed via Value(key) for more than 5 seconds.
	// 构造一个数据
	val := myStruct{"This is a test!", []byte{}}
	// 存入数据，设置存活时间为5s
	cache.Add("someKey", 5*time.Second, &val)

	// Let's retrieve the item from the cache.
	// 试着读取
	res, err := cache.Value("someKey")
	if err == nil {
		fmt.Println("Found value in cache:", res.Data().(*myStruct).text)
	} else {
		fmt.Println("Error retrieving value from cache:", err)
	}

	// Wait for the item to expire in cache.
	// 等待6s后，明显过期了
	time.Sleep(6 * time.Second)
	// 再进行读取
	res, err = cache.Value("someKey")
	if err != nil {
		fmt.Println("Item is not cached (anymore).")
	}

	// Add another item that never expires.
	// 存放一个永不过期的数据
	cache.Add("someKey", 0, &val)

	// cache2go supports a few handy callbacks and loading mechanisms.
	// 设置回调函数，删除数据的时候调用
	cache.SetAboutToDeleteItemCallback(func(e *cache2go.CacheItem) {
		fmt.Println("Deleting:", e.Key(), e.Data().(*myStruct).text, e.CreatedOn())
	})

	// Remove the item from the cache.
	// 删除数据
	cache.Delete("someKey")

	// And wipe the entire cache table.
	// 抹去整个cache中的数据
	cache.Flush()
}
