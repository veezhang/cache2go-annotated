/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"sync"
	"time"
)

// CacheItem is an individual cache item
// Parameter data contains the user-set value in the cache.
// 【CacheItem是单个的缓存条目，　也就是一个key-value缓存数据】
type CacheItem struct {
	// 【读写锁，保证CacheItem同步访问】
	sync.RWMutex

	// The item's key.
	// 【key 可以是任意类型】
	key interface{}
	// The item's data.
	// 【data 可以是任意类型】
	data interface{}
	// How long will the item live in the cache when not being accessed/kept alive.
	// 【不被访问后的保活时间】
	lifeSpan time.Duration

	// Creation timestamp.
	// 【创建的时间】
	createdOn time.Time
	// Last access timestamp.
	// 【最近一次访问时间，KeepAlive函数修改】
	accessedOn time.Time
	// How often the item was accessed.
	// 【访问的次数，KeepAlive函数修改】
	accessCount int64

	// Callback method triggered right before removing the item from the cache
	// 【被移除时候的回调函数】
	aboutToExpire []func(key interface{})
}

// NewCacheItem returns a newly created CacheItem.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
// 【创建CacheItem】
func NewCacheItem(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	t := time.Now()
	return &CacheItem{
		key:           key,
		lifeSpan:      lifeSpan,
		createdOn:     t,
		accessedOn:    t,
		accessCount:   0,
		aboutToExpire: nil,
		data:          data,
	}
}

// KeepAlive marks an item to be kept for another expireDuration period.
// 【重置过期时间， 需要加锁（下面类似的不再说）】
func (item *CacheItem) KeepAlive() {
	item.Lock()
	defer item.Unlock()
	item.accessedOn = time.Now()
	item.accessCount++
}

// LifeSpan returns this item's expiration duration.
// 【返回lifeSpan， 不需要加锁， 因为创建后就没有情况会修改此值（下面类似的不再说）】
func (item *CacheItem) LifeSpan() time.Duration {
	// immutable
	return item.lifeSpan
}

// AccessedOn returns when this item was last accessed.
// 【返回accessedOn】
func (item *CacheItem) AccessedOn() time.Time {
	item.RLock()
	defer item.RUnlock()
	return item.accessedOn
}

// CreatedOn returns when this item was added to the cache.
// 【返回createdOn】
func (item *CacheItem) CreatedOn() time.Time {
	// immutable
	return item.createdOn
}

// AccessCount returns how often this item has been accessed.
// 【返回accessCount】
func (item *CacheItem) AccessCount() int64 {
	item.RLock()
	defer item.RUnlock()
	return item.accessCount
}

// Key returns the key of this cached item.
// 【返回key】
func (item *CacheItem) Key() interface{} {
	// immutable
	return item.key
}

// Data returns the value of this cached item.
// 【返回data】
func (item *CacheItem) Data() interface{} {
	// immutable
	return item.data
}

// SetAboutToExpireCallback configures a callback, which will be called right
// before the item is about to be removed from the cache.
// 【设置被移除时候的回调函数】
func (item *CacheItem) SetAboutToExpireCallback(f func(interface{})) {
	if len(item.aboutToExpire) > 0 {
		item.RemoveAboutToExpireCallback()
	}
	item.Lock()
	defer item.Unlock()
	item.aboutToExpire = append(item.aboutToExpire, f)
}

// AddAboutToExpireCallback appends a new callback to the AboutToExpire queue
// 【添加被移除时候的回调函数】
func (item *CacheItem) AddAboutToExpireCallback(f func(interface{})) {
	item.Lock()
	defer item.Unlock()
	item.aboutToExpire = append(item.aboutToExpire, f)
}

// RemoveAboutToExpireCallback empties the about to expire callback queue
// 【删除被移除时候的回调函数】
func (item *CacheItem) RemoveAboutToExpireCallback() {
	item.Lock()
	defer item.Unlock()
	item.aboutToExpire = nil
}
