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
	// 全局缓存表的Map, 支持多个缓存表
	cache = make(map[string]*CacheTable)
	// cache的锁
	mutex sync.RWMutex
)

// Cache returns the existing cache table with given name or creates a new one
// if the table does not exist yet.
// 【创建缓存】
func Cache(table string) *CacheTable {
	mutex.RLock()
	t, ok := cache[table]
	mutex.RUnlock()

	if !ok {
		mutex.Lock()
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
