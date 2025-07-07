package slicetool

import (
	"github.com/cheny2151/go-toolbox/maptool"
	"slices"
)

// TakeValues 数组元素指针转为数组元素值并返回元素值数组
func TakeValues[T any](points []*T) []T {
	values := make([]T, len(points))
	for i := range points {
		values[i] = *points[i]
	}
	return values
}

// ArrayToMap 数组转map
func ArrayToMap[K maptool.KEY, T any](arr []T, keyFunc func(ele T) K) map[K]T {
	m := make(map[K]T, len(arr))
	for i := range arr {
		ele := arr[i]
		key := keyFunc(ele)
		m[key] = ele
	}
	return m
}

func Distinct[T comparable](arr []T) []T {
	if arr == nil || len(arr) == 1 {
		return arr
	}
	result := make([]T, 0, len(arr))
	cache := make(map[T]bool)
	for _, e := range arr {
		if _, ok := cache[e]; !ok {
			result = append(result, e)
			cache[e] = true
		}
	}
	return result
}

func Subtract[E comparable](arr []E, sub []E) []E {
	result := make([]E, 0)
	for i := range sub {
		e := sub[i]
		if !slices.Contains(arr, e) {
			result = append(result, e)
		}
	}
	return result
}
