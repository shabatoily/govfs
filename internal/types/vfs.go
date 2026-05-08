// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

import (
	vfs "github.com/meteormin/govfs"
)

// ViewType은 VFS 데이터의 조회 방식을 정의합니다.
type ViewType string

const (
	// ViewTypeList는 플랫한 파일 목록 방식을 의미합니다.
	ViewTypeList ViewType = "list"
	// ViewTypeTree는 계층적인 트리 방식을 의미합니다.
	ViewTypeTree ViewType = "tree"
)

// WriteReq는 파일 내용 업데이트 요청 시 사용되는 구조체입니다.
type WriteReq struct {
	Content string `json:"content"`
}

// DstReq는 이동 또는 복사 시 대상 경로 이름을 담고 있는 구조체입니다.
type DstReq struct {
	Name string `json:"name"`
}

// WriteCommentReq는 각 파일의 설명을 업데이트하거나 생성할 때 사용하는 구조체입니다.
type WriteCommentReq struct {
	Comment string `json:"comment"`
}

// TreeNodeRes는 트리 형태의 VFS 응답을 위한 노드 구조체입니다.
type TreeNodeRes struct {
	Meta     MetaRes        `json:"meta"`               // 노드 메타데이터
	Children []*TreeNodeRes `json:"children,omitempty"` // 하위 자식 노드 목록
}

// MetaRes는 VFS 메타데이터에 실제 파일 접근 URL을 포함한 응답용 구조체입니다.
type MetaRes struct {
	vfs.Meta
	URL string `json:"url"` // 파일 다운로드 또는 접근 URL
}

// VfsRes는 VFS 목록/트리 조회 요청에 대한 공통 응답 구조체입니다.
type VfsRes[T any] struct {
	ViewType ViewType `json:"viewType" swaggertype:"string" enums:"list,tree"` // 조회 방식
	Path     string   `json:"path"`                                            // 요청된 경로
	Payload  T        `json:"payload" swaggertype:"object"`                    // 실제 데이터 페이로드
}

// BadgerKeyRes는 BadgerDB의 키 목록 조회 결과를 담고 있는 구조체입니다.
type BadgerKeyRes struct {
	Prefix string   `json:"prefix"` // 조회 시 사용한 접두사
	Keys   []string `json:"keys"`   // 매칭된 키 목록
}

// BadgerStatRes는 BadgerDB의 통계 정보를 담고 있는 구조체입니다.
type BadgerStatRes struct {
	Count int   `json:"count"` // 개수
	Size  int64 `json:"size"`  // 크기
}
