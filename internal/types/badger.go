package types

import "github.com/shabatoily/govfs/pkg/drivers/badger"

// BadgerKeyRes는 BadgerDB의 키 목록 조회 결과를 담고 있는 구조체입니다.
type BadgerKeyRes struct {
	Prefix string   `json:"prefix"` // 조회 시 사용한 접두사
	Keys   []string `json:"keys"`   // 매칭된 키 목록
}

// BadgerStatRes는 BadgerDB의 통계 정보를 담고 있는 구조체입니다.
type BadgerStatRes struct {
	TotalCount int                     `json:"totalCount"` // 개수
	TotalSize  int64                   `json:"totalSize"`  // 크기
	PrefixBy   map[string]badger.Stats `json:"prefixBy"`   // 접두사별 통계
}

type RotateKeyReq struct {
	Key string `json:"key"`
}
