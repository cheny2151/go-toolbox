package slicetool

import "github.com/cheny2151/go-toolbox/maptool"

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
