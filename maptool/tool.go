package maptool

type KEY interface {
	~string | ~int | ~int32 | ~int64
}
