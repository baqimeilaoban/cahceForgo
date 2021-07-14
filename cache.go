/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2012, Radu Ioan Fericean
 *                   2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"sync"
)

var (
	cache = make(map[string]*CacheTable)
	mutex sync.RWMutex
)

// Cache returns the existing cache table with given name or creates a new one
// if the table does not exist yet.
// 表存在则返回表，表不存在的时候创建一个不包含items的空表，然后返回
func Cache(table string) *CacheTable {
	mutex.RLock()
	// 一个用于存放CacheTable的map
	t, ok := cache[table]
	mutex.RUnlock()

	if !ok {
		mutex.Lock()
		// 表不存在的时候需要创建一个空表，这时候做了二次检查，为的是并发安全
		t, ok = cache[table]
		// Double check whether the table exists or not.
		if !ok {
			t = &CacheTable{
				name:  table,
				items: make(map[interface{}]*CacheItem),
			}
			cache[table] = t
		}
		mutex.Unlock()
	}

	return t
}
