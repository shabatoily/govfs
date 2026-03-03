package types

import (
	vfs "github.com/meteormin/govfs"
)

type ViewType string

const (
	ViewTypeList ViewType = "list"
	ViewTypeTree ViewType = "tree"
)

type WriteReq struct {
	Content string `json:"content"`
}

type DstReq struct {
	Name string `json:"name"`
}

type WriteCommentReq struct {
	Comment string `json:"comment"`
}

type TreeNodeRes struct {
	Meta     MetaRes        `json:"meta"`
	Children []*TreeNodeRes `json:"children,omitempty"`
}

type MetaRes struct {
	vfs.Meta
	URL string `json:"url"`
}

type VfsRes[T any] struct {
	ViewType ViewType `json:"viewType" swaggertype:"string" enums:"list,tree"`
	Path     string   `json:"path"`
	Payload  T        `json:"payload" swaggertype:"object"`
}

type BadgerKeyRes struct {
	Prefix string   `json:"prefix"`
	Keys   []string `json:"keys"`
}
