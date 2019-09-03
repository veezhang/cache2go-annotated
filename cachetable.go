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
// 【缓存表， 】
type CacheTable struct {
	// 【读写锁，保证CacheItem同步访问】
	sync.RWMutex

	// The table's name.
	// 【缓存表名称】
	name string
	// All cached items.
	// 【所有缓存的条目】
	items map[interface{}]*CacheItem

	// Timer responsible for triggering cleanup.
	// 【负责触发清除操作的计时器】
	cleanupTimer *time.Timer
	// Current timer duration.
	// 【触发清理清除操作的时间间隔】
	cleanupInterval time.Duration

	// The logger used for this table.
	// 【日志】
	logger *log.Logger

	// Callback method triggered when trying to load a non-existing key.
	// 【加载一个不存在的key时触发的回调函数】
	loadData func(key interface{}, args ...interface{}) *CacheItem
	// Callback method triggered when adding a new item to the cache.
	// 【添加缓存条目时触发的回调函数】
	addedItem []func(item *CacheItem)
	// Callback method triggered before deleting an item from the cache.
	// 【删除缓存条目时触发的回调函数】
	aboutToDeleteItem []func(item *CacheItem)
}

// Count returns how many items are currently stored in the cache.
// 【返回缓存条目的数量】
func (table *CacheTable) Count() int {
	table.RLock()
	defer table.RUnlock()
	return len(table.items)
}

// Foreach all items
// 【遍历缓存条目】
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
// 【设置加载一个不存在的key时触发的回调函数】
func (table *CacheTable) SetDataLoader(f func(interface{}, ...interface{}) *CacheItem) {
	table.Lock()
	defer table.Unlock()
	table.loadData = f
}

// SetAddedItemCallback configures a callback, which will be called every time
// a new item is added to the cache.
// 【设置添加缓存条目时触发的回调函数】
func (table *CacheTable) SetAddedItemCallback(f func(*CacheItem)) {
	if len(table.addedItem) > 0 {
		table.RemoveAddedItemCallbacks()
	}
	table.Lock()
	defer table.Unlock()
	table.addedItem = append(table.addedItem, f)
}

//AddAddedItemCallback appends a new callback to the addedItem queue
// 【添加添加缓存条目时触发的回调函数】
func (table *CacheTable) AddAddedItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.addedItem = append(table.addedItem, f)
}

// RemoveAddedItemCallbacks empties the added item callback queue
// 【删除添加缓存条目时触发的回调函数】
func (table *CacheTable) RemoveAddedItemCallbacks() {
	table.Lock()
	defer table.Unlock()
	table.addedItem = nil
}

// SetAboutToDeleteItemCallback configures a callback, which will be called
// every time an item is about to be removed from the cache.
// 【设置删除缓存条目时触发的回调函数】
func (table *CacheTable) SetAboutToDeleteItemCallback(f func(*CacheItem)) {
	if len(table.aboutToDeleteItem) > 0 {
		table.RemoveAboutToDeleteItemCallback()
	}
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

// AddAboutToDeleteItemCallback appends a new callback to the AboutToDeleteItem queue
// 【添加删除缓存条目时触发的回调函数】
func (table *CacheTable) AddAboutToDeleteItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

// RemoveAboutToDeleteItemCallback empties the about to delete item callback queue
// 【删除删除缓存条目时触发的回调函数】
func (table *CacheTable) RemoveAboutToDeleteItemCallback() {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = nil
}

// SetLogger sets the logger to be used by this cache table.
// 【设置日志】
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.Lock()
	defer table.Unlock()
	table.logger = logger
}

// Expiration check loop, triggered by a self-adjusting timer.
// 【过期检查， 能自动调节间隔】
func (table *CacheTable) expirationCheck() {
	table.Lock()
	// 【计时器停止，后面调整间隔后启动】
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
	if table.cleanupInterval > 0 {
		table.log("Expiration check triggered after", table.cleanupInterval, "for table", table.name)
	} else {
		table.log("Expiration check installed for table", table.name)
	}

	// To be more accurate with timers, we would need to update 'now' on every
	// loop iteration. Not sure it's really efficient though.
	// 【每次会更新】
	now := time.Now()
	// 【最小时间间隔】
	smallestDuration := 0 * time.Second
	// 【遍历所有的items查找最近一个将要过期的时间间隔】
	for key, item := range table.items {
		// Cache values so we don't keep blocking the mutex.
		item.RLock()
		lifeSpan := item.lifeSpan
		accessedOn := item.accessedOn
		item.RUnlock()
		
		// 【0 表示永远不过期】
		if lifeSpan == 0 {
			continue
		}
		// 【已经过期了，删除】
		if now.Sub(accessedOn) >= lifeSpan {
			// Item has excessed its lifespan.
			table.deleteInternal(key)
		} else {
			// Find the item chronologically closest to its end-of-lifespan.
			// 【更新smallestDuration， 获取最近一个将要过期的时间间隔】
			if smallestDuration == 0 || lifeSpan-now.Sub(accessedOn) < smallestDuration {
				smallestDuration = lifeSpan - now.Sub(accessedOn)
			}
		}
	}

	// Setup the interval for the next cleanup run.
	//【设置cleanupInterval为最近将要过期的时间间隔】
	table.cleanupInterval = smallestDuration
	if smallestDuration > 0 {
		// 【重新启动下一次的过期检测】
		table.cleanupTimer = time.AfterFunc(smallestDuration, func() {
			go table.expirationCheck()
		})
	}
	table.Unlock()
}

// 【内部添加函数， 代码重用， 调用这个方法之前需要加锁】
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
	// 触发增加条数的回调函数
	if addedItem != nil {
		for _, callback := range addedItem {
			callback(item)
		}
	}

	// If we haven't set up any expiration check timer or found a more imminent item.
	// 如果当前没有过期检测函数或者当前添加的比当前最短的过期时间还早过期，则更新过期检测
	if item.lifeSpan > 0 && (expDur == 0 || item.lifeSpan < expDur) {
		table.expirationCheck()
	}
}

// Add adds a key/value pair to the cache.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
// 【添加缓存条目到缓存表中， addInternal会释放锁】
func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := NewCacheItem(key, lifeSpan, data)

	// Add item to cache.
	table.Lock()
	table.addInternal(item)

	return item
}

// 【内部删除函数， 代码重用， 调用这个方法之前需要加锁】
func (table *CacheTable) deleteInternal(key interface{}) (*CacheItem, error) {
	r, ok := table.items[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	// Cache value so we don't keep blocking the mutex.
	aboutToDeleteItem := table.aboutToDeleteItem
	table.Unlock()

	// Trigger callbacks before deleting an item from cache.
	// 触发删除条数的回调函数
	if aboutToDeleteItem != nil {
		for _, callback := range aboutToDeleteItem {
			callback(r)
		}
	}

	// 触发CacheItem过期删除的回调函数
	r.RLock()
	defer r.RUnlock()
	if r.aboutToExpire != nil {
		for _, callback := range r.aboutToExpire {
			callback(key)
		}
	}

	table.Lock()
	table.log("Deleting item with key", key, "created on", r.createdOn, "and hit", r.accessCount, "times from table", table.name)
	delete(table.items, key)

	return r, nil
}

// Delete an item from the cache.
// 【从缓存表中删除缓存条目， addInternal会释放锁】
func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
	table.Lock()
	defer table.Unlock()

	return table.deleteInternal(key)
}

// Exists returns whether an item exists in the cache. Unlike the Value method
// Exists neither tries to fetch data via the loadData callback nor does it
// keep the item alive in the cache.
// 【是否存在某个key】
func (table *CacheTable) Exists(key interface{}) bool {
	table.RLock()
	defer table.RUnlock()
	_, ok := table.items[key]

	return ok
}

// NotFoundAdd tests whether an item not found in the cache. Unlike the Exists
// method this also adds data if they key could not be found.
// 【不存在才添加】
func (table *CacheTable) NotFoundAdd(key interface{}, lifeSpan time.Duration, data interface{}) bool {
	table.Lock()

	if _, ok := table.items[key]; ok {
		table.Unlock()
		return false
	}

	item := NewCacheItem(key, lifeSpan, data)
	table.addInternal(item)

	return true
}

// Value returns an item from the cache and marks it to be kept alive. You can
// pass additional arguments to your DataLoader callback function.
// 【获取value, 会通过KeepAlive更新访问时间和访问次数】
func (table *CacheTable) Value(key interface{}, args ...interface{}) (*CacheItem, error) {
	table.RLock()
	r, ok := table.items[key]
	loadData := table.loadData
	table.RUnlock()

	if ok {
		// Update access counter and timestamp.
		r.KeepAlive()
		return r, nil
	}

	// Item doesn't exist in cache. Try and fetch it with a data-loader.
	// 【如果不存在，通过loadData获取】
	if loadData != nil {
		item := loadData(key, args...)
		if item != nil {
			table.Add(key, item.lifeSpan, item.data)
			return item, nil
		}

		return nil, ErrKeyNotFoundOrLoadable
	}

	return nil, ErrKeyNotFound
}

// Flush deletes all items from this cache table.
// 【清除所有的缓存条目， 不会调用 缓存表的aboutToDeleteItem 和 缓存条目的aboutToExpire 】
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
// 【缓存条目对】
type CacheItemPair struct {
	Key         interface{}
	AccessCount int64
}

// CacheItemPairList is a slice of CacheIemPairs that implements sort.
// Interface to sort by AccessCount.
// 【缓存条目对切片】
type CacheItemPairList []CacheItemPair

// 【qsort需要的一些函数， 根据访问次数排序】
func (p CacheItemPairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p CacheItemPairList) Len() int           { return len(p) }
func (p CacheItemPairList) Less(i, j int) bool { return p[i].AccessCount > p[j].AccessCount }

// MostAccessed returns the most accessed items in this cache table
// 【获取访问最多的几个CacheItem， 最多访问count个】
func (table *CacheTable) MostAccessed(count int64) []*CacheItem {
	table.RLock()
	defer table.RUnlock()

	p := make(CacheItemPairList, len(table.items))
	i := 0
	for k, v := range table.items {
		p[i] = CacheItemPair{k, v.accessCount}
		i++
	}
	sort.Sort(p)

	var r []*CacheItem
	c := int64(0)
	for _, v := range p {
		if c >= count {
			break
		}

		item, ok := table.items[v.Key]
		if ok {
			r = append(r, item)
		}
		c++
	}

	return r
}

// Internal logging method for convenience.
// 【内部日志打印】
func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}

	table.logger.Println(v...)
}
